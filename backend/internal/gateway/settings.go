package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mydisha/keirouter/backend/internal/caveman"
	"github.com/mydisha/keirouter/backend/internal/dispatch"
	"github.com/mydisha/keirouter/backend/internal/slimmer"
	"github.com/mydisha/keirouter/backend/internal/terse"
)

// endpointSettingsKey is the settings-store key under which token-saving
// preferences are persisted as a JSON blob (mirrors 9router's endpoint
// settings: RTK token saver + caveman output compression + terse mode).
const endpointSettingsKey = "endpoint_settings"

// EndpointSettings holds the dashboard-configurable token-saving preferences.
type EndpointSettings struct {
	// RTKEnabled toggles input-side tool-output compression (the slimmer).
	RTKEnabled bool `json:"rtk_enabled"`

	// CavemanEnabled toggles output-side caveman compression; CavemanLevel is
	// one of "lite", "full", "ultra".
	CavemanEnabled bool   `json:"caveman_enabled"`
	CavemanLevel   string `json:"caveman_level"`

	// TerseEnabled toggles terse mode, which serializes messages and tools into
	// the compact TERSE format for token-efficient context.
	TerseEnabled bool   `json:"terse_enabled"`
	TerseLevel   string `json:"terse_level"`

	// Routing strategy fields (mirrors 9router).
	RoutingStrategy  string `json:"routing_strategy"`   // "fill-first" | "round-robin"
	StickyLimit      int    `json:"sticky_limit"`       // calls per account before switching
	ComboStrategy    string `json:"combo_strategy"`     // "fallback" | "round-robin"
	ComboStickyLimit int    `json:"combo_sticky_limit"` // calls per combo model before switching

	// Outbound proxy settings.
	OutboundProxyEnabled bool   `json:"outbound_proxy_enabled"`
	OutboundProxyURL     string `json:"outbound_proxy_url"`
	OutboundNoProxy      string `json:"outbound_no_proxy"`

	// Observability.
	ObservabilityEnabled *bool `json:"observability_enabled,omitempty"`
}

// defaultEndpointSettings mirrors 9router's defaults: RTK on, caveman/terse off.
func defaultEndpointSettings() EndpointSettings {
	return EndpointSettings{
		RTKEnabled:           true,
		CavemanEnabled:       false,
		CavemanLevel:         string(caveman.LevelFull),
		TerseEnabled:         false,
		TerseLevel:           "medium",
		RoutingStrategy:      "fill-first",
		StickyLimit:          3,
		ComboStrategy:        "fallback",
		ComboStickyLimit:     1,
		OutboundProxyEnabled: false,
		OutboundProxyURL:     "",
		OutboundNoProxy:      "",
	}
}

// loadEndpointSettings reads the persisted settings, falling back to defaults
// when unset or unreadable. It never errors: token-saving is best-effort.
func (s *Server) loadEndpointSettings(ctx context.Context) EndpointSettings {
	def := defaultEndpointSettings()
	if s.settings == nil {
		return def
	}
	raw, err := s.settings.Get(ctx, endpointSettingsKey)
	if err != nil || raw == "" {
		return def
	}
	var es EndpointSettings
	if err := json.Unmarshal([]byte(raw), &es); err != nil {
		return def
	}
	// Backfill empty levels with defaults.
	if es.CavemanLevel == "" {
		es.CavemanLevel = def.CavemanLevel
	}
	if es.TerseLevel == "" {
		es.TerseLevel = def.TerseLevel
	}
	if es.RoutingStrategy == "" {
		es.RoutingStrategy = def.RoutingStrategy
	} else if normalized, ok := normalizeAccountRoutingStrategy(es.RoutingStrategy); ok {
		es.RoutingStrategy = normalized
	} else {
		es.RoutingStrategy = def.RoutingStrategy
	}
	if es.StickyLimit == 0 {
		es.StickyLimit = def.StickyLimit
	}
	if es.ComboStrategy == "" {
		es.ComboStrategy = def.ComboStrategy
	} else if normalized, ok := normalizeComboRoutingStrategy(es.ComboStrategy); ok {
		es.ComboStrategy = normalized
	} else {
		es.ComboStrategy = def.ComboStrategy
	}
	if es.ComboStickyLimit == 0 {
		es.ComboStickyLimit = def.ComboStickyLimit
	}
	return es
}

// slimmerConfig resolves the slimmer (RTK) settings from endpoint settings.
func (s *Server) slimmerConfig() slimmer.Config {
	es := s.loadEndpointSettings(context.Background())
	return slimmer.Config{Enabled: es.RTKEnabled}
}

// terseConfig resolves terse-mode settings from endpoint settings.
func (s *Server) terseConfig() terse.Config {
	es := s.loadEndpointSettings(context.Background())
	return terse.Config{Enabled: es.TerseEnabled}
}

