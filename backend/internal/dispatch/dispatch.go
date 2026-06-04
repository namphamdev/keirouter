// Package dispatch selects which provider account serves a request and, on
// failure, advances through fallback candidates.
//
// A Target names a provider+model. The dispatcher resolves the live accounts
// for a target's provider, skips accounts on cooldown or lacking the required
// capabilities, and tries them in priority order. When every account for a
// target is exhausted, it advances to the next target in the chain. Errors that
// are not fallbackable (e.g. a malformed request) short-circuit immediately.
//
// Strategy variants:
//   - fallback (default): try targets sequentially until one succeeds.
//   - round-robin: rotate the starting target on each call so load spreads
//     evenly across models. A "sticky limit" controls how many consecutive
//     requests land on the same target before advancing the cursor.
package dispatch

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mydisha/keirouter/backend/internal/capability"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/proxy"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

// Exponential backoff constants (mirrors 9router's accountFallback logic).
const (
	// BackoffBase is the base cooldown duration at backoff level 1.
	BackoffBase = 2 * time.Second
	// BackoffMax caps the maximum cooldown produced by exponential backoff.
	BackoffMax = 5 * time.Minute
	// BackoffMaxLevel is the ceiling for the backoff exponent.
	BackoffMaxLevel = 15
	// TransientCooldown is the default cooldown for transient/upstream errors
	// that have no explicit Retry-After.
	TransientCooldown = 30 * time.Second
	// ModelCooldownMultiplier scales the per-model cooldown relative to the
	// account-level cooldown (same duration — model locks are independent).
	ModelCooldownMultiplier = 1
	// DefaultStickyLimit is the number of consecutive requests served by one
	// target before round-robin advances to the next.
	DefaultStickyLimit = 1
)

// Target is one candidate in a fallback chain.
type Target struct {
	Provider string
	Model    string
}

// Attempt describes a single resolved try: the connector, credentials, and the
// account it came from. The pipeline executes attempts via the connector.
type Attempt struct {
	Target  Target
	Conn    core.Connector
	Creds   core.Credentials
	Account store.Account
}

// Strategy controls how targets within a chain are ordered.
type Strategy string

const (
	// StrategyFallback tries targets in declared order (the default).
	StrategyFallback Strategy = "fallback"
	// StrategyRoundRobin rotates the starting target per call.
	StrategyRoundRobin Strategy = "round-robin"
)

// ConnectorSource resolves a connector by provider id.
type ConnectorSource interface {
	Get(provider string) (core.Connector, error)
}

// TokenRefresher refreshes an account's OAuth access token just-in-time when it
// is expired or about to expire. It is optional; a nil refresher means accounts
// are used as-is. The oauth.TokenManager implements this.
type TokenRefresher interface {
	EnsureFresh(ctx context.Context, acc store.Account) (store.Account, error)
}

// RoutingSource provides model-level cooldowns and chain rotation state.
type RoutingSource interface {
	SetModelCooldown(ctx context.Context, accountID, model string, until time.Time) error
	ClearModelCooldown(ctx context.Context, accountID, model string) error
	IsModelCooldownActive(ctx context.Context, accountID, model string) (bool, error)
	GetChainRotationState(ctx context.Context, chainID string) (store.ChainRotation, error)
	SetChainRotationState(ctx context.Context, state store.ChainRotation) error
	GetTargetRotationState(ctx context.Context, scopeKey string) (store.TargetRotation, error)
	SetTargetRotationState(ctx context.Context, state store.TargetRotation) error
}

// Dispatcher walks fallback chains, yielding resolved attempts.
type Dispatcher struct {
	conns     ConnectorSource
	accounts  *store.AccountRepo
	vault     *vault.Vault
	pools     proxy.PoolSource
	refresher TokenRefresher
	routing   RoutingSource
	// defaultCooldown is applied to an account when an error carries no
	// upstream-specified Retry-After.
	defaultCooldown time.Duration
}

