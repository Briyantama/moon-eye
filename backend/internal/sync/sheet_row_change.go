package syncworker

// SheetRowChange represents a single row-level change that should be
// applied to or read from Google Sheets.
type SheetRowChange struct {
	RowID   string
	Payload map[string]any
}
