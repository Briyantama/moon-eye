package syncworker

// SheetsConnection represents a logical connection to a Google Sheet
// for a specific user.
type SheetsConnection struct {
	ID           string
	UserID       string
	SheetID      string
	SheetRange   string
	SyncMode     string
	LastSyncedAt *int64
}