// New builds a Dispatcher.
func New(conns ConnectorSource, accounts *store.AccountRepo, v *vault.Vault) *Dispatcher {
	return &Dispatcher{
		conns:           conns,
		accounts:        accounts,
		vault:           v,
		defaultCooldown: 60 * time.Second,
	}
}

// SetTokenRefresher installs an OAuth token refresher, consulted before opening
// each account's credentials.
func (d *Dispatcher) SetTokenRefresher(r TokenRefresher) { d.refresher = r }

// SetPoolSource installs a proxy pool resolver, consulted when an account has a
// proxy_pool_id binding.
func (d *Dispatcher) SetPoolSource(p proxy.PoolSource) { d.pools = p }

// SetRoutingSource installs the model-cooldown and chain-rotation backend.
func (d *Dispatcher) SetRoutingSource(r RoutingSource) { d.routing = r }

// PlanOptions carries per-request strategy configuration.
type PlanOptions struct {
	// Strategy is "fallback" (default) or "round-robin".
	Strategy Strategy
	// ChainID is the persisted chain identifier, used by round-robin to
	// store/retrieve the rotation cursor. Empty for inline targets.
	ChainID string
	// StickyLimit is the number of consecutive requests per target before
	// round-robin advances. Zero defaults to DefaultStickyLimit.
	StickyLimit int
	// AccountStrategy controls how accounts inside one provider/model target
	// are ordered. "fallback" keeps priority order; "round-robin" rotates the
	// starting account while preserving cooldown/fallback behavior.
	AccountStrategy Strategy
	// AccountStickyLimit is the number of consecutive requests per account
	// before account round-robin advances. Zero defaults to DefaultStickyLimit.
	AccountStickyLimit int
}

// Plan resolves the ordered list of attempts for a chain of targets, scoped to
// a tenant and constrained to the given required capabilities. It returns an
// error only when no attempt could be resolved at all (no usable account for
// any target); otherwise the pipeline tries attempts in order.
func (d *Dispatcher) Plan(ctx context.Context, tenantID string, targets []Target, required core.CapabilitySet) ([]Attempt, error) {
	return d.PlanWith(ctx, tenantID, targets, required, PlanOptions{})
}

// PlanWith is like Plan but accepts strategy options.
func (d *Dispatcher) PlanWith(ctx context.Context, tenantID string, targets []Target, required core.CapabilitySet, opts PlanOptions) ([]Attempt, error) {
	// Apply round-robin rotation if requested.
	ordered := d.applyRotation(ctx, targets, opts)

	now := time.Now()
	var attempts []Attempt
	var lastReason string

	for _, target := range ordered {
		// Capability guard: never fall back to a model that cannot honor the
		// request. This prevents silent quality downgrades.
		if !capability.Supports(target.Model, required) {
			lastReason = fmt.Sprintf("model %q lacks required capabilities", target.Model)
			continue
		}

		conn, err := d.conns.Get(target.Provider)
		if err != nil {
			lastReason = err.Error()
			continue
		}

		accs, err := d.accounts.ListByProvider(ctx, tenantID, target.Provider)
		if err != nil {
			return nil, fmt.Errorf("dispatch: list accounts for %s: %w", target.Provider, err)
		}
		if len(accs) == 0 {
			lastReason = fmt.Sprintf("no accounts configured for provider %q", target.Provider)
			continue
		}
		accs = d.applyAccountRotation(ctx, tenantID, target, accs, opts)

		for _, acc := range accs {
			// Account-level cooldown (global cooldown from NoteFailure).
			if acc.CooldownUntil != nil && acc.CooldownUntil.After(now) {
				lastReason = fmt.Sprintf("account %s on cooldown", acc.ID)
				continue
			}
			// Model-level cooldown: skip this account only for this model.
			if d.routing != nil {
				locked, _ := d.routing.IsModelCooldownActive(ctx, acc.ID, target.Model)
				if locked {
					lastReason = fmt.Sprintf("account %s model %s on cooldown", acc.ID, target.Model)
					continue
				}
			}
			// Refresh an expiring OAuth access token before use, so the
			// connector always receives a live token. A refresh failure skips
			// this account and falls back to the next.
			if d.refresher != nil {
				refreshed, rerr := d.refresher.EnsureFresh(ctx, acc)
				if rerr != nil {
					lastReason = rerr.Error()
					continue
				}
				acc = refreshed
			}
			creds, err := d.vault.Open(acc)
			if err != nil {
				lastReason = err.Error()
				continue
			}
			// Resolve proxy pool binding for this account.
			if d.pools != nil && acc.ProxyPoolID != "" {
				if perr := proxy.ResolvePool(ctx, d.pools, acc.ProxyPoolID, &creds); perr != nil {
					lastReason = perr.Error()
					continue
				}
			}
			attempts = append(attempts, Attempt{
				Target:  target,
				Conn:    conn,
				Creds:   creds,
				Account: acc,
			})
		}
	}

	if len(attempts) == 0 {
		if lastReason == "" {
			lastReason = "no usable targets in chain"
		}
		return nil, &core.ProviderError{Kind: core.ErrInternal, Message: "dispatch: " + lastReason}
	}
	return attempts, nil
}

