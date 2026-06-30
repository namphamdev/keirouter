package headroom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// Compressor wraps a fail-open HTTP client and a logger. It compresses a
// request's messages by calling an external Headroom proxy and replacing the
// messages with the compressed version. Every failure path is fail-open: the
// request is left untouched and no error is propagated to the caller.
type Compressor struct {
	client *http.Client
	log    *slog.Logger
}

// New constructs a Compressor. The HTTP client deliberately has NO client-level
// Timeout: the per-call deadline is enforced via context so a late response is
// abandoned through context cancellation rather than a transport-level timeout.
// A nil logger falls back to slog.Default().
func New(log *slog.Logger) *Compressor {
	if log == nil {
		log = slog.Default()
	}
	return &Compressor{
		client: &http.Client{},
		log:    log,
	}
}

// compressRequest is the JSON body POSTed to Compress_Endpoint.
type compressRequest struct {
	Messages []openAIMessage `json:"messages"`
	Model    string          `json:"model"`
	Config   *compressConfig `json:"config,omitempty"`
}

// compressConfig carries the optional per-call proxy configuration.
type compressConfig struct {
	CompressUserMessages bool `json:"compress_user_messages"`
}

// compressResponse is the decoded proxy response. A nil/absent/empty Messages
// slice is treated as a failure (fail-open); only a non-empty array counts as a
// successful compression.
type compressResponse struct {
	Messages []openAIMessage `json:"messages"`
	Stats    *compressStats  `json:"stats"`
}

// compressStats mirrors the proxy-reported token statistics. Absent stats leave
// every token field at zero.
type compressStats struct {
	TokensBefore int `json:"tokens_before"`
	TokensAfter  int `json:"tokens_after"`
	TokensSaved  int `json:"tokens_saved"`
}

// Compress mutates req.Messages in place when compression succeeds and returns
// a Stats snapshot. It NEVER returns an error and NEVER panics out: every
// failure path is fail-open (req left unchanged) and logged at warning level
// with a masked URL.
func (c *Compressor) Compress(ctx context.Context, req *core.ChatRequest, cfg Config) *Stats {
	// Skip entirely when disabled or no URL is configured.
	if req == nil || !cfg.Enabled || strings.TrimSpace(cfg.URL) == "" {
		return &Stats{}
	}

	// Capture the outbound JSON size before the call so phantom-savings can be
	// judged against the real payload, not the proxy's token claim.
	bytesBefore := jsonBytes(toOpenAIMessages(req))

	resp, err := c.callCompress(ctx, req, cfg)
	if err != nil {
		c.logFailOpen(cfg.URL, err)
		return &Stats{}
	}
	if resp == nil || len(resp.Messages) == 0 {
		// Missing / null / non-array / empty messages -> fail-open.
		c.logFailOpen(cfg.URL, errors.New("response contained no compressed messages"))
		return &Stats{}
	}

	// Success: replace the request messages with the compressed mapping and
	// measure the resulting payload size.
	req.Messages = fromOpenAIMessages(resp.Messages)
	bytesAfter := jsonBytes(toOpenAIMessages(req))

	stats := Stats{
		BytesBefore: bytesBefore,
		BytesAfter:  bytesAfter,
		Compressed:  true,
	}
	if resp.Stats != nil {
		stats.TokensBefore = resp.Stats.TokensBefore
		stats.TokensAfter = resp.Stats.TokensAfter
		stats.TokensSaved = resp.Stats.TokensSaved
	}
	// Phantom detection: tokens claimed saved but the body did not shrink.
	stats.Phantom = isPhantom(bytesBefore, bytesAfter, defaultMinShrinkRatio)

	clamped := stats.clamp()
	return &clamped
}

// callCompress performs the POST to Compress_Endpoint with a context deadline
// of cfg.Timeout. It returns an error for any non-success condition (transport
// error, non-2xx status, decode failure, timeout/late response via context
// cancellation) so the caller can fail open.
func (c *Compressor) callCompress(ctx context.Context, req *core.ChatRequest, cfg Config) (*compressResponse, error) {
	body := compressRequest{
		Messages: toOpenAIMessages(req),
		Model:    req.Model,
	}
	if cfg.CompressUserMessages {
		body.Config = &compressConfig{CompressUserMessages: true}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// The per-call deadline; a response arriving after it is abandoned because
	// client.Do returns a context error.
	callCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	endpoint := buildCompressEndpoint(cfg.URL)
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		return nil, fmt.Errorf("unexpected status %d", httpResp.StatusCode)
	}

	var decoded compressResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return &decoded, nil
}

// logFailOpen records a fail-open warning with a masked endpoint (never the raw
// URL, so embedded credentials and query strings stay out of logs). The work is
// wrapped in a recover so that a logging failure is itself suppressed and never
// escapes the pipeline.
func (c *Compressor) logFailOpen(rawURL string, cause error) {
	defer func() { _ = recover() }()
	c.log.Warn("headroom compression failed; leaving request unchanged (fail-open)",
		"endpoint", maskURL(rawURL),
		"error", cause,
	)
}
