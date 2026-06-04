package dispatch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/crypto"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

type fakeConnectorSource struct {
	conn core.Connector
}

func (s fakeConnectorSource) Get(provider string) (core.Connector, error) {
	return s.conn, nil
}

type fakeConnector struct{}

func (fakeConnector) ID() string            { return "openai" }
func (fakeConnector) Dialect() core.Dialect { return core.DialectOpenAI }
func (fakeConnector) Chat(context.Context, *core.ChatRequest, core.Credentials) (*core.ChatResponse, error) {
	return nil, nil
}
func (fakeConnector) Stream(context.Context, *core.ChatRequest, core.Credentials, core.StreamConfig) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk)
	close(ch)
	return ch, nil
}

func newDispatchTest(t *testing.T, accounts ...store.Account) (*Dispatcher, *store.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })

	mk, err := crypto.GenerateMasterKey()
	require.NoError(t, err)
	sealer, err := crypto.NewSealer(mk)
	require.NoError(t, err)
	v := vault.New(sealer)

	for i := range accounts {
		require.NoError(t, v.Seal(&accounts[i], vault.NewSecret{APIKey: "sk-test"}))
		require.NoError(t, db.Accounts().Create(ctx, accounts[i]))
	}

	d := New(fakeConnectorSource{conn: fakeConnector{}}, db.Accounts(), v)
	d.SetRoutingSource(db.Routing())
	return d, db
}

func testAccount(id string, priority int) store.Account {
	now := time.Now()
	return store.Account{
		ID:        id,
		TenantID:  store.DefaultTenantID,
		Provider:  "openai",
		Label:     id,
		AuthKind:  store.AuthAPIKey,
		Priority:  priority,
		Metadata:  "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestPlanWith_AccountRoundRobinRotatesProviderAccounts(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
		testAccount("acc-3", 30),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategyRoundRobin}

	var got []string
	for i := 0; i < 4; i++ {
		attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		require.NotEmpty(t, attempts)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-2", "acc-3", "acc-1"}, got)
}

func TestPlanWith_AccountRoundRobinHonorsStickyLimit(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategyRoundRobin, AccountStickyLimit: 2}

	var got []string
	for i := 0; i < 5; i++ {
		attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		require.NotEmpty(t, attempts)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-1", "acc-2", "acc-2", "acc-1"}, got)
}

func TestAdvanceRotationStateHonorsStickyLimit(t *testing.T) {
	cursor, nextCursor, hits := advanceRotationState(3, 0, 0, 2)
	require.Equal(t, 0, cursor)
	require.Equal(t, 0, nextCursor)
	require.Equal(t, 1, hits)

	cursor, nextCursor, hits = advanceRotationState(3, nextCursor, hits, 2)
	require.Equal(t, 0, cursor)
	require.Equal(t, 1, nextCursor)
	require.Equal(t, 0, hits)
}
