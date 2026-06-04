package gateway

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mydisha/keirouter/backend/internal/budget"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/dispatch"
	"github.com/mydisha/keirouter/backend/internal/pipeline"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/transform"
)

// maxBodyBytes caps inbound request bodies to protect against oversized uploads.
const maxBodyBytes = 32 << 20 // 32 MiB

// logRequest logs a completed request to the console log buffer.
func (s *Server) logRequest(provider, model string, tokens int, costMicros int64, latencyMs int, cacheHit bool, err error) {
	if s.consoleLog == nil {
		return
	}
	level := "INFO"
	if err != nil {
		level = "ERROR"
	} else if latencyMs > 8000 {
		level = "WARN"
	}
	cost := float64(costMicros) / 1_000_000
	cache := ""
	if cacheHit {
		cache = " · cache"
	}
	s.consoleLog.Logf(level, "%s · %s · %d tok · $%.4f · %dms%s",
		provider, model, tokens, cost, latencyMs, cache)
	if err != nil {
		s.consoleLog.Logf("ERROR", "  └─ %v", err)
	}
}

// handleOpenAIChat serves /v1/chat/completions in the OpenAI dialect.
func (s *Server) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, core.DialectOpenAI)
}

// handleAnthropicMessages serves /v1/messages in the Anthropic dialect.
func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, core.DialectAnthropic)
}

// handleOpenAIResponses serves /v1/responses in the OpenAI Responses dialect
// (Codex and Responses-native clients).
func (s *Server) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	s.handleChat(w, r, core.DialectOpenAIResponses)
}

// handleChat is the shared chat handler parameterized by the client dialect.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request, dialect core.Dialect) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)
	client := detectClient(r)

	s.consoleLog.Logf("DEBUG", "→ %s %s · dialect=%s · client=%s · key=%s",
		r.Method, r.URL.Path, dialect, client, key.ID)

	codec, err := s.codecs.Codec(dialect)
	if err != nil {
		s.consoleLog.Logf("ERROR", "unsupported dialect: %s", dialect)
		writeError(w, http.StatusInternalServerError, "unsupported dialect")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		s.consoleLog.Logf("ERROR", "failed to read body: %v", err)
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	s.consoleLog.Logf("DEBUG", "  body read: %d bytes", len(body))

	req, err := codec.ParseRequest(body)
	if err != nil {
		s.consoleLog.Logf("ERROR", "parse failed: %v", err)
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	// Attach routing metadata.
	req.Metadata = core.RequestMetadata{
		ClientKind:    client,
		SourceDialect: dialect,
		APIKeyID:      key.ID,
		TenantID:      tenantID,
		ProjectID:     key.ProjectID,
	}

	s.consoleLog.Logf("DEBUG", "  model=%s · stream=%v · messages=%d · tenant=%s",
		req.Model, req.Stream, len(req.Messages), tenantID)

	resolved, err := resolveTargets(r.Context(), s.chains, s.aliases, tenantID, req.Model)
	if err != nil {
		var bad badModelError
		if errors.As(err, &bad) {
			s.consoleLog.Logf("WARN", "bad model: %s", bad.Error())
			writeError(w, http.StatusBadRequest, bad.Error())
			return
		}
		s.consoleLog.Logf("ERROR", "resolve targets failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to resolve model")
		return
	}

	// Enforce per-key model access restrictions. Filter resolved targets to
	// only include models the key is allowed to access.
	if len(resolved.Targets) > 0 {
		filtered, ferr := s.filterAllowedTargets(r.Context(), key.ID, resolved.Targets)
		if ferr != nil {
			s.consoleLog.Logf("ERROR", "model access check failed: %v", ferr)
			writeError(w, http.StatusInternalServerError, "model access check failed")
			return
		}
		if len(filtered) == 0 {
			s.consoleLog.Logf("WARN", "model access denied: key=%s model=%s", key.ID, req.Model)
			writeError(w, http.StatusForbidden, "access denied: this API key is not permitted to use model "+req.Model)
			return
		}
		resolved.Targets = filtered
	}

	for i, t := range resolved.Targets {
		s.consoleLog.Logf("DEBUG", "  target[%d]: %s/%s", i, t.Provider, t.Model)
	}

	opts := pipeline.Options{
		Targets:  resolved.Targets,
		PlanOpts: s.endpointPlanOptions(r.Context(), resolved.PlanOpts),
		Slimmer:  s.slimmerConfig(),
		Terse:    s.terseConfig(),
		Caveman:  s.cavemanConfig(),
	}

	if req.Stream {
		s.consoleLog.Logf("DEBUG", "  entering stream path")
		s.streamChat(w, r, codec, req, opts)
		return
	}
	s.consoleLog.Logf("DEBUG", "  entering unary path")
	s.unaryChat(w, r, codec, req, opts)
}

// unaryChat runs a non-streaming request and renders the response.
func (s *Server) unaryChat(w http.ResponseWriter, r *http.Request, codec transform.Codec, req *core.ChatRequest, opts pipeline.Options) {
	start := time.Now()
	s.consoleLog.Logf("DEBUG", "  ▶ pipeline.Chat() start")
	result, err := s.pipeline.Chat(r.Context(), req, opts)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		s.consoleLog.Logf("ERROR", "  ✖ pipeline.Chat() failed after %dms: %v", latency, err)
		s.logRequest(req.Model, req.Model, 0, 0, latency, false, err)
		s.writeProviderError(w, err)
		return
	}

	out, err := codec.RenderResponse(result.Response)
	if err != nil {
		s.consoleLog.Logf("ERROR", "  ✖ render failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to render response")
		return
	}
	tokens := result.Response.Usage.PromptTokens + result.Response.Usage.CompletionTokens
	s.consoleLog.Logf("DEBUG", "  ✔ %s/%s · %d tok · acct=%s · cache=%v · %dms",
		result.Provider, result.Model, tokens, result.AccountID, result.CacheHit, latency)
	s.logRequest(result.Provider, result.Model, tokens, result.CostMicros, latency, result.CacheHit, nil)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-KeiRouter-Provider", result.Provider)
	w.Header().Set("X-KeiRouter-Model", result.Model)
	if result.CacheHit {
		w.Header().Set("X-KeiRouter-Cache", "hit")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out) // out is already a []byte from RenderResponse
}

