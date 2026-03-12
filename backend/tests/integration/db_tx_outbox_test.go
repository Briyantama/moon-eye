//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"moon-eye/backend/internal/db"
	sqlcdb "moon-eye/backend/internal/db/sqlc"
	service "moon-eye/backend/internal/service"
	"moon-eye/backend/internal/service/projection"
	"moon-eye/backend/internal/wiring"
)

// setupPostgresContainer starts a PostgreSQL 15 container for integration tests.
func setupPostgresContainer(t *testing.T) (testcontainers.Container, string) {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "moon_eye",
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://postgres:postgres@%s:%s/moon_eye?sslmode=disable", host, port.Port())

	return container, dsn
}

// setupDatabase initializes a pgxpool. Call runMigrations after to apply schema.
func setupDatabase(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// runMigrations executes migration SQL files in order against the pool.
// Expects to be run from the backend directory (migrations/ relative to cwd).
func runMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	files := []string{
		"migrations/0001_init.sql",
		"migrations/0002_audit_and_soft_delete.sql",
		"migrations/0003_transactions_pagination.sql",
		"migrations/0004_projections.sql",
		"migrations/0005_auth_tokens.sql",
	}

	ctx := context.Background()
	for _, name := range files {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Skipf("migrations not found (run from backend dir): %v", err)
			return
		}
		_, err = pool.Exec(ctx, string(body))
		require.NoError(t, err, "run migration %s", name)
	}
}

