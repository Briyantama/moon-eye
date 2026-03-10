package syncworker

// Changes summarizes local and remote changes participating in a sync
// operation.
type Changes struct {
	Local  []SheetRowChange
	Remote []SheetRowChange
}
