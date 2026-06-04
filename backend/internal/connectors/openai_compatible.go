package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/transform"
)

// OpenAICompatible drives any endpoint that speaks the OpenAI Chat Completions
// API: OpenAI itself, plus GLM, MiniMax, DeepSeek, Groq, Together, and custom
// gateways. The provider id and default base URL are supplied at construction
// so one implementation backs many registered providers.
type OpenAICompatible struct {
	id          string
	defaultBase string
	codec       transform.OpenAICodec
}

// NewOpenAICompatible builds a connector for an OpenAI-compatible provider.
func NewOpenAICompatible(id, defaultBaseURL string) *OpenAICompatible {
	return &OpenAICompatible{id: id, defaultBase: defaultBaseURL}
}

func (c *OpenAICompatible) ID() string            { return c.id }
func (c *OpenAICompatible) Dialect() core.Dialect { return core.DialectOpenAI }

func (c *OpenAICompatible) baseURL(creds core.Credentials) string {
	u := c.defaultBase
	if creds.BaseURL != "" {
		u = creds.BaseURL
	}
	// Resolve template placeholders like {accountId} from creds.Extra.
	// Cloudflare Workers AI uses: /accounts/{accountId}/ai/v1/chat/completions
	for key, val := range creds.Extra {
		u = strings.ReplaceAll(u, "{"+key+"}", val)
	}
	return u
}

func (c *OpenAICompatible) headers(creds core.Credentials) map[string]string {
	h := map[string]string{}
	switch {
	case creds.AccessToken != "":
		h["Authorization"] = bearer(creds.AccessToken)
	case creds.APIKey != "":
		h["Authorization"] = bearer(creds.APIKey)
	}
	return mergeHeaders(h, creds.Headers)
}

// Chat performs a non-streaming completion.
func (c *OpenAICompatible) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := joinURL(c.baseURL(creds), "chat/completions")

	// Use streaming JSON decode when the codec supports it — avoids buffering
	// the entire response body into a []byte before parsing.
	if sc, ok := interface{}(c.codec).(transform.StreamingResponseCodec); ok {
		_, respBody, decErr := doJSONDecode(ctx, c.id, req.Model, url, body, c.headers(creds))
		if decErr != nil {
			return nil, decErr
		}
		defer respBody.Close()
		resp, perr := sc.ParseResponseFrom(respBody, req.Model)
		if perr != nil {
			return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: perr.Error(), Cause: perr}
		}
		return resp, nil
	}

	// Fallback: buffer the entire response body.
	respBody, err := doJSON(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	resp, err := c.codec.ParseResponse(respBody, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

// Validate probes the upstream /models endpoint to confirm the credentials are
// accepted. Returns nil on success.
func (c *OpenAICompatible) Validate(ctx context.Context, creds core.Credentials) error {
	url := joinURL(c.baseURL(creds), "models")
	_, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", url, nil, c.headers(creds))
	if err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

// OpenAICompatibleModelSource implements LiveModelSource by fetching the
// upstream GET /models endpoint — the standard discovery API for OpenAI-
// compatible providers. This auto-discovers models at runtime using the
// connected account's credentials.
type OpenAICompatibleModelSource struct {
	provider    string
	defaultBase string
}

// ListModels fetches GET /models from the upstream and returns ModelSpecs.
func (s *OpenAICompatibleModelSource) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	base := s.defaultBase
	if creds.BaseURL != "" {
		base = creds.BaseURL
	}
	// Resolve template placeholders (e.g. cloudflare {accountId}).
	for key, val := range creds.Extra {
		base = strings.ReplaceAll(base, "{"+key+"}", val)
	}

	url := joinURL(base, "models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case creds.AccessToken != "":
		req.Header.Set("Authorization", bearer(creds.AccessToken))
	case creds.APIKey != "":
		req.Header.Set("Authorization", bearer(creds.APIKey))
	}
	req.Header.Set("Accept", "application/json")

	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("GET /models returned %d: %s", resp.StatusCode, truncateError(body))
	}

	// Parse the standard OpenAI models response shape.
	var envelope struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode /models response: %w", err)
	}

	out := make([]ModelSpec, 0, len(envelope.Data))
	for _, entry := range envelope.Data {
		if entry.ID == "" {
			continue
		}
		out = append(out, ModelSpec{
			ID:   entry.ID,
			Name: entry.ID, // best-effort; static catalog may have a better name
			Kind: core.ServiceLLM,
		})
	}
	return out, nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping. The caller must close body when done.
func (c *OpenAICompatible) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := joinURL(c.baseURL(creds), "chat/completions")
	resp, err := openStream(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}

// Stream performs a streaming completion, emitting canonical chunks.
func (c *OpenAICompatible) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := joinURL(c.baseURL(creds), "chat/completions")
	resp, err := openStream(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	out := make(chan core.StreamChunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		streamStart := time.Now()
		ttftReported := false

		scanner := sseScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			payload, ok := parseSSEData(scanner.Text())
			if !ok {
				continue
			}
			chunks, perr := c.codec.ParseStreamLine([]byte(payload), req.Model)
			if perr != nil {
				// Skip a single malformed chunk rather than aborting the stream.
				continue
			}
			for _, ch := range chunks {
				if !ttftReported && isMeaningfulChunk(ch) && cfg.OnFirstChunk != nil {
					ttftReported = true
					cfg.OnFirstChunk(time.Since(streamStart))
				}
				select {
				case out <- ch:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			out <- core.StreamChunk{
				Type: core.ChunkError,
				Err:  &core.ProviderError{Kind: core.ErrTimeout, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err},
			}
		}
	}()
	return out, nil
}