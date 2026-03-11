package db

import (
	"time"

	"github.com/gofrs/uuid"
)

// Transaction models the database representation of a transaction row.
// This is intentionally close to the schema, and can be mapped to the
// higher-level domain.Transaction type by callers.
type Transaction struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	AccountID    uuid.UUID
	Amount       float64
	Currency     string
	Type         string
	CategoryID   *uuid.UUID
	Description  *string
	OccurredAt   time.Time
	Metadata     map[string]any
	Version      int64
	LastModified time.Time
	Source       string
	SheetsRowID  *string
	Deleted      bool
}

// CreateTransactionParams captures the fields required to insert a new
// transaction row.
type CreateTransactionParams struct {
	UserID       uuid.UUID
	AccountID    uuid.UUID
	Amount       float64
	Currency     string
	Type         string
	CategoryID   *uuid.UUID
	Description  *string
	OccurredAt   time.Time
	Metadata     map[string]any
	Version      int64
	LastModified time.Time
	Source       string
	SheetsRowID  *string
	Deleted      bool
}

// UpdateTransactionParams captures updatable fields for an existing transaction.
// The ID and UserID pair uniquely identify the row to update.
type UpdateTransactionParams struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	AccountID   uuid.UUID
	Amount      float64
	Currency    string
	Type        string
	CategoryID  *uuid.UUID
	Description *string
	OccurredAt  time.Time
	Metadata    map[string]any
	Source      string
	SheetsRowID *string
}

// TransactionFilter provides a rich filter surface for listing transactions.
type TransactionFilter struct {
	UserID        uuid.UUID
	AccountID     *uuid.UUID
	Type          *string
	FromOccurredAt *time.Time
	ToOccurredAt   *time.Time
	Limit         int
	Offset        int
}

