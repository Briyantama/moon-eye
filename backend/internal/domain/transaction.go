package domain

import "time"

// Transaction models the core transaction entity in the domain layer.
// It is intentionally decoupled from persistence concerns.
type Transaction struct {
	ID           string
	UserID       string
	AccountID    string
	Amount       float64
	Currency     string
	Type         string
	CategoryID   *string
	Description  *string
	OccurredAt   time.Time
	Metadata     map[string]any
	Version      int64
	LastModified time.Time
	Source       string
	SheetsRowID  *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    time.Time
}

