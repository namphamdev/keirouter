package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// RoutingRepo handles model-level cooldowns and chain rotation state.
type RoutingRepo struct{ db *DB }

// Routing returns the routing repository.
func (db *DB) Routing() *RoutingRepo { return &RoutingRepo{db: db} }

// Model cooldowns -----------------------------------------------------------

// SetModelCooldown inserts or updates a model-level cooldown on an account.
// While active, the dispatch layer skips this account for the given model
// but still allows it for other models.
func (r *RoutingRepo) SetModelCooldown(ctx context.Context, accountID, model string, until time.Time) error {
	id := randHex(16)
	q := r.db.rebind(`INSERT INTO model_cooldowns (id, account_id, model, cooldown_until, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(account_id, model) DO UPDATE SET cooldown_until = excluded.cooldown_until`)
	_, err := r.db.sql.ExecContext(ctx, q, id, accountID, model, formatTime(until), formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set model cooldown: %w", err)
	}
	return nil
}

// ClearModelCooldown removes a model cooldown (on success).
func (r *RoutingRepo) ClearModelCooldown(ctx context.Context, accountID, model string) error {
	q := r.db.rebind(`DELETE FROM model_cooldowns WHERE account_id = ? AND model = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, accountID, model)
	return err
}

// ClearAccountModelCooldowns removes all model cooldowns for an account.
func (r *RoutingRepo) ClearAccountModelCooldowns(ctx context.Context, accountID string) error {
	q := r.db.rebind(`DELETE FROM model_cooldowns WHERE account_id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, accountID)
	return err
}

// IsModelCooldownActive checks if a specific model on an account is still
// locked. Returns true when the cooldown has not yet expired.
func (r *RoutingRepo) IsModelCooldownActive(ctx context.Context, accountID, model string) (bool, error) {
	q := r.db.rebind(`SELECT cooldown_until FROM model_cooldowns
		WHERE account_id = ? AND model = ?
		UNION ALL
		SELECT cooldown_until FROM model_cooldowns
		WHERE account_id = ? AND model = '__all__'`)
	rows, err := r.db.sql.QueryContext(ctx, q, accountID, model, accountID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		t := parseTime(raw)
		if t.After(now) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// ExpireModelCooldowns removes all expired model cooldowns (garbage collection).
func (r *RoutingRepo) ExpireModelCooldowns(ctx context.Context) (int64, error) {
	q := r.db.rebind(`DELETE FROM model_cooldowns WHERE cooldown_until < ?`)
	res, err := r.db.sql.ExecContext(ctx, q, formatTime(time.Now()))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Chain rotation ------------------------------------------------------------

// GetChainRotation returns the persisted round-robin index for a chain.
// Returns 0 if no rotation state exists yet.
func (r *RoutingRepo) GetChainRotation(ctx context.Context, chainID string) (int, error) {
	q := r.db.rebind(`SELECT last_index FROM chain_rotation WHERE chain_id = ?`)
	var idx int
	err := r.db.sql.QueryRowContext(ctx, q, chainID).Scan(&idx)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: get chain rotation: %w", err)
	}
	return idx, nil
}

// SetChainRotation persists the round-robin cursor for a chain.
func (r *RoutingRepo) SetChainRotation(ctx context.Context, chainID string, index int) error {
	q := r.db.rebind(`INSERT INTO chain_rotation (chain_id, last_index, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(chain_id) DO UPDATE SET last_index = excluded.last_index, updated_at = excluded.updated_at`)
	_, err := r.db.sql.ExecContext(ctx, q, chainID, index, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set chain rotation: %w", err)
	}
	return nil
}

// randHex generates a random hex string of the given byte length.
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}