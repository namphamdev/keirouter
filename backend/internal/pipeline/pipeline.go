// Package pipeline orchestrates the request lifecycle: it applies token-saving
// transforms, enforces budgets, selects an account via the dispatcher, executes
// the upstream call with fallback, and records usage.
//
// It operates entirely on canonical core types; dialect translation happens at
// the gateway edge, before and after the pipeline. Both unary and streaming
// paths share the same candidate-selection and fallback logic.
package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/mydisha/keirouter/backend/internal/budget"
	"github.com/mydisha/keirouter/backend/internal/cache"
	"github.com/mydisha/keirouter/backend/internal/capability"
	"github.com/mydisha/keirouter/backend/internal/caveman"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/dispatch"
	"github.com/mydisha/keirouter/backend/internal/meter"
	"github.com/mydisha/keirouter/backend/internal/normalizer"
	"github.com/mydisha/keirouter/backend/internal/observ"
	"github.com/mydisha/keirouter/backend/internal/slimmer"
	"github.com/mydisha/keirouter/backend/internal/terse"
)

// Pipeline wires the request-processing stages together.
type Pipeline struct {
	dispatcher *dispatch.Dispatcher
	meter      *meter.Meter
	budget     *budget.Engine
	slimmer    *slimmer.Engine
	metrics    *observ.Metrics
	cache      *cache.Cache
	embedder   cache.Embedder
	log        *slog.Logger

	requestTimeout    time.Duration // upper bound on non-streaming upstream calls
	streamStallTimeout time.Duration // aborts a stream with no bytes for this long
}

// Deps bundles the pipeline's collaborators.
type Deps struct {
	Dispatcher *dispatch.Dispatcher
	Meter      *meter.Meter
	Budget     *budget.Engine
	Slimmer    *slimmer.Engine
	Metrics    *observ.Metrics
	Cache      *cache.Cache
	Embedder   cache.Embedder
	Logger     *slog.Logger

	// RequestTimeout bounds non-streaming upstream calls. Zero means no limit.
	RequestTimeout time.Duration
	// StreamStallTimeout aborts a stream that produces no bytes for this
	// long. Zero means no stall detection.
	StreamStallTimeout time.Duration
}

// New builds a Pipeline.
func New(d Deps) *Pipeline {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Pipeline{
		dispatcher: d.Dispatcher,
		meter:      d.Meter,
		budget:     d.Budget,
		slimmer:    d.Slimmer,
		metrics:    d.Metrics,
		cache:      d.Cache,
		embedder:   d.Embedder,
		log:        log,

		requestTimeout:    d.RequestTimeout,
		streamStallTimeout: d.StreamStallTimeout,
	}
}

// Options carries per-request routing and token-saving settings resolved from
// the API key, request, and global config.
type Options struct {
	// Targets is the ordered fallback chain (provider+model candidates).
	Targets []dispatch.Target
	// Slimmer / Terse / Caveman control token-saving transforms. Slimmer (RTK)
	// compresses bulky tool outputs on the input side; Terse and Caveman inject
	// system-prompt directives that reduce output tokens.
	Slimmer slimmer.Config
	Terse   terse.Config
	Caveman caveman.Config
}

// Result reports the outcome of a unary request for metering and audit.
type Result struct {
	Response   *core.ChatResponse
	Provider   string
	Model      string
	AccountID  string
	CostMicros int64
	Latency    time.Duration
	SlimStats  *slimmer.Stats
	CacheHit   bool
}

