package syncworker

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/api/sheets/v4"

	"moon-eye/backend/pkg/shared/mapx"
)

// GoogleSheetsClient implements SheetsClient using the Google Sheets API v4.
// Pass a configured *sheets.Service (e.g. from option.WithTokenSource or option.WithCredentialsFile).
type GoogleSheetsClient struct {
	svc *sheets.Service
}

// NewGoogleSheetsClient returns a SheetsClient using the given Sheets API service.
// svc must be created with credentials (OAuth2 token source or service account).
func NewGoogleSheetsClient(svc *sheets.Service) *GoogleSheetsClient {
	return &GoogleSheetsClient{svc: svc}
}

// FetchChanges returns remote sheet rows by reading the connection's range.
// Cursor is optional (e.g. last row index); empty means read from start.
// Returns Changes with Remote populated and a new cursor (e.g. last row index as string).
func (c *GoogleSheetsClient) FetchChanges(ctx context.Context, conn SheetsConnection, cursor string) (Changes, string, error) {
	if c.svc == nil {
		return Changes{}, "", fmt.Errorf("Google Sheets client not configured: wire *sheets.Service")
	}
	range_ := conn.SheetRange
	if range_ == "" {
		range_ = "Sheet1" // default
	}
	resp, err := c.svc.Spreadsheets.Values.Get(conn.SheetID, range_).Context(ctx).Do()
	if err != nil {
		return Changes{}, "", fmt.Errorf("sheets values get: %w", err)
	}
	rows := resp.Values
	if len(rows) == 0 {
		return Changes{Remote: nil}, "", nil
	}
	// First row = headers; build Remote as row index -> Payload map[column index string]value
	remote := make([]SheetRowChange, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		payload := make(map[string]any)
		for j, cell := range rows[i] {
			payload[strconv.Itoa(j)] = cell
		}
		remote = append(remote, SheetRowChange{
			RowID:   strconv.Itoa(i + 1), // 1-based row number in sheet
			Payload: payload,
		})
	}
	newCursor := ""
	if len(rows) > 1 {
		newCursor = strconv.Itoa(len(rows))
	}
	return Changes{Remote: remote}, newCursor, nil
}

// ApplyChanges writes rows to the sheet. Payload keys are column indices "0","1",...
// Values are written in ascending column index order; missing keys are written as empty.
func (c *GoogleSheetsClient) ApplyChanges(ctx context.Context, conn SheetsConnection, rows []SheetRowChange) error {
	if c.svc == nil {
		return fmt.Errorf("Google Sheets client not configured: wire *sheets.Service")
	}
	if len(rows) == 0 {
		return nil
	}
	range_ := conn.SheetRange
	if range_ == "" {
		range_ = "Sheet1"
	}
	valueRows := make([][]interface{}, 0, len(rows))
	for _, r := range rows {
		ordered := mapx.OrderedSliceByIndex(r.Payload)
		valueRows = append(valueRows, ordered)
	}
	vr := &sheets.ValueRange{Values: valueRows}
	_, err := c.svc.Spreadsheets.Values.Update(conn.SheetID, range_, vr).
		ValueInputOption("USER_ENTERED").
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("sheets values update: %w", err)
	}
	return nil
}

