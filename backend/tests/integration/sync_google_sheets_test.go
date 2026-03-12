//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/db"
	sqlcdb "moon-eye/backend/internal/db/sqlc"
	"moon-eye/backend/internal/queue"
	service "moon-eye/backend/internal/service"
	syncworker "moon-eye/backend/internal/sync"
	"moon-eye/backend/internal/wiring"
)

// mockSheetsClient records FetchChanges and ApplyChanges for assertions.
type mockSheetsClient struct {
	mu       sync.Mutex
	fetched  []syncworker.SheetsConnection
	applied  []syncworker.SheetRowChange
	fetchErr error
	applyErr error
	// remote can be set to simulate remote sheet state (e.g. for conflict tests).
	remote []syncworker.SheetRowChange
}

func (m *mockSheetsClient) FetchChanges(ctx context.Context, conn syncworker.SheetsConnection, cursor string) (syncworker.Changes, string, error) {
	_ = ctx
	_ = cursor
	m.mu.Lock()
	m.fetched = append(m.fetched, conn)
	remote := m.remote
	m.mu.Unlock()
	if m.fetchErr != nil {
		return syncworker.Changes{}, "", m.fetchErr
	}
	return syncworker.Changes{Remote: remote}, "", nil
}

func (m *mockSheetsClient) ApplyChanges(ctx context.Context, conn syncworker.SheetsConnection, rows []syncworker.SheetRowChange) error {
	_ = ctx
	_ = conn
	m.mu.Lock()
	m.applied = append(m.applied, rows...)
	m.mu.Unlock()
	if m.applyErr != nil {
		return m.applyErr
	}
	return nil
}

func (m *mockSheetsClient) getApplied() []syncworker.SheetRowChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]syncworker.SheetRowChange, len(m.applied))
	copy(out, m.applied)
	return out
}

func (m *mockSheetsClient) setRemote(rows []syncworker.SheetRowChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remote = rows
}

// seedSheetsConnection inserts a sheets_connection for tests (required columns: google_user_id, access_token).
func seedSheetsConnection(t *testing.T, pool *pgxpool.Pool, connID, userID uuid.UUID, sheetID, sheetRange string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO sheets_connections (id, user_id, google_user_id, access_token, sheet_id, sheet_range, sync_mode, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'two-way', now())
		 ON CONFLICT (id) DO NOTHING`,
		connID, userID, "test-google-id", "test-token", sheetID, sheetRange,
	)
	require.NoError(t, err)
}

// TestSync_HandleSyncJob_AppliesLocalChanges creates a transaction (so change_events exist), then runs HandleSyncJob
// with a mock SheetsClient and asserts that ApplyChanges was called with the expected row (idempotent push).
func TestSync_HandleSyncJob_AppliesLocalChanges(t *testing.T) {
	if os.Getenv("INTEGRATION_DB") == "" {
		t.Skip("integration test; set INTEGRATION_DB=1 to run")
	}

	_, dsn := setupPostgresContainer(t)
	pool := setupDatabase(t, dsn)
	runMigrations(t, pool)

	userID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440031"))
	accountID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440032"))
	connID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440033"))
	seedUserAndAccount(t, pool, userID, accountID)
	seedSheetsConnection(t, pool, connID, userID, "mock-sheet-id", "Sheet1!A:E")

	queries := sqlcdb.New(pool)
	repos := db.NewRepositories(pool, queries)
	txRunner := db.NewTxRunner(pool)
	txManager := wiring.NewTxManagerAdapter(txRunner, repos.Transactions, repos.ChangeEvents)
	txRepo := wiring.NewServiceTransactionAdapter(repos.Transactions, nil)
	changeEventsWriter := service.NewDBChangeEventWriter(repos.ChangeEvents)
	svc := service.NewTransactionService(txRepo, changeEventsWriter, nil, txManager)

	ctx := context.Background()
	in := service.CreateTransactionInput{
		UserID:     userID.String(),
		AccountID:  accountID.String(),
		Amount:     75,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now().UTC(),
		Source:     "app",
	}
	txn, err := svc.CreateTransaction(ctx, in)
	require.NoError(t, err)
	require.NotEmpty(t, txn.ID)

	// Assert change_events row exists
	var evtCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM change_events WHERE entity_type = 'transaction' AND entity_id = $1`, txn.ID,
	).Scan(&evtCount)
	require.NoError(t, err)
	require.Equal(t, 1, evtCount)

	// Build SyncService with mock client and run HandleSyncJob (no Redis; direct call).
	mockSheets := &mockSheetsClient{}
	sheetsConn := db.NewPGShedsConnectionRepository(pool)
	sheetsMapping := db.NewPGSheetMappingRepository(pool)
	changeEventReader := db.NewPGChangeEventReader(pool)
	syncSvc := syncworker.NewSyncService(sheetsConn, sheetsMapping, changeEventReader, mockSheets)

	job := syncworker.SyncJobPayload{
		UserID:       userID.String(),
		ConnectionID: connID.String(),
		Mode:         "two-way",
		SinceVersion:  0,
	}
	rawJob, err := json.Marshal(job)
	require.NoError(t, err)
	msg := queue.Message{
		ID:       "test-1",
		Entity:   "sync",
		Operation: "sync",
		Payload:  rawJob,
	}

	err = syncSvc.HandleSyncJob(ctx, msg)
	require.NoError(t, err)

	applied := mockSheets.getApplied()
	require.Len(t, applied, 1, "expected one row applied to sheet")
	require.Equal(t, txn.ID, applied[0].RowID)
	require.NotNil(t, applied[0].Payload)
	// version is set from change_event (int64)
	ver := extractVersionFromPayload(t, applied[0].Payload)
	require.GreaterOrEqual(t, ver, int64(1))
}