// NoteFailure applies cooldowns to an account (and optionally a model) based on
// a provider error. Exponential backoff increases the cooldown on repeated
// failures for rate-limit / quota errors.
func (d *Dispatcher) NoteFailure(ctx context.Context, accountID string, err *core.ProviderError) {
	if err == nil {
		return
	}

	var cooldown time.Duration
	switch err.Kind {
	case core.ErrRateLimit:
		cooldown = d.exponentialCooldown(ctx, accountID)
	case core.ErrQuotaExhausted:
		cooldown = 30 * time.Minute
		if err.RetryAfter > 0 {
			cooldown = err.RetryAfter
		}
	case core.ErrAuth:
		cooldown = 5 * time.Minute
	case core.ErrUpstream, core.ErrTimeout:
		// Transient errors: apply a short cooldown so the account gets a
		// breather without being locked out for too long.
		cooldown = TransientCooldown
	default:
		return
	}

	if err.RetryAfter > 0 && err.Kind != core.ErrRateLimit {
		cooldown = err.RetryAfter
	}

	_ = d.accounts.SetCooldown(ctx, accountID, time.Now().Add(cooldown))

	// Also set a model-level cooldown when a model is specified, so other
	// models on the same account remain available.
	if d.routing != nil && err.Model != "" {
		modelCooldown := time.Duration(int64(cooldown) * ModelCooldownMultiplier)
		_ = d.routing.SetModelCooldown(ctx, accountID, err.Model, time.Now().Add(modelCooldown))
	}
}

// NoteSuccess resets the backoff level for an account and clears any model
// cooldown. Called by the pipeline after a successful upstream response.
func (d *Dispatcher) NoteSuccess(ctx context.Context, accountID, model string) {
	_ = d.accounts.ResetBackoffLevel(ctx, accountID)
	if d.routing != nil && model != "" {
		_ = d.routing.ClearModelCooldown(ctx, accountID, model)
	}
}

// exponentialCooldown computes the cooldown duration using exponential backoff.
// Level 1: 2s, Level 2: 4s, Level 3: 8s... up to BackoffMax (5min).
func (d *Dispatcher) exponentialCooldown(ctx context.Context, accountID string) time.Duration {
	// Try to read current backoff level from the account.
	acc, err := d.accounts.Get(ctx, accountID)
	if err != nil {
		return d.defaultCooldown
	}

	newLevel := acc.BackoffLevel + 1
	if newLevel > BackoffMaxLevel {
		newLevel = BackoffMaxLevel
	}

	// Persist the new backoff level.
	_ = d.accounts.SetBackoffLevel(ctx, accountID, newLevel)

	cooldown := time.Duration(float64(BackoffBase) * math.Pow(2, float64(newLevel-1)))
	if cooldown > BackoffMax {
		cooldown = BackoffMax
	}
	return cooldown
}