// Chat runs a non-streaming request through the full pipeline with fallback.
func (p *Pipeline) Chat(ctx context.Context, req *core.ChatRequest, opts Options) (*Result, error) {
	if err := p.preflight(ctx, req, opts); err != nil {
		p.log.Debug("preflight rejected", "err", err)
		return nil, err
	}

	// Semantic cache lookup before any token-saving transform mutates the
	// request, so cache keys are stable across slimmer/terse settings. Cached
	// hits cost nothing and skip the upstream entirely.
	vec := p.embedPrompt(ctx, req)
	if hit, ok := p.cacheLookup(ctx, vec); ok {
		p.log.Debug("cache hit, skipping upstream")
		// Record cache hit for usage visibility.
		if p.meter != nil {
			p.meter.Record(ctx, meter.Event{
				TenantID:  req.Metadata.TenantID,
				ProjectID: req.Metadata.ProjectID,
				APIKeyID:  req.Metadata.APIKeyID,
				Provider:  "cache",
				Model:     hit.Model,
				CacheHit:  true,
			})
		}
		if p.metrics != nil {
			p.metrics.RecordCache(true)
		}
		return &Result{Response: hit, Provider: "cache", Model: hit.Model, CacheHit: true}, nil
	}
	if p.metrics != nil && p.cache != nil && p.cache.Enabled() {
		p.metrics.RecordCache(false)
	}

	slimStats := p.applyTokenSaving(req, opts)
	if slimStats != nil {
		p.log.Debug("slimmer stats", "saved", slimStats.Saved(), "hits", len(slimStats.Hits))
	}
	save := buildSaveState(slimStats, opts)

	required := capability.Required(req)
	attempts, err := p.dispatcher.Plan(ctx, req.Metadata.TenantID, opts.Targets, required)
	if err != nil {
		p.log.Debug("dispatcher plan failed", "err", err)
		return nil, err
	}
	p.log.Debug("dispatcher planned", "attempts", len(attempts), "required", required)

	var lastErr error
	for i, attempt := range attempts {
		started := time.Now()
		p.log.Debug("attempt start", "i", i, "provider", attempt.Target.Provider,
			"model", attempt.Target.Model, "account", attempt.Account.ID)
		attemptReq := cloneForAttempt(req, attempt.Target.Model)

		// Inject proxy config from credentials into context so the connector's
		// HTTP client uses the right proxy/relay for this account.
		callCtx := core.WithProxy(ctx, attempt.Creds)
		if p.requestTimeout > 0 {
			var cancel context.CancelFunc
			callCtx, cancel = context.WithTimeout(callCtx, p.requestTimeout)
			defer cancel()
		}
		resp, callErr := attempt.Conn.Chat(callCtx, attemptReq, attempt.Creds)
		latency := time.Since(started)

		if callErr != nil {
			pe := core.AsProviderError(callErr)
			lastErr = pe
			p.dispatcher.NoteFailure(ctx, attempt.Account.ID, pe)
			if p.metrics != nil {
				p.metrics.RecordUpstreamError(attempt.Target.Provider, string(pe.Kind))
			}
			if !pe.Fallbackable() {
				p.log.Debug("attempt not fallbackable, aborting", "kind", pe.Kind)
				return nil, pe
			}
			if p.metrics != nil {
				p.metrics.RecordFallback(string(pe.Kind))
			}
			p.log.Warn("chat attempt failed, falling back",
				"provider", attempt.Target.Provider, "model", attempt.Target.Model, "kind", pe.Kind)
			continue
		}

		cost := p.record(ctx, req.Metadata, attempt, resp.Usage, false, latency, save)
		p.cacheStore(ctx, vec, resp)
		p.log.Debug("attempt success", "provider", attempt.Target.Provider,
			"model", attempt.Target.Model, "latency_ms", latency.Milliseconds(),
			"prompt_tok", resp.Usage.PromptTokens, "completion_tok", resp.Usage.CompletionTokens)
		return &Result{
			Response:   resp,
			Provider:   attempt.Target.Provider,
			Model:      attempt.Target.Model,
			AccountID:  attempt.Account.ID,
			CostMicros: cost,
			Latency:    latency,
			SlimStats:  slimStats,
		}, nil
	}

	if lastErr == nil {
		lastErr = &core.ProviderError{Kind: core.ErrInternal, Message: "pipeline: no attempts executed"}
	}
	return nil, lastErr
}

// StreamResult is delivered before chunks start flowing, identifying the chosen
// target so the gateway can set response headers.
type StreamResult struct {
	Chunks    <-chan core.StreamChunk
	Provider  string
	Model     string
	AccountID string
}

