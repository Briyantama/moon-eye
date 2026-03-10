package syncworker

import "context"

// NoopSheetsClient is a stub implementation used in local/dev setups.
type NoopSheetsClient struct{}

func (c *NoopSheetsClient) FetchChanges(ctx context.Context, conn SheetsConnection, cursor string) (Changes, string, error) {
	_ = ctx
	_ = conn
	return Changes{}, cursor, nil
}

func (c *NoopSheetsClient) ApplyChanges(ctx context.Context, conn SheetsConnection, rows []SheetRowChange) error {
	_ = ctx
	_ = conn
	_ = rows
	return nil
}
