package syncworker_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"moon-eye/backend/internal/queue"
	syncworker "moon-eye/backend/internal/sync"
)

type stubConnRepo struct {
	conn *syncworker.SheetsConnection
}

func (s *stubConnRepo) GetByID(ctx context.Context, id string) (*syncworker.SheetsConnection, error) {
	_ = ctx
	_ = id
	return s.conn, nil
}

func (s *stubConnRepo) ListActiveByUser(ctx context.Context, userID string) ([]syncworker.SheetsConnection, error) {
	_ = ctx
	if s.conn != nil && s.conn.UserID == userID {
		return []syncworker.SheetsConnection{*s.conn}, nil
	}
	return nil, nil
}

type stubMappingRepo struct{}

func (s *stubMappingRepo) ListByConnection(ctx context.Context, connectionID string) ([]syncworker.SheetMapping, error) {
	_ = ctx
	_ = connectionID
	return nil, nil
}

type stubChangeReader struct {
	events []syncworker.ChangeEvent
}

func (s *stubChangeReader) ListTransactionEventsSince(ctx context.Context, userID string, sinceVersion int64, limit int) ([]syncworker.ChangeEvent, error) {
	_ = ctx
	_ = userID
	_ = sinceVersion
	_ = limit
	return s.events, nil
}

type stubSheetsClient struct {
	applied []syncworker.SheetRowChange
}

func (s *stubSheetsClient) FetchChanges(ctx context.Context, conn syncworker.SheetsConnection, cursor string) (syncworker.Changes, string, error) {
	_ = ctx
	_ = conn
	return syncworker.Changes{}, cursor, nil
}

func (s *stubSheetsClient) ApplyChanges(ctx context.Context, conn syncworker.SheetsConnection, rows []syncworker.SheetRowChange) error {
	_ = ctx
	_ = conn
	s.applied = append(s.applied, rows...)
	return nil
}

func TestSyncService_HandleSyncJob_PushesLocalChanges(t *testing.T) {
	conn := &syncworker.SheetsConnection{
		ID:     "conn-1",
		UserID: "user-1",
	}

	connRepo := &stubConnRepo{conn: conn}
	mappingRepo := &stubMappingRepo{}

	evPayload, _ := json.Marshal(map[string]any{"id": "txn-1"})
	changeReader := &stubChangeReader{
		events: []syncworker.ChangeEvent{
			{
				EntityType: "transaction",
				EntityID:   "txn-1",
				UserID:     "user-1",
				OpType:     "create",
				Version:    2,
				Payload:    evPayload,
			},
		},
	}

	sheetsClient := &stubSheetsClient{}
	svc := syncworker.NewSyncService(connRepo, mappingRepo, changeReader, sheetsClient)

	job := syncworker.SyncJobPayload{
		UserID:       "user-1",
		ConnectionID: "conn-1",
		Mode:         "two-way",
		SinceVersion: 1,
	}
	rawJob, _ := json.Marshal(job)

	msg := queue.Message{
		ID:        "1-0",
		Entity:    "transaction",
		Operation: "create",
		Payload:   rawJob,
	}

	ctx := context.Background()
	err := svc.HandleSyncJob(ctx, msg)
	require.NoError(t, err)

	require.Len(t, sheetsClient.applied, 1, "expected one local change to be applied")
	require.Equal(t, "txn-1", sheetsClient.applied[0].RowID)
}