// Stream runs a streaming request with fallback. Fallback applies only to the
// connection-establishment phase; once the first attempt's channel is returned,
// errors surface as ChunkError on that channel. Usage metering happens in a
// goroutine that observes the final usage chunk.
func (p *Pipeline) Stream(ctx context.Context, req *core.ChatRequest, opts Options) (*StreamResult, error) {
	if err := p.preflight(ctx, req, opts); err != nil {
		p.log.Debug("stream preflight rejected", "err", err)
		return nil, err
	}
	slimStats := p.applyTokenSaving(req, opts)
	save := buildSaveState(slimStats, opts)

	required := capability.Required(req)
	attempts, err := p.dispatcher.Plan(ctx, req.Metadata.TenantID, opts.Targets, required)
	if err != nil {
		p.log.Debug("stream dispatcher plan failed", "err", err)
		return nil, err
	}
	p.log.Debug("stream dispatcher planned", "attempts", len(attempts))

	var lastErr error
	for i, attempt := range attempts {
		attemptReq := cloneForAttempt(req, attempt.Target.Model)
		started := time.Now()
		p.log.Debug("stream attempt start", "i", i, "provider", attempt.Target.Provider,
			"model", attempt.Target.Model, "account", attempt.Account.ID)

		// Capture TTFT from the connector's first-chunk callback.
		var ttft time.Duration
		streamCfg := core.StreamConfig{
			OnFirstChunk: func(elapsed time.Duration) {
				ttft = elapsed
			},
		}

		callCtx := core.WithProxy(ctx, attempt.Creds)
		upstream, callErr := attempt.Conn.Stream(callCtx, attemptReq, attempt.Creds, streamCfg)
		if callErr != nil {
			pe := core.AsProviderError(callErr)
			lastErr = pe
			p.dispatcher.NoteFailure(ctx, attempt.Account.ID, pe)
			if p.metrics != nil {
				p.metrics.RecordUpstreamError(attempt.Target.Provider, string(pe.Kind))
			}
			if !pe.Fallbackable() {
				p.log.Debug("stream attempt not fallbackable, aborting", "kind", pe.Kind)
				return nil, pe
			}
			if p.metrics != nil {
				p.metrics.RecordFallback(string(pe.Kind))
			}
			p.log.Warn("stream attempt failed, falling back",
				"provider", attempt.Target.Provider, "model", attempt.Target.Model, "kind", pe.Kind)
			continue
		}

		p.log.Debug("stream connected", "provider", attempt.Target.Provider,
			"model", attempt.Target.Model, "account", attempt.Account.ID)

		// Tee the upstream channel so we can meter terminal usage without
		// blocking the client consumer.
		out := make(chan core.StreamChunk, 16)
		meta := req.Metadata
		acc := attempt
		go p.pumpStream(ctx, upstream, out, meta, acc, started, &ttft, save)

		return &StreamResult{
			Chunks:    out,
			Provider:  attempt.Target.Provider,
			Model:     attempt.Target.Model,
			AccountID: attempt.Account.ID,
		}, nil
	}

	if lastErr == nil {
		lastErr = &core.ProviderError{Kind: core.ErrInternal, Message: "pipeline: no attempts executed"}
	}
	return nil, lastErr
}

// pumpStream forwards chunks to the client channel while capturing usage for
// metering when the stream completes. Usage chunks are merged rather than
// replaced: Anthropic streams input tokens in message_start and output tokens
// in message_delta as separate events — replacing would lose the first.
//
// When StreamStallTimeout is configured, a timer resets on every chunk. If no
// chunk arrives within the timeout, the stream is cancelled with ErrTimeout.
func (p *Pipeline) pumpStream(ctx context.Context, in <-chan core.StreamChunk, out chan<- core.StreamChunk,
	meta core.RequestMetadata, attempt dispatch.Attempt, started time.Time, ttft *time.Duration, save *saveState) {
	defer close(out)

	// Set up stall detection. The timer resets every time a chunk arrives.
	var stallCancel context.CancelFunc
	stallCtx := ctx
	if p.streamStallTimeout > 0 {
		stallCtx, stallCancel = context.WithCancel(ctx)
		defer stallCancel()
		// Drain upstream when stall fires.
		go func() {
			<-stallCtx.Done()
			if ctx.Err() == nil && stallCtx.Err() != nil {
				// Stall detected (parent ctx still alive but stall ctx cancelled).
				// Consume remaining upstream chunks to unblock the producer.
				for range in {
				}
			}
		}()
	}

	var stallTimer *time.Timer
	resetStall := func() {
		if p.streamStallTimeout <= 0 {
			return
		}
		if stallTimer == nil {
			stallTimer = time.AfterFunc(p.streamStallTimeout, stallCancel)
		} else {
			stallTimer.Reset(p.streamStallTimeout)
		}
	}
	stopStall := func() {
		if stallTimer != nil {
			stallTimer.Stop()
		}
	}
	defer stopStall()

	resetStall() // arm the timer for the initial connection

	var usage core.Usage
	for {
		select {
		case chunk, ok := <-in:
			if !ok {
				// Upstream closed — stream complete.
				p.recordWithTTFT(ctx, meta, attempt, usage, false, time.Since(started), *ttft, save)
				return
			}
			resetStall()
			if chunk.Type == core.ChunkUsage && chunk.Usage != nil {
				usage = mergeUsage(usage, *chunk.Usage)
			}
			select {
			case out <- chunk:
			case <-stallCtx.Done():
				// Client disconnected or stall — drain upstream.
				for range in {
				}
				return
			}
		case <-stallCtx.Done():
			if ctx.Err() != nil {
				// Parent context cancelled (client disconnected).
				for range in {
				}
				return
			}
			// Stall timeout fired.
			stopStall()
			out <- core.StreamChunk{
				Type: core.ChunkError,
				Err:  &core.ProviderError{Kind: core.ErrTimeout, Provider: attempt.Target.Provider, Model: attempt.Target.Model, Message: "stream stall: no data received for " + p.streamStallTimeout.String()},
			}
			// Drain upstream.
			for range in {
			}
			p.recordWithTTFT(ctx, meta, attempt, usage, false, time.Since(started), *ttft, save)
			return
		}
	}
}

