package gateway

import (
	"context"
	"strings"

	"github.com/mydisha/keirouter/backend/internal/dispatch"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// ChainSource resolves a named chain for a tenant.
type ChainSource interface {
	ListByTenant(ctx context.Context, tenantID string) ([]store.Chain, error)
}

// AliasSource resolves a model alias to a provider/model target.
type AliasSource interface {
	Get(ctx context.Context, alias string) (store.ModelAlias, error)
}

// resolveTargets turns an inbound model string into an ordered fallback chain.
//
// Four forms are supported, in priority order:
//   - "provider/model"  -> a single explicit target (e.g. "openai/gpt-4o").
//     Slashes beyond the first are kept in the model id so vendor-namespaced
//     ids like "anthropic/claude-3.5" via openrouter still work.
//   - "chain:name"       -> the named routing chain's steps.
//   - bare "name"        -> resolved as a chain named "name" if one exists,
//     then as a model alias. A bare name is never assumed to be a provider
//     model; routing stays explicit and predictable.
func resolveTargets(ctx context.Context, chains ChainSource, aliases AliasSource, tenantID, model string) ([]dispatch.Target, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errBadModel("model is required")
	}

	// chain:<name>
	if name, ok := strings.CutPrefix(model, "chain:"); ok {
		return chainTargets(ctx, chains, tenantID, name)
	}

	// provider/model
	if provider, rest, ok := strings.Cut(model, "/"); ok && provider != "" && rest != "" {
		return []dispatch.Target{{Provider: provider, Model: rest}}, nil
	}

	// bare name -> try a chain first
	targets, err := chainTargets(ctx, chains, tenantID, model)
	if err == nil {
		return targets, nil
	}

	// bare name -> try an alias
	if aliases != nil {
		alias, aerr := aliases.Get(ctx, model)
		if aerr == nil && alias.Target != "" {
			if provider, rest, ok := strings.Cut(alias.Target, "/"); ok && provider != "" && rest != "" {
				return []dispatch.Target{{Provider: provider, Model: rest}}, nil
			}
		}
	}

	return nil, errBadModel("model must be 'provider/model', a chain name, or an alias: " + model)
}

func chainTargets(ctx context.Context, chains ChainSource, tenantID, name string) ([]dispatch.Target, error) {
	list, err := chains.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		if c.Name == name {
			targets := dispatch.TargetsFromChain(c)
			if len(targets) == 0 {
				return nil, errBadModel("chain has no steps: " + name)
			}
			return targets, nil
		}
	}
	return nil, errBadModel("no chain named " + name)
}

// badModelError signals an unresolvable model string (a client error).
type badModelError struct{ msg string }

func (e badModelError) Error() string { return e.msg }

func errBadModel(msg string) error { return badModelError{msg: msg} }