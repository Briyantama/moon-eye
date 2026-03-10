package syncworker

import "context"

// SheetsClient abstracts Google Sheets operations.
type SheetsClient interface {
	FetchChanges(ctx context.Context, conn SheetsConnection, cursor string) (Changes, string, error)
	ApplyChanges(ctx context.Context, conn SheetsConnection, rows []SheetRowChange) error
}
