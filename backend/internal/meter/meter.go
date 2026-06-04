// Package meter records per-request usage and computes cost.
//
// Cost is stored in micros (millionths of a USD) as an integer to avoid
// floating-point drift in budget accounting. The pricing table maps
// provider/model to its per-million-token rates; unknown models fall back to
// provider-level rates, then to zero (treated as free for display purposes).
package meter

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/usagehub"
)

// Price holds per-million-token rates in USD.
type Price struct {
	InputPerM        float64 // standard input tokens
	OutputPerM       float64 // standard output tokens
	CachedInputPerM  float64 // cache-read input tokens (often 50-90% off standard)
	CacheWritePerM   float64 // cache-write input tokens (often 25% above standard)
	ReasoningPerM    float64 // reasoning/extended-thinking output tokens
}

// Meter records usage rows and computes cost from a pricing table.
type Meter struct {
	usage       *store.UsageRepo
	pricing     map[string]Price   // provider-level fallback
	modelPrices map[string]Price   // provider/model-level (e.g. "openai/gpt-4o")
	hub         *usagehub.Hub      // notifies subscribers of new usage records
}

// SetHub installs a usage event hub. When set, the meter publishes an event
// after every successful Record call. This enables SSE-based near-real-time
// dashboard updates.
func (m *Meter) SetHub(h *usagehub.Hub) { m.hub = h }

// New builds a Meter backed by a usage repo and pricing tables.
func New(usage *store.UsageRepo, pricing map[string]Price, modelPrices map[string]Price) *Meter {
	if pricing == nil {
		pricing = map[string]Price{}
	}
	if modelPrices == nil {
		modelPrices = map[string]Price{}
	}
	return &Meter{usage: usage, pricing: pricing, modelPrices: modelPrices}
}

// Event captures the facts about one completed (or cached) request.
type Event struct {
	TenantID  string
	ProjectID string
	APIKeyID  string
	Provider  string
	Model     string
	AccountID string
	Usage     core.Usage
	CacheHit  bool
	Latency   time.Duration
	TTFT      time.Duration // time-to-first-token (0 if not measured)

	// Token-saving analytics.
	SlimStats    *SlimSnapshot // nil when RTK did not fire
	CavemanActive bool         // caveman output compression was active
	TerseActive   bool         // terse output compression was active
}

// SlimSnapshot captures the RTK slimmer's per-request compression results.
type SlimSnapshot struct {
	BytesSaved  int
	TokensSaved int
	Rules       string // comma-separated rule names that fired
}

// resolvePrice looks up the price for a provider/model pair. It tries
// "provider/model" first, then "provider", then returns a zero price.
func (m *Meter) resolvePrice(provider, model string) Price {
	// Try exact model match first.
	if model != "" {
		if p, ok := m.modelPrices[provider+"/"+model]; ok {
			return p
		}
		// Also try provider/model without provider prefix (e.g. "gpt-4o" not "openai/gpt-4o").
		for key, p := range m.modelPrices {
			if len(key) > len(provider)+1 && key[:len(provider)+1] == provider+"/" {
				suffix := key[len(provider)+1:]
				if suffix == model {
					return p
				}
			}
		}
	}
	// Fall back to provider-level.
	if p, ok := m.pricing[provider]; ok {
		return p
	}
	return Price{}
}

// CostMicros returns the cost of a usage event in micros of USD. Cached
// requests cost nothing (the whole point of the cache). Cached read tokens
// use the discounted CachedInputPerM rate; cache write tokens use the
// (often higher) CacheWritePerM rate. Reasoning tokens use the ReasoningPerM
// rate when set.
func (m *Meter) CostMicros(provider, model string, u core.Usage, cacheHit bool) int64 {
	if cacheHit {
		return 0
	}
	p := m.resolvePrice(provider, model)

	// Standard input tokens = total input minus cache reads and writes.
	standardInput := u.PromptTokens - u.CachedTokens - u.CacheWriteTokens
	if standardInput < 0 {
		standardInput = 0
	}

	// Cost breakdown:
	//   standard input     * InputPerM
	// + cache-read input   * CachedInputPerM  (often 50-90% off InputPerM)
	// + cache-write input  * CacheWritePerM   (often 25% above InputPerM)
	// + reasoning tokens   * ReasoningPerM    (often same as OutputPerM)
	// + completion tokens  * OutputPerM
	inputCost := float64(standardInput) * p.InputPerM
	cachedReadCost := float64(u.CachedTokens) * p.CachedInputPerM
	cacheWriteCost := float64(u.CacheWriteTokens) * p.CacheWritePerM
	reasoningCost := float64(u.ReasoningTokens) * p.ReasoningPerM
	outputCost := float64(u.CompletionTokens) * p.OutputPerM

	return int64(inputCost + cachedReadCost + cacheWriteCost + reasoningCost + outputCost)
}

// Record persists a usage row for an event and returns the computed cost.
func (m *Meter) Record(ctx context.Context, ev Event) (int64, error) {
	cost := m.CostMicros(ev.Provider, ev.Model, ev.Usage, ev.CacheHit)
	rec := store.UsageRecord{
		ID:               uuid.NewString(),
		TenantID:         ev.TenantID,
		ProjectID:        ev.ProjectID,
		APIKeyID:         ev.APIKeyID,
		Provider:         ev.Provider,
		Model:            ev.Model,
		AccountID:        ev.AccountID,
		PromptTokens:     ev.Usage.PromptTokens,
		CompletionTokens: ev.Usage.CompletionTokens,
		CachedTokens:     ev.Usage.CachedTokens,
		CacheWriteTokens: ev.Usage.CacheWriteTokens,
		CostMicros:       cost,
		CacheHit:         ev.CacheHit,
		LatencyMS:        int(ev.Latency.Milliseconds()),
		TTFTMS:           int(ev.TTFT.Milliseconds()),
		CavemanActive:    ev.CavemanActive,
		TerseActive:      ev.TerseActive,
		CreatedAt:        time.Now(),
	}
	if ev.SlimStats != nil {
		rec.SlimBytesSaved = ev.SlimStats.BytesSaved
		rec.SlimTokensSaved = ev.SlimStats.TokensSaved
		rec.SlimRules = ev.SlimStats.Rules
	}
	if err := m.usage.Record(ctx, rec); err != nil {
		return cost, err
	}
	// Notify SSE subscribers of the new usage record for near-real-time
	// dashboard updates.
	if m.hub != nil {
		m.hub.Publish(usagehub.Event{
			Provider:  ev.Provider,
			Model:     ev.Model,
			AccountID: ev.AccountID,
			Tokens:    ev.Usage.PromptTokens + ev.Usage.CompletionTokens,
		})
	}
	return cost, nil
}

// PricingFromCatalog builds a provider-level pricing table from provider specs.
func PricingFromCatalog(specs []SpecPrice) map[string]Price {
	out := make(map[string]Price, len(specs))
	for _, s := range specs {
		out[s.ID] = Price{InputPerM: s.InputPerM, OutputPerM: s.OutputPerM}
	}
	return out
}

// SpecPrice is the minimal pricing projection of a provider spec.
type SpecPrice struct {
	ID         string
	InputPerM  float64
	OutputPerM float64
}