// streamChat runs a streaming request and relays SSE events in the client's
// dialect, honoring client disconnects and the configured stall timeout.
func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, codec transform.Codec, req *core.ChatRequest, opts pipeline.Options) {
	streamCodec, ok := codec.(transform.StreamCodec)
	if !ok {
		writeError(w, http.StatusInternalServerError, "dialect does not support streaming")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported by server")
		return
	}

	start := time.Now()
	s.consoleLog.Logf("DEBUG", "  ▶ pipeline.Stream() start")
	result, err := s.pipeline.Stream(r.Context(), req, opts)
	if err != nil {
		latency := int(time.Since(start).Milliseconds())
		s.consoleLog.Logf("ERROR", "  ✖ pipeline.Stream() failed after %dms: %v", latency, err)
		s.logRequest(req.Model, req.Model, 0, 0, latency, false, err)
		s.writeProviderError(w, err)
		return
	}
	s.consoleLog.Logf("DEBUG", "  ✔ stream connected: %s/%s · acct=%s",
		result.Provider, result.Model, result.AccountID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-KeiRouter-Provider", result.Provider)
	w.Header().Set("X-KeiRouter-Model", result.Model)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Zero-copy direct pipe path: the pipeline detected same-dialect, no-tools
	// and obtained a raw io.ReadCloser from the upstream. Pipe it directly to
	// the client via io.Copy — no JSON parse/serialize, no goroutines, minimal
	// memory allocation. This is the fastest possible streaming path.
	if result.DirectBody != nil {
		defer result.DirectBody.Close()
		n, cpErr := io.Copy(w, result.DirectBody)
		if cpErr != nil && !isClientDisconnect(cpErr) {
			s.consoleLog.Logf("ERROR", "  ✖ direct pipe error after %d bytes: %v", n, cpErr)
			s.log.Warn("direct pipe error", "bytes", n, "err", cpErr)
		}
		flusher.Flush()
		// Record usage from the captured SSE stream. The pipeline wraps
		// the direct body in a tee reader that captures all bytes; the
		// DirectUsageFunc parses the captured data for usage tokens.
		if result.DirectUsageFunc != nil {
			result.DirectUsageFunc()
		}
		latency := int(time.Since(start).Milliseconds())
		s.consoleLog.Logf("DEBUG", "  ✔ direct pipe done: %d bytes · %dms", n, latency)
		s.logRequest(result.Provider, result.Model, 0, 0, latency, false, nil)
		return
	}

	// Wrap the response writer in a bufio.Writer to batch small SSE writes
	// into fewer syscalls. The pool avoids allocating a new writer per request.
	bw := core.SSEWriterPool.Get().(*bufio.Writer)
	defer core.SSEWriterPool.Put(bw)
	bw.Reset(w)

	state := &transform.StreamState{Model: result.Model}
	streamStart := time.Now()
	var totalTokens int
	var chunkCount int

	// ToolArgSanitizer buffers streaming tool call arguments and emits
	// sanitized JSON when each tool call completes. This fixes malformed
	// arguments from non-Anthropic models (e.g., Read.limit as string).
	sanitizer := transform.NewToolArgSanitizer()
	renderChunk := func(cleaned core.StreamChunk) {
		events, rerr := streamCodec.RenderStreamChunk(cleaned, state)
		if rerr != nil {
			s.log.Warn("failed to render stream chunk", "err", rerr)
			return
		}
		for _, ev := range events {
			if _, werr := bw.Write(ev); werr != nil {
				s.consoleLog.Logf("WARN", "  client disconnected after %d chunks", chunkCount)
				return
			}
		}
		// Flush the buffered writer to the underlying http.ResponseWriter,
		// then flush the HTTP flusher to push bytes to the client.
		bw.Flush()
		flusher.Flush()
	}

	for chunk := range result.Chunks {
		if chunk.Type == core.ChunkError {
			s.consoleLog.Logf("ERROR", "  ✖ stream chunk error: %v", chunk.Err)
			s.log.Warn("stream error", "err", chunk.Err)
			break
		}
		if chunk.Type == core.ChunkUsage && chunk.Usage != nil {
			totalTokens = chunk.Usage.PromptTokens + chunk.Usage.CompletionTokens
		}
		chunkCount++
		sanitizer.Process(chunk, renderChunk)
	}

	// Flush any remaining buffered tool calls.
	sanitizer.Flush(renderChunk)

	for _, ev := range streamCodec.RenderStreamDone(state) {
		_, _ = bw.Write(ev)
	}
	bw.Flush()
	flusher.Flush()

	latency := int(time.Since(streamStart).Milliseconds())
	s.consoleLog.Logf("DEBUG", "  ✔ stream done: %d chunks · %d tok · %dms", chunkCount, totalTokens, latency)
	s.logRequest(result.Provider, result.Model, totalTokens, 0, latency, false, nil)
}

// writeProviderError maps a structured provider error to an HTTP status.
func (s *Server) writeProviderError(w http.ResponseWriter, err error) {
	pe := core.AsProviderError(err)
	status := http.StatusBadGateway
	switch pe.Kind {
	case core.ErrBadRequest:
		status = http.StatusBadRequest
	case core.ErrAuth:
		status = http.StatusUnauthorized
	case core.ErrRateLimit:
		status = http.StatusTooManyRequests
	case core.ErrQuotaExhausted, core.ErrBudgetBlocked:
		status = http.StatusPaymentRequired
	case core.ErrTimeout:
		status = http.StatusGatewayTimeout
	case core.ErrInternal:
		status = http.StatusInternalServerError
	}
	writeError(w, status, pe.Message)
}

// isClientDisconnect reports whether an error is a client disconnection
// (broken pipe, reset by peer) rather than a server-side failure. These are
// expected during streaming and should not be logged as errors.
func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "reset by peer") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "use of closed network connection")
}