// mergeUsage combines two usage snapshots. Fields present in the new snapshot
// (non-zero) overwrite the old; zero fields preserve the old value. This
// handles providers that split usage across multiple events (e.g. Anthropic
// sends input tokens in message_start and output tokens in message_delta).
func mergeUsage(old, new core.Usage) core.Usage {
	if new.PromptTokens != 0 {
		old.PromptTokens = new.PromptTokens
	}
	if new.CompletionTokens != 0 {
		old.CompletionTokens = new.CompletionTokens
	}
	if new.TotalTokens != 0 {
		old.TotalTokens = new.TotalTokens
	}
	if new.CachedTokens != 0 {
		old.CachedTokens = new.CachedTokens
	}
	if new.CacheWriteTokens != 0 {
		old.CacheWriteTokens = new.CacheWriteTokens
	}
	if new.ReasoningTokens != 0 {
		old.ReasoningTokens = new.ReasoningTokens
	}
	return old
}

// preflight runs validation and the budget guard before any upstream call.
func (p *Pipeline) preflight(ctx context.Context, req *core.ChatRequest, opts Options) error {
	if len(opts.Targets) == 0 {
		return &core.ProviderError{Kind: core.ErrBadRequest, Message: "no routing targets resolved for model"}
	}
	if p.budget != nil {
		scope := budget.Scope{
			TenantID:  req.Metadata.TenantID,
			ProjectID: req.Metadata.ProjectID,
			APIKeyID:  req.Metadata.APIKeyID,
		}
		if err := p.budget.CheckOrError(ctx, scope); err != nil {
			return err
		}
	}
	return nil
}

// applyTokenSaving runs the input-side (slimmer/RTK) and output-side
// (terse, caveman) token-saving transforms in place. Terse and caveman both
// inject system-prompt directives; if both are enabled, terse runs first and
// caveman appends after, but in practice only one output-saver is used.
func (p *Pipeline) applyTokenSaving(req *core.ChatRequest, opts Options) *slimmer.Stats {
	// Normalize tool call IDs and fix missing tool results before any
	// downstream processing. This ensures Anthropic-compatible IDs and
	// complete tool_use/tool_result pairs.
	normalizer.Apply(req)

	var stats *slimmer.Stats
	if p.slimmer != nil && opts.Slimmer.Enabled {
		stats = p.slimmer.Compress(req, opts.Slimmer)
		if stats != nil {
			p.log.Debug("slimmer compressed request", "saved_bytes", stats.Saved(), "hits", len(stats.Hits))
		}
	}
	terse.Apply(req, opts.Terse)
	caveman.Apply(req, opts.Caveman)
	return stats
}

// saveState captures which token-saving features were active and their results
// for a single request, so the meter can persist them.
type saveState struct {
	slimSnap *meter.SlimSnapshot
	caveman  bool
	terse    bool
}

