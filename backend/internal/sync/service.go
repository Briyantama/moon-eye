package syncworker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"moon-eye/backend/internal/queue"
)

// SheetsConnectionRepository loads sheets_connections.
type SheetsConnectionRepository interface {
	GetByID(ctx context.Context, id string) (*SheetsConnection, error)
	ListActiveByUser(ctx context.Context, userID string) ([]SheetsConnection, error)
}

// SheetMappingRepository loads sheet_mappings for a connection.
type SheetMappingRepository interface {
	ListByConnection(ctx context.Context, connectionID string) ([]SheetMapping, error)
}

// ChangeEventReader reads change_events for a user/entity.
type ChangeEventReader interface {
	ListTransactionEventsSince(ctx context.Context, userID string, sinceVersion int64, limit int) ([]ChangeEvent, error)
}

// SheetMapping describes how a single sheet column maps to a DB field.
type SheetMapping struct {
	ID           string
	ConnectionID string
	SheetColumn  string
	DBField      string
	Transform    json.RawMessage
}

// ChangeEvent is a lightweight view over a change_events row.
type ChangeEvent struct {
	ID         int64
	EntityType string
	EntityID   string
	UserID     string
	OpType     string
	Version    int64
	Payload    json.RawMessage
	CreatedAt  time.Time
}

// SyncService orchestrates sync between Postgres and Google Sheets.
type SyncService struct {
	connections SheetsConnectionRepository
	mappings    SheetMappingRepository
	events      ChangeEventReader
	sheets      SheetsClient
}

// NewSyncService constructs a SyncService.
func NewSyncService(
	connections SheetsConnectionRepository,
	mappings SheetMappingRepository,
	events ChangeEventReader,
	sheets SheetsClient,
) *SyncService {
	return &SyncService{
		connections: connections,
		mappings:    mappings,
		events:      events,
		sheets:      sheets,
	}
}

// SyncJobPayload is the structured payload expected inside queue.Message.Payload.
type SyncJobPayload struct {
	UserID       string `json:"userId"`
	ConnectionID string `json:"connectionId"`
	Mode         string `json:"mode"`
	// SinceVersion can be used for idempotency and incremental sync.
	SinceVersion int64 `json:"sinceVersion"`
}

// HandleSyncJob is the main entrypoint from workers.
func (s *SyncService) HandleSyncJob(ctx context.Context, msg queue.Message) error {
	if s == nil {
		return errors.New("nil SyncService")
	}

	var payload SyncJobPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}

	return s.SyncUserTransactions(ctx, payload.UserID, payload.ConnectionID, payload.Mode, payload.SinceVersion)
}

// SyncUserTransactions performs a high-level, idempotent sync for one user/connection.
func (s *SyncService) SyncUserTransactions(
	ctx context.Context,
	userID string,
	connID string,
	mode string,
	sinceVersion int64,
) error {
	conn, err := s.connections.GetByID(ctx, connID)
	if err != nil {
		return err
	}

	if conn == nil || conn.UserID != userID {
		return errors.New("connection not found or does not belong to user")
	}

	// Load mappings for this connection.
	if _, err := s.mappings.ListByConnection(ctx, connID); err != nil {
		return err
	}

	// Read local change events.
	localEvents, err := s.events.ListTransactionEventsSince(ctx, userID, sinceVersion, 1000)
	if err != nil {
		return err
	}

	// For now, we treat all local events as potential sheet row changes.
	localChanges := make([]SheetRowChange, 0, len(localEvents))
	for _, ev := range localEvents {
		localChanges = append(localChanges, SheetRowChange{
			RowID:   ev.EntityID,
			Payload: map[string]any{"raw": ev.Payload},
		})
	}

	// Fetch remote changes using the provided SheetsClient.
	changes, _, err := s.sheets.FetchChanges(ctx, *conn, "")
	if err != nil {
		return err
	}

	_ = mode
	// TODO: implement proper conflict resolution using:
	// - localEvents (with Version / CreatedAt)
	// - changes.Remote
	// - conn.SyncMode

	// For now, perform a simple one-way push of local changes.
	if len(localChanges) > 0 {
		if err := s.sheets.ApplyChanges(ctx, *conn, localChanges); err != nil {
			return err
		}
	}

	// Optionally handle remote-only changes using changes.Remote in a future iteration.
	_ = changes

	return nil
}

