package store

import (
	"context"
	"fmt"
	"time"
)

// UsageRepo records and aggregates per-request metering data.
type UsageRepo struct{ db *DB }

// Usage returns the usage repository.
func (db *DB) Usage() *UsageRepo { return &UsageRepo{db: db} }

// Record inserts a usage row for a completed request.
func (r *UsageRepo) Record(ctx context.Context, u UsageRecord) error {
	q := r.db.rebind(`
		INSERT INTO usage_records
			(id, tenant_id, project_id, api_key_id, provider, model, account_id,
			 prompt_tokens, completion_tokens, cached_tokens, cache_write_tokens,
			 cost_micros, cache_hit, latency_ms, ttft_ms,
			 slim_bytes_saved, slim_tokens_saved, slim_rules, caveman_active, terse_active,
			 created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.sql.ExecContext(ctx, q,
		u.ID, u.TenantID, nullString(u.ProjectID), nullString(u.APIKeyID),
		u.Provider, u.Model, nullString(u.AccountID),
		u.PromptTokens, u.CompletionTokens, u.CachedTokens, u.CacheWriteTokens,
		u.CostMicros, boolToInt(u.CacheHit), u.LatencyMS, u.TTFTMS,
		u.SlimBytesSaved, u.SlimTokensSaved, u.SlimRules, boolToInt(u.CavemanActive), boolToInt(u.TerseActive),
		formatTime(u.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: record usage: %w", err)
	}
	return nil
}

// SpendSince returns total cost in micros for a budget scope since the given
// time. Used by the budget engine to enforce hard limits.
func (r *UsageRepo) SpendSince(ctx context.Context, scope BudgetScope, scopeID string, since time.Time) (int64, error) {
	var column string
	switch scope {
	case ScopeTenant:
		column = "tenant_id"
	case ScopeProject:
		column = "project_id"
	case ScopeAPIKey:
		column = "api_key_id"
	default:
		return 0, fmt.Errorf("store: unknown budget scope %q", scope)
	}

	q := r.db.rebind(fmt.Sprintf(
		`SELECT COALESCE(SUM(cost_micros), 0) FROM usage_records WHERE %s = ? AND created_at >= ?`,
		column))
	var total int64
	if err := r.db.sql.QueryRowContext(ctx, q, scopeID, formatTime(since)).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: spend since: %w", err)
	}
	return total, nil
}

// Summary aggregates usage for a tenant over a time window.
type Summary struct {
	TotalRequests    int64
	PromptTokens     int64
	CompletionTokens int64
	CachedTokens     int64
	CacheWriteTokens int64
	CostMicros       int64
	CacheHits        int64
	AvgTTFTMS        int64 // average time-to-first-token for streaming requests
	AvgLatencyMS     int64 // average latency for non-cache requests
	SuccessCount     int64 // requests that succeeded (latency > 0 or cache hit)
	SlimBytesSaved   int64 // total bytes saved by RTK slimmer
	SlimTokensSaved  int64 // total estimated tokens saved by RTK
	CavemanRequests  int64 // requests where caveman was active
	TerseRequests    int64 // requests where terse was active
}

// Summarize returns aggregate usage for a tenant since the given time.
func (r *UsageRepo) Summarize(ctx context.Context, tenantID string, since time.Time) (Summary, error) {
	q := r.db.rebind(`
		SELECT
			COUNT(*),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0),
			COALESCE(SUM(cost_micros), 0),
			COALESCE(SUM(cache_hit), 0),
			COALESCE(CAST(AVG(CASE WHEN ttft_ms > 0 THEN ttft_ms END) AS INTEGER), 0),
			COALESCE(CAST(AVG(CASE WHEN latency_ms > 0 THEN latency_ms END) AS INTEGER), 0),
			COUNT(CASE WHEN latency_ms > 0 OR cache_hit = 1 THEN 1 END),
			COALESCE(SUM(slim_bytes_saved), 0),
			COALESCE(SUM(slim_tokens_saved), 0),
			COALESCE(SUM(caveman_active), 0),
			COALESCE(SUM(terse_active), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?`)
	var s Summary
	err := r.db.sql.QueryRowContext(ctx, q, tenantID, formatTime(since)).Scan(
		&s.TotalRequests, &s.PromptTokens, &s.CompletionTokens,
		&s.CachedTokens, &s.CacheWriteTokens, &s.CostMicros, &s.CacheHits, &s.AvgTTFTMS,
		&s.AvgLatencyMS, &s.SuccessCount,
		&s.SlimBytesSaved, &s.SlimTokensSaved, &s.CavemanRequests, &s.TerseRequests)
	if err != nil {
		return Summary{}, fmt.Errorf("store: summarize usage: %w", err)
	}
	return s, nil
}

// ProviderUsage aggregates usage for a single upstream provider. It powers the
// routing-activity map and per-provider breakdown on the Usage page.
type ProviderUsage struct {
	Provider         string
	TotalRequests    int64
	PromptTokens     int64
	CompletionTokens int64
	CostMicros       int64
}

// Breakdown returns per-provider aggregate usage for a tenant since the given
// time, ordered by request volume (busiest first).
func (r *UsageRepo) Breakdown(ctx context.Context, tenantID string, since time.Time) ([]ProviderUsage, error) {
	q := r.db.rebind(`
		SELECT
			provider,
			COUNT(*),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(cost_micros), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
		GROUP BY provider
		ORDER BY COUNT(*) DESC`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: usage breakdown: %w", err)
	}
	defer rows.Close()

	var out []ProviderUsage
	for rows.Next() {
		var p ProviderUsage
		if err := rows.Scan(&p.Provider, &p.TotalRequests, &p.PromptTokens, &p.CompletionTokens, &p.CostMicros); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecentRecord is a single recent request row for the activity feed and console.
type RecentRecord struct {
	ID               string
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	CacheWriteTokens int
	CostMicros       int64
	CacheHit         bool
	LatencyMS        int
	TTFTMS           int    // time-to-first-token in ms (0 for non-streaming)
	SlimBytesSaved   int    // bytes saved by RTK slimmer
	SlimTokensSaved  int    // estimated tokens saved by RTK
	SlimRules        string // comma-separated rule names that fired
	CavemanActive    bool
	TerseActive      bool
	CreatedAt        time.Time
}

// Recent returns the most recent usage records for a tenant, newest first.
func (r *UsageRepo) Recent(ctx context.Context, tenantID string, limit int) ([]RecentRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := r.db.rebind(`
		SELECT id, provider, model, prompt_tokens, completion_tokens, cached_tokens,
		       cache_write_tokens, cost_micros, cache_hit, latency_ms, ttft_ms,
		       slim_bytes_saved, slim_tokens_saved, slim_rules, caveman_active, terse_active,
		       created_at
		FROM usage_records
		WHERE tenant_id = ?
		ORDER BY created_at DESC
		LIMIT ?`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: recent usage: %w", err)
	}
	defer rows.Close()

	var out []RecentRecord
	for rows.Next() {
		var (
			rec                   RecentRecord
			cacheHit, caveman, terse int
			createdAt             string
		)
		if err := rows.Scan(&rec.ID, &rec.Provider, &rec.Model, &rec.PromptTokens,
			&rec.CompletionTokens, &rec.CachedTokens, &rec.CacheWriteTokens,
			&rec.CostMicros, &cacheHit, &rec.LatencyMS, &rec.TTFTMS,
			&rec.SlimBytesSaved, &rec.SlimTokensSaved, &rec.SlimRules, &caveman, &terse,
			&createdAt); err != nil {
			return nil, err
		}
		rec.CacheHit = cacheHit != 0
		rec.CavemanActive = caveman != 0
		rec.TerseActive = terse != 0
		rec.CreatedAt = parseTime(createdAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// AccountUsage aggregates usage keyed by upstream account. It powers the quota
// tracker, which shows how much each connected account has consumed.
type AccountUsage struct {
	AccountID        string
	TotalRequests    int64
	PromptTokens     int64
	CompletionTokens int64
	CachedTokens     int64
	CacheWriteTokens int64
	CostMicros       int64
}

// ByAccount returns per-account aggregate usage for a tenant since the given
// time. Records with no associated account are grouped under an empty id.
func (r *UsageRepo) ByAccount(ctx context.Context, tenantID string, since time.Time) ([]AccountUsage, error) {
	q := r.db.rebind(`
		SELECT
			COALESCE(account_id, ''),
			COUNT(*),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(cache_write_tokens), 0),
			COALESCE(SUM(cost_micros), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
		GROUP BY account_id`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: usage by account: %w", err)
	}
	defer rows.Close()

	var out []AccountUsage
	for rows.Next() {
		var a AccountUsage
		if err := rows.Scan(&a.AccountID, &a.TotalRequests, &a.PromptTokens,
			&a.CompletionTokens, &a.CachedTokens, &a.CacheWriteTokens, &a.CostMicros); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ModelUsage aggregates usage for a single provider+model pair. It powers the
// per-model breakdown table on the Usage page.
type ModelUsage struct {
	Provider         string
	Model            string
	TotalRequests    int64
	PromptTokens     int64
	CompletionTokens int64
	CostMicros       int64
}

// ByModel returns per-model aggregate usage for a tenant since the given time,
// ordered by request volume (busiest first).
func (r *UsageRepo) ByModel(ctx context.Context, tenantID string, since time.Time) ([]ModelUsage, error) {
	q := r.db.rebind(`
		SELECT
			provider,
			model,
			COUNT(*),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(cost_micros), 0)
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
		GROUP BY provider, model
		ORDER BY COUNT(*) DESC`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: usage by model: %w", err)
	}
	defer rows.Close()

	var out []ModelUsage
	for rows.Next() {
		var m ModelUsage
		if err := rows.Scan(&m.Provider, &m.Model, &m.TotalRequests, &m.PromptTokens, &m.CompletionTokens, &m.CostMicros); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// TimePoint is the created_at + cost of a single record, used to build the
// activity-over-time series in the handler (bucketed in Go for engine
// portability).
type TimePoint struct {
	CreatedAt  time.Time
	CostMicros int64
}

// Timeline returns the (created_at, cost) of records for a tenant since the
// given time, oldest first, capped to a sane limit for a local dashboard.
func (r *UsageRepo) Timeline(ctx context.Context, tenantID string, since time.Time) ([]TimePoint, error) {
	q := r.db.rebind(`
		SELECT created_at, cost_micros
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ?
		ORDER BY created_at ASC
		LIMIT 20000`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: usage timeline: %w", err)
	}
	defer rows.Close()

	var out []TimePoint
	for rows.Next() {
		var (
			tp        TimePoint
			createdAt string
		)
		if err := rows.Scan(&createdAt, &tp.CostMicros); err != nil {
			return nil, err
		}
		tp.CreatedAt = parseTime(createdAt)
		out = append(out, tp)
	}
	return out, rows.Err()
}

// RuleSavings aggregates savings per RTK slimmer rule.
type RuleSavings struct {
	Rule       string
	Count      int64 // number of requests where this rule fired
	BytesSaved int64 // total bytes saved by this rule
}

// SavingsByRule returns per-rule RTK savings aggregated from the slim_rules
// column. Since rules are stored as comma-separated strings, we parse them in
// Go rather than in SQL for engine portability.
func (r *UsageRepo) SavingsByRule(ctx context.Context, tenantID string, since time.Time) ([]RuleSavings, error) {
	q := r.db.rebind(`
		SELECT slim_rules, slim_bytes_saved
		FROM usage_records
		WHERE tenant_id = ? AND created_at >= ? AND slim_rules != ''`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: savings by rule: %w", err)
	}
	defer rows.Close()

	totals := map[string]*RuleSavings{}
	for rows.Next() {
		var rules string
		var bytesSaved int64
		if err := rows.Scan(&rules, &bytesSaved); err != nil {
			return nil, err
		}
		parts := splitRules(rules)
		if len(parts) == 0 {
			continue
		}
		perRule := bytesSaved / int64(len(parts))
		for _, name := range parts {
			if _, ok := totals[name]; !ok {
				totals[name] = &RuleSavings{Rule: name}
			}
			totals[name].Count++
			totals[name].BytesSaved += perRule
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]RuleSavings, 0, len(totals))
	for _, rs := range totals {
		out = append(out, *rs)
	}
	// Sort by bytes saved descending.
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].BytesSaved > out[i].BytesSaved {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// splitRules splits a comma-separated rule string into individual names,
// trimming whitespace and skipping empties.
func splitRules(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			// trim inline
			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}
			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}
