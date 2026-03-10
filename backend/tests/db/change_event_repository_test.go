package db_test

import (
	"testing"

	"moon-eye/backend/internal/db"
)

// NOTE: These tests currently focus on compile-time safety and wiring. Full
// behavior tests (including real inserts, tx vs non-tx, and rollback) will be
// added later using testcontainers once the integration test harness is in
// place.

func TestChangeEventRepository_InterfaceSatisfaction(t *testing.T) {
	var _ db.ChangeEventRepository = db.NewChangeEventRepository(nil, nil)
}

