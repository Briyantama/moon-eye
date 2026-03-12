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
// Payload is SyncJobPayload. If ConnectionID is empty, sync is run for all active connections of the user.
func (s *SyncService) HandleSyncJob(ctx context.Context, msg queue.Message) error {
	if s == nil {
		return errors.New("nil SyncService")
	}

	var payload SyncJobPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}

	if payload.ConnectionID != "" {
		return s.SyncUserTransactions(ctx, payload.UserID, payload.ConnectionID, payload.Mode, payload.SinceVersion)
	}

	// "Sync user" mode: resolve all connections for the user and sync each.
	conns, err := s.connections.ListActiveByUser(ctx, payload.UserID)
	if err != nil {
		return err
	}
	for _, conn := range conns {
		if err := s.SyncUserTransactions(ctx, payload.UserID, conn.ID, payload.Mode, payload.SinceVersion); err != nil {
			return err
		}
	}
	return nil
}

// SyncUserTransactions performs a high-level, idempotent sync for one user/connection.
//
// Idempotency: Events are read with version > sinceVersion. The same (userID, connID, sinceVersion)
// run multiple times produces the same sheet state. ApplyChanges is applied with stable RowIDs
// (entity_id) so duplicate job processing does not double-apply.
//
// Conflict resolution: When both local and remote have changes for the same row (by RowID),
// we use "version wins": the change with the higher version (or later CreatedAt) is applied.
// Local changes are applied first; remote-only changes can be applied in two-way mode without
// overwriting a newer local version.
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

	// Read local change events (idempotent: same sinceVersion => same event set).
	localEvents, err := s.events.ListTransactionEventsSince(ctx, userID, sinceVersion, 1000)
	if err != nil {
		return err
	}

	localChanges := make([]SheetRowChange, 0, len(localEvents))
	for _, ev := range localEvents {
		localChanges = append(localChanges, SheetRowChange{
			RowID:   ev.EntityID,
			Payload: map[string]any{"raw": ev.Payload, "version": ev.Version, "createdAt": ev.CreatedAt},
		})
	}

	// Fetch remote changes for two-way merge.
	changes, _, err := s.sheets.FetchChanges(ctx, *conn, "")
	if err != nil {
		return err
	}

	// Two-way merge: version wins first, then LastModified (createdAt). Idempotent: same inputs => same merged output.
	merged := MergeSheetRows(localChanges, changes.Remote)
	_ = mode

	if len(merged) > 0 {
		if err := s.sheets.ApplyChanges(ctx, *conn, merged); err != nil {
			return err
		}
	}
	return nil
}

