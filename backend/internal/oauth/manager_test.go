package oauth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

// Concurrent EnsureFresh calls on the same expiring account must trigger only
// one token exchange. Without singleflight dedup, each goroutine would call the
// refresh endpoint with the same refresh token; providers that rotate refresh
// tokens then reject all but the first with refresh_token_reused.
func TestTokenManager_EnsureFresh_DedupesConcurrentRefreshes(t *testing.T) {
	var calls int32
	start := make(chan struct{})

	m := &TokenManager{
		// Non-nil deps satisfy EnsureFresh's guard; the stubbed refresh never
		// touches them, so zero-value pointers are sufficient for this test.
		vault:    &vault.Vault{},
		accounts: &store.AccountRepo{},
	}
	m.refresh = func(_ context.Context, acc store.Account) (store.Account, error) {
		atomic.AddInt32(&calls, 1)
		// Hold the in-flight call open until every goroutine has arrived, so a
		// missing dedup would deterministically produce multiple calls.
		<-start
		return acc, nil
	}

	past := time.Now().Add(-time.Hour)
	acc := store.Account{ID: "acct-1", Provider: "kiro", AuthKind: store.AuthOAuth, TokenExpiresAt: &past}

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := m.EnsureFresh(context.Background(), acc); err != nil {
				t.Errorf("EnsureFresh returned error: %v", err)
			}
		}()
	}

	// Give the goroutines time to coalesce on the singleflight key, then release.
	time.Sleep(50 * time.Millisecond)
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 refresh call under concurrency, got %d", got)
	}
}

// A fresh token (expiry beyond the skew window) must not trigger any refresh.
func TestTokenManager_EnsureFresh_SkipsWhenStillValid(t *testing.T) {
	var calls int32
	m := &TokenManager{vault: &vault.Vault{}, accounts: &store.AccountRepo{}}
	m.refresh = func(_ context.Context, acc store.Account) (store.Account, error) {
		atomic.AddInt32(&calls, 1)
		return acc, nil
	}

	future := time.Now().Add(time.Hour)
	acc := store.Account{ID: "acct-2", Provider: "kiro", AuthKind: store.AuthOAuth, TokenExpiresAt: &future}

	if _, err := m.EnsureFresh(context.Background(), acc); err != nil {
		t.Fatalf("EnsureFresh returned error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected no refresh for a still-valid token, got %d", got)
	}
}
