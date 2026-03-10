package service

import (
	"context"

	"moon-eye/backend/internal/db"
)

// dbChangeEventWriter is an adapter that allows the DB change event repository
// to satisfy the ChangeEventWriter interface used by services.
type dbChangeEventWriter struct {
	repo db.ChangeEventRepository
}

// NewDBChangeEventWriter wraps a DB-backed ChangeEventRepository so it can be
// used as a ChangeEventWriter in the service layer.
func NewDBChangeEventWriter(repo db.ChangeEventRepository) ChangeEventWriter {
	return &dbChangeEventWriter{repo: repo}
}

func (w *dbChangeEventWriter) Create(ctx context.Context, in ChangeEventInput) error {
	if w == nil || w.repo == nil {
		return nil
	}

	return w.repo.Insert(ctx, nil, db.ChangeEvent{
		EntityType: in.EntityType,
		EntityID:   in.EntityID,
		UserID:     in.UserID,
		OpType:     in.Operation,
		Payload:    in.Payload,
		Version:    in.Version,
	})
}

