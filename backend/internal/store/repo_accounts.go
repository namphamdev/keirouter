package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AccountRepo persists upstream provider accounts and their encrypted secrets.
type AccountRepo struct{ db *DB }

// Accounts returns the account repository.
func (db *DB) Accounts() *AccountRepo { return &AccountRepo{db: db} }

const accountColumns = `id, tenant_id, provider, label, auth_kind,
	secret_wrapped_dek, secret_ciphertext,
	token_wrapped_dek, token_ciphertext,
	refresh_wrapped_dek, refresh_ciphertext,
	token_expires_at, metadata, priority, backoff_level, disabled, cooldown_until,
	proxy_pool_id, created_at, updated_at`

// SetBackoffLevel updates the exponential backoff level for an account.
func (r *AccountRepo) SetBackoffLevel(ctx context.Context, id string, level int) error {
	q := r.db.rebind(`UPDATE accounts SET backoff_level = ?, updated_at = ? WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, level, formatTime(time.Now()), id)
	return err
}

// ResetBackoffLevel resets the exponential backoff level to 0 and clears
// cooldown, called on a successful request.
func (r *AccountRepo) ResetBackoffLevel(ctx context.Context, id string) error {
	q := r.db.rebind(`UPDATE accounts SET backoff_level = 0, cooldown_until = NULL, updated_at = ? WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, formatTime(time.Now()), id)
	return err
}

// Create inserts a new account.
func (r *AccountRepo) Create(ctx context.Context, a Account) error {
	q := r.db.rebind(`INSERT INTO accounts (` + accountColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.sql.ExecContext(ctx, q,
		a.ID, a.TenantID, a.Provider, a.Label, string(a.AuthKind),
		nullString(a.SecretWrappedDEK), nullString(a.SecretCiphertext),
		nullString(a.TokenWrappedDEK), nullString(a.TokenCiphertext),
		nullString(a.RefreshWrappedDEK), nullString(a.RefreshCiphertext),
		nullTime(a.TokenExpiresAt), a.Metadata, a.Priority, a.BackoffLevel,
		boolToInt(a.Disabled), nullTime(a.CooldownUntil), a.ProxyPoolID,
		formatTime(a.CreatedAt), formatTime(a.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create account: %w", err)
	}
	return nil
}

// Get returns one account by id.
func (r *AccountRepo) Get(ctx context.Context, id string) (Account, error) {
	q := r.db.rebind(`SELECT ` + accountColumns + ` FROM accounts WHERE id = ?`)
	row := r.db.sql.QueryRowContext(ctx, q, id)
	a, err := scanAccountRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// ListByProvider returns enabled accounts for a provider within a tenant,
// ordered by ascending priority (lower number = higher priority).
func (r *AccountRepo) ListByProvider(ctx context.Context, tenantID, provider string) ([]Account, error) {
	q := r.db.rebind(`SELECT ` + accountColumns + `
		FROM accounts
		WHERE tenant_id = ? AND provider = ? AND disabled = 0
		ORDER BY priority ASC, created_at ASC`)
	return r.queryList(ctx, q, tenantID, provider)
}

// ListByTenant returns all accounts for a tenant.
func (r *AccountRepo) ListByTenant(ctx context.Context, tenantID string) ([]Account, error) {
	q := r.db.rebind(`SELECT ` + accountColumns + `
		FROM accounts WHERE tenant_id = ? ORDER BY provider, priority ASC`)
	return r.queryList(ctx, q, tenantID)
}

// SetCooldown marks an account unavailable until the given time (used by the
// dispatcher after a rate-limit / quota error).
func (r *AccountRepo) SetCooldown(ctx context.Context, id string, until time.Time) error {
	q := r.db.rebind(`UPDATE accounts SET cooldown_until = ?, updated_at = ? WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, formatTime(until), formatTime(time.Now()), id)
	return err
}

// UpdateTokens replaces the sealed OAuth tokens and expiry after a refresh.
func (r *AccountRepo) UpdateTokens(ctx context.Context, a Account) error {
	q := r.db.rebind(`UPDATE accounts SET
		token_wrapped_dek = ?, token_ciphertext = ?,
		refresh_wrapped_dek = ?, refresh_ciphertext = ?,
		token_expires_at = ?, updated_at = ?
		WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q,
		nullString(a.TokenWrappedDEK), nullString(a.TokenCiphertext),
		nullString(a.RefreshWrappedDEK), nullString(a.RefreshCiphertext),
		nullTime(a.TokenExpiresAt), formatTime(time.Now()), a.ID)
	return err
}

// Update writes mutable fields (label, priority, disabled, proxy_pool_id).
func (r *AccountRepo) Update(ctx context.Context, a Account) error {
	q := r.db.rebind(`UPDATE accounts SET label = ?, priority = ?, disabled = ?,
		proxy_pool_id = ?, updated_at = ? WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q,
		a.Label, a.Priority, boolToInt(a.Disabled),
		a.ProxyPoolID, formatTime(time.Now()), a.ID)
	return err
}

// Delete removes an account.
func (r *AccountRepo) Delete(ctx context.Context, id string) error {
	q := r.db.rebind(`DELETE FROM accounts WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, id)
	return err
}

func (r *AccountRepo) queryList(ctx context.Context, q string, args ...any) ([]Account, error) {
	rows, err := r.db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		a, err := scanAccountRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// scanAccountRow scans an account from either a *sql.Row or *sql.Rows via the
// shared Scan signature.
func scanAccountRow(scan func(dest ...any) error) (Account, error) {
	var (
		a            Account
		authKind     string
		secretDEK    sql.NullString
		secretCT     sql.NullString
		tokenDEK     sql.NullString
		tokenCT      sql.NullString
		refreshDEK   sql.NullString
		refreshCT    sql.NullString
		tokenExpires sql.NullString
		backoffLevel int
		cooldown     sql.NullString
		disabled     int
		proxyPoolID  sql.NullString
		createdRaw   string
		updatedRaw   string
	)
	err := scan(
		&a.ID, &a.TenantID, &a.Provider, &a.Label, &authKind,
		&secretDEK, &secretCT, &tokenDEK, &tokenCT, &refreshDEK, &refreshCT,
		&tokenExpires, &a.Metadata, &a.Priority, &backoffLevel, &disabled, &cooldown,
		&proxyPoolID, &createdRaw, &updatedRaw,
	)
	if err != nil {
		return Account{}, err
	}
	a.AuthKind = AuthKind(authKind)
	a.SecretWrappedDEK = secretDEK.String
	a.SecretCiphertext = secretCT.String
	a.TokenWrappedDEK = tokenDEK.String
	a.TokenCiphertext = tokenCT.String
	a.RefreshWrappedDEK = refreshDEK.String
	a.RefreshCiphertext = refreshCT.String
	a.BackoffLevel = backoffLevel
	a.Disabled = disabled != 0
	a.ProxyPoolID = proxyPoolID.String
	a.CreatedAt = parseTime(createdRaw)
	a.UpdatedAt = parseTime(updatedRaw)
	if tokenExpires.Valid {
		t := parseTime(tokenExpires.String)
		a.TokenExpiresAt = &t
	}
	if cooldown.Valid {
		t := parseTime(cooldown.String)
		a.CooldownUntil = &t
	}
	return a, nil
}