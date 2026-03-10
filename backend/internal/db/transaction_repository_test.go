package db

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
)

// NOTE: These tests are intentionally light-weight and focus on compile-time
// safety and basic wiring. Full DB-backed tests will be introduced in later
// phases using testcontainers.

func TestTransactionFilter_Defaults(t *testing.T) {
	id := uuid.Must(uuid.NewV4())
	filter := TransactionFilter{
		UserID: id,
	}

	// Using a nil repository is fine here; we only care that List can be
	// called with a fully-populated filter type. The real behavior is
	// exercised in integration tests.
	var repo TransactionRepository = (*PGXTransactionRepository)(nil)

	ctx := context.Background()
	_, _ = repo.List(ctx, nil, filter) // compile-time interface satisfaction
}

func TestCreateUpdateParams_Types(t *testing.T) {
	userID := uuid.Must(uuid.NewV4())
	accountID := uuid.Must(uuid.NewV4())
	now := time.Now()

	categoryID := uuid.Must(uuid.NewV4())
	desc := "test"
	rowID := "row-1"

	create := CreateTransactionParams{
		UserID:       userID,
		AccountID:    accountID,
		Amount:       100.0,
		Currency:     "USD",
		Type:         "expense",
		CategoryID:   &categoryID,
		Description:  &desc,
		OccurredAt:   now,
		Metadata:     map[string]any{"key": "value"},
		Version:      1,
		LastModified: now,
		Source:       "app",
		SheetsRowID:  &rowID,
		Deleted:      false,
	}

	update := UpdateTransactionParams{
		ID:          uuid.Must(uuid.NewV4()),
		UserID:      userID,
		AccountID:   accountID,
		Amount:      200.0,
		Currency:    "USD",
		Type:        "income",
		CategoryID:  &categoryID,
		Description: &desc,
		OccurredAt:  now,
		Metadata:    map[string]any{"key": "updated"},
		Source:      "app",
		SheetsRowID: &rowID,
	}

	require.Equal(t, userID, create.UserID)
	require.Equal(t, accountID, create.AccountID)
	require.Equal(t, "USD", update.Currency)
}