// filterAllowedTargets filters resolved routing targets to only include models
// the given API key is allowed to access. Returns empty slice if no target
// matches the key's model access policy.
func (s *Server) filterAllowedTargets(ctx context.Context, keyID string, targets []dispatch.Target) ([]dispatch.Target, error) {
	keys := s.identity.Keys()
	allowed, err := keys.GetAllowedModels(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return targets, nil // no restriction
	}

	var filtered []dispatch.Target
	for _, t := range targets {
		ok, err := keys.IsModelAllowed(ctx, keyID, t.Model)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// handleKeyUsage serves GET /v1/keys/me/usage — the authenticated API key
// owner can check their own token/cost usage and remaining budget.
func (s *Server) handleKeyUsage(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	ctx := r.Context()

	// Get budgets scoped to this key.
	budgets, err := s.budgets.ListByScope(ctx, store.ScopeAPIKey, key.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list budgets")
		return
	}

	type budgetOut struct {
		Period        string  `json:"period"`
		LimitTokens   int64   `json:"limit_tokens"`
		TokensUsed    int64   `json:"tokens_used"`
		TokensRemain  int64   `json:"tokens_remaining"`
		TokensPctUsed float64 `json:"tokens_pct_used"`
		LimitUSD      float64 `json:"limit_usd"`
		SpentUSD      float64 `json:"spent_usd"`
		USDRemaining  float64 `json:"usd_remaining"`
		USDUsed       float64 `json:"usd_pct_used"`
		Alert         bool    `json:"alert"`
	}

	var budgetOuts []budgetOut
	for _, b := range budgets {
		since := budget.PeriodStart(b.Period, time.Now())
		costMicros, tokens, err := s.usage.SpendAndTokens(ctx, b.ScopeKind, b.ScopeID, since)
		if err != nil {
			s.log.Error("key usage: spend lookup failed", "err", err)
			continue
		}

		bo := budgetOut{
			Period:      b.Period,
			LimitTokens: b.LimitTokens,
			TokensUsed:  tokens,
			LimitUSD:    float64(b.LimitMicros) / 1_000_000,
			SpentUSD:    float64(costMicros) / 1_000_000,
		}
		if b.LimitTokens > 0 {
			bo.TokensRemain = b.LimitTokens - tokens
			if bo.TokensRemain < 0 {
				bo.TokensRemain = 0
			}
			bo.TokensPctUsed = float64(tokens) / float64(b.LimitTokens) * 100
		}
		if b.LimitMicros > 0 {
			bo.USDRemaining = bo.LimitUSD - bo.SpentUSD
			if bo.USDRemaining < 0 {
				bo.USDRemaining = 0
			}
			bo.USDUsed = float64(costMicros) / float64(b.LimitMicros) * 100
		}
		// Alert if either threshold crossed.
		if b.AlertPct > 0 {
			if (b.LimitMicros > 0 && costMicros*100 >= b.LimitMicros*int64(b.AlertPct)) ||
				(b.LimitTokens > 0 && tokens*100 >= b.LimitTokens*int64(b.AlertPct)) {
				bo.Alert = true
			}
		}
		budgetOuts = append(budgetOuts, bo)
	}

	// Get allowed models for this key.
	allowedModels, err := s.identity.Keys().GetAllowedModels(ctx, key.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get model access")
		return
	}

	// Get current period summary.
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	summary, err := s.usage.Summarize(ctx, tenantOf(key), periodStart)
	if err != nil {
		s.log.Error("key usage: summarize failed", "err", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"key_id":         key.ID,
		"key_name":       key.Name,
		"budgets":        budgetOuts,
		"allowed_models": allowedModels,
		"current_period": map[string]any{
			"prompt_tokens":     summary.PromptTokens,
			"completion_tokens": summary.CompletionTokens,
			"total_requests":    summary.TotalRequests,
			"cost_usd":          float64(summary.CostMicros) / 1_000_000,
		},
	})
}

// detectClient identifies the calling tool from request headers, used for
// telemetry and client-specific quirks. Best-effort; empty when unknown.
func detectClient(r *http.Request) string {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	switch {
	case strings.Contains(ua, "claude"):
		return "claude-code"
	case strings.Contains(ua, "cursor"):
		return "cursor"
	case strings.Contains(ua, "codex"):
		return "codex"
	case r.Header.Get("x-stainless-lang") != "":
		return "openai-sdk"
	default:
		return ""
	}
}