// splitRuleNames splits a comma-separated rule string into individual names.
func splitRuleNames(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := s[start:i]; part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// buildSaveState converts pipeline-level token-saving results into a snapshot
// suitable for the meter. Returns nil when no features were active.
func buildSaveState(stats *slimmer.Stats, opts Options) *saveState {
	save := &saveState{
		caveman: opts.Caveman.Enabled,
		terse:   opts.Terse.Enabled,
	}
	if stats != nil && stats.Saved() > 0 {
		// Build comma-separated rule names from hits.
		rules := make(map[string]struct{})
		for _, h := range stats.Hits {
			rules[h.Rule] = struct{}{}
		}
		var ruleNames string
		for name := range rules {
			if ruleNames != "" {
				ruleNames += ","
			}
			ruleNames += name
		}
		save.slimSnap = &meter.SlimSnapshot{
			BytesSaved:  stats.Saved(),
			TokensSaved: stats.Saved() / 4, // rough char-to-token estimate
			Rules:       ruleNames,
		}
	}
	if save.slimSnap == nil && !save.caveman && !save.terse {
		return nil
	}
	return save
}

// record meters a completed attempt; failures to record are logged, not fatal.
func (p *Pipeline) record(ctx context.Context, meta core.RequestMetadata, attempt dispatch.Attempt,
	usage core.Usage, cacheHit bool, latency time.Duration, save *saveState) int64 {
	return p.recordWithTTFT(ctx, meta, attempt, usage, cacheHit, latency, 0, save)
}

// recordWithTTFT is like record but also records time-to-first-token.
func (p *Pipeline) recordWithTTFT(ctx context.Context, meta core.RequestMetadata, attempt dispatch.Attempt,
	usage core.Usage, cacheHit bool, latency, ttft time.Duration, save *saveState) int64 {
	if p.meter == nil {
		return 0
	}
	ev := meter.Event{
		TenantID:  meta.TenantID,
		ProjectID: meta.ProjectID,
		APIKeyID:  meta.APIKeyID,
		Provider:  attempt.Target.Provider,
		Model:     attempt.Target.Model,
		AccountID: attempt.Account.ID,
		Usage:     usage,
		CacheHit:  cacheHit,
		Latency:   latency,
		TTFT:      ttft,
	}
	if save != nil {
		ev.SlimStats = save.slimSnap
		ev.CavemanActive = save.caveman
		ev.TerseActive = save.terse
	}
	cost, err := p.meter.Record(ctx, ev)
	if err != nil {
		p.log.Error("failed to record usage", "err", err)
	}

	if p.metrics != nil {
		p.metrics.RecordRequest(
			attempt.Target.Provider, attempt.Target.Model, "success",
			latency.Seconds(),
			usage.PromptTokens, usage.CompletionTokens, usage.CachedTokens, cost,
			ttft.Seconds(),
		)
		if cacheHit {
			p.metrics.RecordCache(true)
		}
		// Record token-saving metrics.
		if save != nil {
			if save.slimSnap != nil {
				for _, rule := range splitRuleNames(save.slimSnap.Rules) {
					p.metrics.RecordSlimSavings(rule, save.slimSnap.BytesSaved)
				}
			}
			if save.caveman {
				p.metrics.RecordCavemanActivation()
			}
			if save.terse {
				p.metrics.RecordTerseActivation()
			}
		}
	}
	return cost
}

// embedPrompt computes a cache key vector for the request, or nil when caching
// is disabled or no embedder is configured.
func (p *Pipeline) embedPrompt(ctx context.Context, req *core.ChatRequest) []float32 {
	if p.cache == nil || !p.cache.Enabled() || p.embedder == nil {
		return nil
	}
	vec, err := p.embedder.Embed(ctx, cache.PromptText(req))
	if err != nil {
		p.log.Warn("cache embed failed; skipping cache", "err", err)
		return nil
	}
	return vec
}

// cacheLookup checks the semantic cache for a stored response.
func (p *Pipeline) cacheLookup(ctx context.Context, vec []float32) (*core.ChatResponse, bool) {
	if p.cache == nil || len(vec) == 0 {
		return nil, false
	}
	resp, ok, err := p.cache.Lookup(ctx, vec)
	if err != nil {
		p.log.Warn("cache lookup failed", "err", err)
		return nil, false
	}
	return resp, ok
}

// cacheStore caches a successful response under its prompt vector.
func (p *Pipeline) cacheStore(ctx context.Context, vec []float32, resp *core.ChatResponse) {
	if p.cache == nil || len(vec) == 0 {
		return
	}
	if err := p.cache.Store(ctx, vec, resp); err != nil {
		p.log.Warn("cache store failed", "err", err)
	}
}

// cloneForAttempt produces a shallow copy of the request with the candidate's
// model id, so each fallback attempt targets the right model without mutating
// the shared request.
func cloneForAttempt(req *core.ChatRequest, model string) *core.ChatRequest {
	clone := *req
	clone.Model = model
	return &clone
}