// seedUserAndAccount inserts a user and account with fixed UUIDs for tests.
func seedUserAndAccount(t *testing.T, pool *pgxpool.Pool, userID, accountID uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, display_name, created_at, updated_at)
		 VALUES ($1, $2, $3, now(), now())
		 ON CONFLICT (id) DO NOTHING`,
		userID, "test@example.com", "Test User",
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx,
		`INSERT INTO accounts (id, user_id, name, currency, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now())
		 ON CONFLICT (id) DO NOTHING`,
		accountID, userID, "Test Account", "USD",
	)
	require.NoError(t, err)
}

// TestTxOutbox_CreateTransaction_PersistsTransactionAndChangeEvent runs a full
// CreateTransaction flow with migrations, then asserts transaction and change_events rows.
func TestTxOutbox_CreateTransaction_PersistsTransactionAndChangeEvent(t *testing.T) {
	if os.Getenv("INTEGRATION_DB") == "" {
		t.Skip("integration test; set INTEGRATION_DB=1 to run")
	}

	_, dsn := setupPostgresContainer(t)
	pool := setupDatabase(t, dsn)
	runMigrations(t, pool)

	userID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440001"))
	accountID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440002"))
	seedUserAndAccount(t, pool, userID, accountID)

	queries := sqlcdb.New(pool)
	repos := db.NewRepositories(pool, queries)
	txRunner := db.NewTxRunner(pool)
	txManager := wiring.NewTxManagerAdapter(txRunner, repos.Transactions, repos.ChangeEvents)

	txRepo := wiring.NewServiceTransactionAdapter(repos.Transactions, nil)
	changeEvents := service.NewDBChangeEventWriter(repos.ChangeEvents)
	var syncQ service.SyncQueue

	svc := service.NewTransactionService(txRepo, changeEvents, syncQ, txManager)

	ctx := context.Background()
	in := service.CreateTransactionInput{
		UserID:     userID.String(),
		AccountID:  accountID.String(),
		Amount:     100,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now().UTC(),
		Metadata:   map[string]any{"note": "integration"},
		Source:     "app",
	}

	txn, err := svc.CreateTransaction(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, txn)
	require.NotEmpty(t, txn.ID)

	// Assert transaction row
	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions WHERE user_id = $1 AND account_id = $2 AND deleted = false`,
		userID, accountID,
	).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected exactly one transaction row")

	var amount float64
	var version int64
	txnID, err := uuid.FromString(txn.ID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx,
		`SELECT amount, version FROM transactions WHERE id = $1`, txnID,
	).Scan(&amount, &version)
	require.NoError(t, err)
	require.Equal(t, 100.0, amount)
	require.Equal(t, int64(1), version)

	// Assert change_events row
	var evtCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM change_events WHERE entity_type = $1 AND entity_id = $2 AND op_type = $3 AND version = $4`,
		"transaction", txnID, "create", int64(1),
	).Scan(&evtCount)
	require.NoError(t, err)
	require.Equal(t, 1, evtCount, "expected exactly one change_event row")

	var payload []byte
	err = pool.QueryRow(ctx,
		`SELECT payload FROM change_events WHERE entity_type = 'transaction' AND entity_id = $1`, txnID,
	).Scan(&payload)
	require.NoError(t, err)
	require.NotEmpty(t, payload)
}

// runProjectionOnce runs one batch of the projection: list events since 0, process with summary and balance projectors, advance cursor.
func runProjectionOnce(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	source := projection.NewDBChangeEventSource(pool)
	cursor := projection.NewDBCursorStore(pool)
	summaryProjector := projection.NewTransactionSummaryProjector(pool)
	balanceProjector := projection.NewMonthlyBalanceProjector(pool)
	events, err := source.ListSince(ctx, 0, 50)
	require.NoError(t, err)
	if len(events) == 0 {
		return
	}
	var maxID int64
	for _, ev := range events {
		require.NoError(t, summaryProjector.Process(ctx, ev))
		require.NoError(t, balanceProjector.Process(ctx, ev))
		if ev.ID > maxID {
			maxID = ev.ID
		}
	}
	require.NoError(t, cursor.Set(ctx, "test", maxID))
}

// TestTxOutbox_ProjectionUpdates_AfterCreateTransaction creates a transaction, runs projection once, then asserts transaction_summary and monthly_balance.
func TestTxOutbox_ProjectionUpdates_AfterCreateTransaction(t *testing.T) {
	if os.Getenv("INTEGRATION_DB") == "" {
		t.Skip("integration test; set INTEGRATION_DB=1 to run")
	}

	_, dsn := setupPostgresContainer(t)
	pool := setupDatabase(t, dsn)
	runMigrations(t, pool)

	userID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440021"))
	accountID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440022"))
	seedUserAndAccount(t, pool, userID, accountID)

	queries := sqlcdb.New(pool)
	repos := db.NewRepositories(pool, queries)
	txRunner := db.NewTxRunner(pool)
	txManager := wiring.NewTxManagerAdapter(txRunner, repos.Transactions, repos.ChangeEvents)
	txRepo := wiring.NewServiceTransactionAdapter(repos.Transactions, nil)
	changeEvents := service.NewDBChangeEventWriter(repos.ChangeEvents)
	svc := service.NewTransactionService(txRepo, changeEvents, nil, txManager)

	ctx := context.Background()
	occ := time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)
	in := service.CreateTransactionInput{
		UserID:     userID.String(),
		AccountID:  accountID.String(),
		Amount:     50,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: occ,
		Source:     "app",
	}
	txn, err := svc.CreateTransaction(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, txn)

	runProjectionOnce(t, ctx, pool)

	periodKey := "2025-03"
	var summaryCount int
	var totalAmount float64
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(total_amount), 0) FROM transaction_summary WHERE user_id = $1 AND period_key = $2 AND currency = 'USD' AND type = 'expense'`,
		userID, periodKey,
	).Scan(&summaryCount, &totalAmount)
	require.NoError(t, err)
	require.Equal(t, 1, summaryCount, "transaction_summary should have one row for user/period/currency/type")
	require.Equal(t, 50.0, totalAmount)

	var balanceCount int
	var balance float64
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(balance), 0) FROM monthly_balance WHERE user_id = $1 AND account_id = $2 AND month_key = $3`,
		userID, accountID, periodKey,
	).Scan(&balanceCount, &balance)
	require.NoError(t, err)
	require.Equal(t, 1, balanceCount, "monthly_balance should have one row")
	require.Equal(t, 50.0, balance)
}

// failingChangeEventRepo wraps a ChangeEventRepository and always returns an error on Insert.
type failingChangeEventRepo struct {
	db.ChangeEventRepository
}

func (f *failingChangeEventRepo) Insert(ctx context.Context, tx pgx.Tx, e db.ChangeEvent) error {
	return fmt.Errorf("simulated change_event write failure")
}

// TestTxOutbox_CreateTransaction_RollbackOnChangeEventFailure verifies that when
// the change_event write fails inside the transaction, no transaction or
// change_event rows are committed.
func TestTxOutbox_CreateTransaction_RollbackOnChangeEventFailure(t *testing.T) {
	if os.Getenv("INTEGRATION_DB") == "" {
		t.Skip("integration test; set INTEGRATION_DB=1 to run")
	}

	_, dsn := setupPostgresContainer(t)
	pool := setupDatabase(t, dsn)
	runMigrations(t, pool)

	userID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440011"))
	accountID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440012"))
	seedUserAndAccount(t, pool, userID, accountID)

	queries := sqlcdb.New(pool)
	repos := db.NewRepositories(pool, queries)
	txRunner := db.NewTxRunner(pool)
	// Use a change event repo that always fails on Insert so the transaction rolls back.
	failingEvtRepo := &failingChangeEventRepo{ChangeEventRepository: repos.ChangeEvents}
	txManager := wiring.NewTxManagerAdapter(txRunner, repos.Transactions, failingEvtRepo)

	txRepo := wiring.NewServiceTransactionAdapter(repos.Transactions, nil)
	// Service needs a ChangeEventWriter that is used only inside the UoW; the UoW uses
	// failingEvtRepo via TxManagerAdapter. So we still need a ChangeEventWriter for the
	// service - but inside RunInTx the uow.ChangeEvents() is the tx-scoped writer that
	// calls failingEvtRepo.Insert. So the writer we pass to NewTransactionService is
	// never used when TxManager is set (all writes go through uow.ChangeEvents()).
	changeEvents := service.NewDBChangeEventWriter(repos.ChangeEvents)
	svc := service.NewTransactionService(txRepo, changeEvents, nil, txManager)

	ctx := context.Background()
	in := service.CreateTransactionInput{
		UserID:     userID.String(),
		AccountID:  accountID.String(),
		Amount:     200,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now().UTC(),
		Source:     "app",
	}

	txn, err := svc.CreateTransaction(ctx, in)
	require.Error(t, err)
	require.Nil(t, txn)

	// No transaction row committed
	var txCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transactions WHERE user_id = $1 AND account_id = $2`,
		userID, accountID,
	).Scan(&txCount)
	require.NoError(t, err)
	require.Equal(t, 0, txCount, "expected no transaction row when change_event write fails")

	// No change_events row for this user's transaction
	var evtCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM change_events WHERE user_id = $1`, userID,
	).Scan(&evtCount)
	require.NoError(t, err)
	require.Equal(t, 0, evtCount, "expected no change_event row when insert fails")
}