// TestSync_ConflictResolution_VersionWins verifies merge: local and remote same RowID; higher version wins.
func TestSync_ConflictResolution_VersionWins(t *testing.T) {
	if os.Getenv("INTEGRATION_DB") == "" {
		t.Skip("integration test; set INTEGRATION_DB=1 to run")
	}

	_, dsn := setupPostgresContainer(t)
	pool := setupDatabase(t, dsn)
	runMigrations(t, pool)

	userID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440041"))
	accountID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440042"))
	connID := uuid.Must(uuid.FromString("550e8400-e29b-41d4-a716-446655440043"))
	seedUserAndAccount(t, pool, userID, accountID)
	seedSheetsConnection(t, pool, connID, userID, "mock-sheet-id", "Sheet1")

	// Mock returns remote row with version 10 for entity "entity-1"; local will have version 2.
	// After merge, applied rows should contain the winner: version 10 (remote).
	mockSheets := &mockSheetsClient{}
	mockSheets.setRemote([]syncworker.SheetRowChange{
		{
			RowID: "entity-1",
			Payload: map[string]any{
				"version":   int64(10),
				"createdAt": time.Now().Add(-time.Hour).Format(time.RFC3339),
				"0":         "entity-1",
				"1":         float64(10),
			},
		},
	})

	sheetsConn := db.NewPGShedsConnectionRepository(pool)
	sheetsMapping := db.NewPGSheetMappingRepository(pool)
	changeEventReader := db.NewPGChangeEventReader(pool)
	syncSvc := syncworker.NewSyncService(sheetsConn, sheetsMapping, changeEventReader, mockSheets)

	// Stub change_events so that "local" has entity-1 with version 2 (we don't need real DB events for merge test;
	// we only need SyncUserTransactions to read local events and merge with remote).
	// So we need to inject local events. The real flow reads from change_events; for this test we can either
	// create a real transaction and then manually insert a change_event with version 2 for entity-1, or use a
	// stub ChangeEventReader. Using real DB: create one transaction to get one change_event (version 1), then
	// run sync with mock remote (version 10). Merge should keep remote (version 10 > 1).
	queries := sqlcdb.New(pool)
	repos := db.NewRepositories(pool, queries)
	txRunner := db.NewTxRunner(pool)
	txManager := wiring.NewTxManagerAdapter(txRunner, repos.Transactions, repos.ChangeEvents)
	txRepo := wiring.NewServiceTransactionAdapter(repos.Transactions, nil)
	ceWriter := service.NewDBChangeEventWriter(repos.ChangeEvents)
	txSvc := service.NewTransactionService(txRepo, ceWriter, nil, txManager)
	ctx := context.Background()
	in := service.CreateTransactionInput{
		UserID:     userID.String(),
		AccountID:  accountID.String(),
		Amount:     100,
		Currency:   "USD",
		Type:       "expense",
		OccurredAt: time.Now().UTC(),
		Source:     "app",
	}
	txn, err := txSvc.CreateTransaction(ctx, in)
	require.NoError(t, err)
	// Local has one event with version 1. Remote has entity with same ID (txn.ID) but version 10.
	mockSheets.setRemote([]syncworker.SheetRowChange{
		{
			RowID: txn.ID,
			Payload: map[string]any{
				"version":   int64(10),
				"createdAt": time.Now().Add(2 * time.Hour).Format(time.RFC3339),
				"0":         txn.ID,
				"1":         float64(10),
			},
		},
	})

	job := syncworker.SyncJobPayload{
		UserID:       userID.String(),
		ConnectionID: connID.String(),
		Mode:         "two-way",
		SinceVersion:  0,
	}
	rawJob, _ := json.Marshal(job)
	err = syncSvc.HandleSyncJob(ctx, queue.Message{Payload: rawJob})
	require.NoError(t, err)

	applied := mockSheets.getApplied()
	require.Len(t, applied, 1)
	// Merge should prefer remote (version 10) over local (version 1).
	require.Equal(t, txn.ID, applied[0].RowID)
	require.GreaterOrEqual(t, extractVersionFromPayload(t, applied[0].Payload), int64(10))
}

func extractVersionFromPayload(t *testing.T, p map[string]any) int64 {
	t.Helper()
	if p == nil {
		return 0
	}
	switch v := p["version"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}