// cavemanConfig resolves caveman settings from endpoint settings.
func (s *Server) cavemanConfig() caveman.Config {
	es := s.loadEndpointSettings(context.Background())
	return caveman.Config{Enabled: es.CavemanEnabled, Level: caveman.Level(es.CavemanLevel)}
}

// ---- admin endpoints --------------------------------------------------------

// adminGetEndpointSettings returns the current token-saving preferences.
func (s *Server) adminGetEndpointSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.loadEndpointSettings(r.Context()))
}

// adminUpdateEndpointSettings persists token-saving preferences. It accepts a
// partial body and merges it over the current settings, validating enum values.
func (s *Server) adminUpdateEndpointSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings store not configured")
		return
	}
	current := s.loadEndpointSettings(r.Context())

	var patch struct {
		RTKEnabled           *bool   `json:"rtk_enabled"`
		CavemanEnabled       *bool   `json:"caveman_enabled"`
		CavemanLevel         *string `json:"caveman_level"`
		TerseEnabled         *bool   `json:"terse_enabled"`
		TerseLevel           *string `json:"terse_level"`
		RoutingStrategy      *string `json:"routing_strategy"`
		StickyLimit          *int    `json:"sticky_limit"`
		ComboStrategy        *string `json:"combo_strategy"`
		ComboStickyLimit     *int    `json:"combo_sticky_limit"`
		OutboundProxyEnabled *bool   `json:"outbound_proxy_enabled"`
		OutboundProxyURL     *string `json:"outbound_proxy_url"`
		OutboundNoProxy      *string `json:"outbound_no_proxy"`
		ObservabilityEnabled *bool   `json:"observability_enabled"`
	}
	if !decodeJSON(w, r, &patch) {
		return
	}

	if patch.RTKEnabled != nil {
		current.RTKEnabled = *patch.RTKEnabled
	}
	if patch.CavemanEnabled != nil {
		current.CavemanEnabled = *patch.CavemanEnabled
	}
	if patch.CavemanLevel != nil {
		if !caveman.ValidLevel(caveman.Level(*patch.CavemanLevel)) {
			writeError(w, http.StatusBadRequest, "caveman_level must be lite, full, or ultra")
			return
		}
		current.CavemanLevel = *patch.CavemanLevel
	}
	if patch.TerseEnabled != nil {
		current.TerseEnabled = *patch.TerseEnabled
	}
	if patch.TerseLevel != nil {
		current.TerseLevel = *patch.TerseLevel
	}
	if patch.RoutingStrategy != nil {
		normalized, ok := normalizeAccountRoutingStrategy(*patch.RoutingStrategy)
		if !ok {
			writeError(w, http.StatusBadRequest, "routing_strategy must be fill-first or round-robin")
			return
		}
		current.RoutingStrategy = normalized
	}
	if patch.StickyLimit != nil {
		if *patch.StickyLimit < 1 {
			writeError(w, http.StatusBadRequest, "sticky_limit must be at least 1")
			return
		}
		current.StickyLimit = *patch.StickyLimit
	}
	if patch.ComboStrategy != nil {
		normalized, ok := normalizeComboRoutingStrategy(*patch.ComboStrategy)
		if !ok {
			writeError(w, http.StatusBadRequest, "combo_strategy must be fallback or round-robin")
			return
		}
		current.ComboStrategy = normalized
	}
	if patch.ComboStickyLimit != nil {
		if *patch.ComboStickyLimit < 1 {
			writeError(w, http.StatusBadRequest, "combo_sticky_limit must be at least 1")
			return
		}
		current.ComboStickyLimit = *patch.ComboStickyLimit
	}
	if patch.OutboundProxyEnabled != nil {
		current.OutboundProxyEnabled = *patch.OutboundProxyEnabled
	}
	if patch.OutboundProxyURL != nil {
		current.OutboundProxyURL = *patch.OutboundProxyURL
	}
	if patch.OutboundNoProxy != nil {
		current.OutboundNoProxy = *patch.OutboundNoProxy
	}
	if patch.ObservabilityEnabled != nil {
		current.ObservabilityEnabled = patch.ObservabilityEnabled
	}

	// Enforce mutual exclusion: caveman and terse both inject system-prompt
	// directives that conflict, so only one can be active. The last toggle
	// set wins (frontend sends both fields; the one being enabled is the
	// "winner"). As a safety net for direct API callers, if both end up
	// enabled, disable the one that wasn't explicitly set in this patch.
	if current.CavemanEnabled && current.TerseEnabled {
		if patch.TerseEnabled != nil && *patch.TerseEnabled {
			current.CavemanEnabled = false
		} else {
			current.TerseEnabled = false
		}
	}

	raw, err := json.Marshal(current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.settings.Set(r.Context(), endpointSettingsKey, string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) endpointPlanOptions(ctx context.Context, opts dispatch.PlanOptions) dispatch.PlanOptions {
	es := s.loadEndpointSettings(ctx)
	if es.RoutingStrategy == string(dispatch.StrategyRoundRobin) {
		opts.AccountStrategy = dispatch.StrategyRoundRobin
		opts.AccountStickyLimit = es.StickyLimit
	}
	if opts.Strategy == dispatch.StrategyRoundRobin || es.ComboStrategy == string(dispatch.StrategyRoundRobin) {
		opts.Strategy = dispatch.StrategyRoundRobin
		opts.StickyLimit = es.ComboStickyLimit
	}
	return opts
}

func normalizeAccountRoutingStrategy(raw string) (string, bool) {
	switch normalizeStrategyToken(raw) {
	case "", "fill-first", "fill_first", "priority", "fallback":
		return "fill-first", true
	case "round-robin", "round_robin", "roundrobin":
		return string(dispatch.StrategyRoundRobin), true
	default:
		return "", false
	}
}

func normalizeComboRoutingStrategy(raw string) (string, bool) {
	switch normalizeStrategyToken(raw) {
	case "", "fallback", "priority", "fill-first", "fill_first":
		return string(dispatch.StrategyFallback), true
	case "round-robin", "round_robin", "roundrobin":
		return string(dispatch.StrategyRoundRobin), true
	default:
		return "", false
	}
}

func normalizeStrategyToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// ---- access settings --------------------------------------------------------

// accessSettingsKey is the settings-store key for the Endpoints page access
// options (local access, secure tunnel, Tailscale), mirroring 9router.
const accessSettingsKey = "access_settings"

// AccessSettings holds the endpoint reachability toggles. These are dashboard
// preferences; the actual tunnels are provisioned out-of-band, so the toggles
// record intent and surface the primary endpoint URL.
type AccessSettings struct {
	LocalEnabled  bool   `json:"local_enabled"`
	TunnelEnabled bool   `json:"tunnel_enabled"`
	Tailscale     bool   `json:"tailscale_enabled"`
	TunnelURL     string `json:"tunnel_url,omitempty"`
	TailscaleURL  string `json:"tailscale_url,omitempty"`
}

func defaultAccessSettings() AccessSettings {
	return AccessSettings{LocalEnabled: true, TunnelEnabled: false, Tailscale: false}
}

func (s *Server) loadAccessSettings(ctx context.Context) AccessSettings {
	def := defaultAccessSettings()
	if s.settings == nil {
		return def
	}
	raw, err := s.settings.Get(ctx, accessSettingsKey)
	if err != nil || raw == "" {
		return def
	}
	var as AccessSettings
	if err := json.Unmarshal([]byte(raw), &as); err != nil {
		return def
	}
	return as
}

// adminGetAccessSettings returns the access options plus the primary endpoint
// URL derived from the request, so the dashboard can render the Endpoints page.
func (s *Server) adminGetAccessSettings(w http.ResponseWriter, r *http.Request) {
	as := s.loadAccessSettings(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"local_enabled":     as.LocalEnabled,
		"tunnel_enabled":    as.TunnelEnabled,
		"tailscale_enabled": as.Tailscale,
		"tunnel_url":        as.TunnelURL,
		"tailscale_url":     as.TailscaleURL,
		"endpoint_url":      s.publicBaseURL(r) + "/v1",
	})
}

// adminUpdateAccessSettings persists the access option toggles, merging a
// partial body over the current settings.
func (s *Server) adminUpdateAccessSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusInternalServerError, "settings store not configured")
		return
	}
	current := s.loadAccessSettings(r.Context())

	var patch struct {
		LocalEnabled  *bool `json:"local_enabled"`
		TunnelEnabled *bool `json:"tunnel_enabled"`
		Tailscale     *bool `json:"tailscale_enabled"`
	}
	if !decodeJSON(w, r, &patch) {
		return
	}
	if patch.LocalEnabled != nil {
		current.LocalEnabled = *patch.LocalEnabled
	}
	if patch.TunnelEnabled != nil {
		current.TunnelEnabled = *patch.TunnelEnabled
	}
	if patch.Tailscale != nil {
		current.Tailscale = *patch.Tailscale
	}

	raw, err := json.Marshal(current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.settings.Set(r.Context(), accessSettingsKey, string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"local_enabled":     current.LocalEnabled,
		"tunnel_enabled":    current.TunnelEnabled,
		"tailscale_enabled": current.Tailscale,
		"endpoint_url":      s.publicBaseURL(r) + "/v1",
	})
}