// applyRotation reorders targets according to the round-robin strategy.
// For "fallback" (or when routing is not configured), targets are returned
// as-is. For "round-robin", the persisted cursor is advanced and the targets
// are rotated so the cursor index comes first.
func (d *Dispatcher) applyRotation(ctx context.Context, targets []Target, opts PlanOptions) []Target {
	if opts.Strategy != StrategyRoundRobin || len(targets) <= 1 || d.routing == nil || opts.ChainID == "" {
		return targets
	}

	sticky := opts.StickyLimit
	if sticky <= 0 {
		sticky = DefaultStickyLimit
	}
	state, _ := d.routing.GetChainRotationState(ctx, opts.ChainID)
	cursor, nextCursor, nextHitCount := advanceRotationState(len(targets), state.LastIndex, state.HitCount, sticky)

	rotated := make([]Target, len(targets))
	for i := range targets {
		rotated[i] = targets[(cursor+i)%len(targets)]
	}

	_ = d.routing.SetChainRotationState(ctx, store.ChainRotation{
		ChainID:   opts.ChainID,
		LastIndex: nextCursor,
		HitCount:  nextHitCount,
	})

	return rotated
}

// applyAccountRotation reorders accounts within one target according to the
// account round-robin strategy. The persisted key is tenant/provider/model so
// direct model routes and combo steps share fair account distribution.
func (d *Dispatcher) applyAccountRotation(ctx context.Context, tenantID string, target Target, accounts []store.Account, opts PlanOptions) []store.Account {
	if opts.AccountStrategy != StrategyRoundRobin || len(accounts) <= 1 || d.routing == nil {
		return accounts
	}

	sticky := opts.AccountStickyLimit
	if sticky <= 0 {
		sticky = DefaultStickyLimit
	}
	scopeKey := accountRotationKey(tenantID, target)
	state, _ := d.routing.GetTargetRotationState(ctx, scopeKey)
	cursor, nextCursor, nextHitCount := advanceRotationState(len(accounts), state.LastIndex, state.HitCount, sticky)

	rotated := make([]store.Account, len(accounts))
	for i := range accounts {
		rotated[i] = accounts[(cursor+i)%len(accounts)]
	}

	_ = d.routing.SetTargetRotationState(ctx, store.TargetRotation{
		ScopeKey:  scopeKey,
		LastIndex: nextCursor,
		HitCount:  nextHitCount,
	})
	return rotated
}

func accountRotationKey(tenantID string, target Target) string {
	return tenantID + "\x00" + target.Provider + "\x00" + target.Model
}

// advanceRotationState returns the cursor to use for this request, plus the
// cursor/hit-count state to persist for the next request. lastIndex is the next
// starting index.
func advanceRotationState(length, lastIndex, hitCount, stickyLimit int) (cursor int, nextCursor int, nextHitCount int) {
	if length <= 0 {
		return 0, 0, 0
	}
	if stickyLimit <= 0 {
		stickyLimit = DefaultStickyLimit
	}
	cursor = lastIndex % length
	if cursor < 0 {
		cursor += length
	}
	nextCursor = cursor
	nextHitCount = hitCount + 1
	if nextHitCount >= stickyLimit {
		nextCursor = (cursor + 1) % length
		nextHitCount = 0
	}
	return cursor, nextCursor, nextHitCount
}

// TargetsFromChain flattens a stored chain into ordered targets.
func TargetsFromChain(chain store.Chain) []Target {
	out := make([]Target, 0, len(chain.Steps))
	for _, s := range chain.Steps {
		out = append(out, Target{Provider: s.Provider, Model: s.Model})
	}
	return out
}
