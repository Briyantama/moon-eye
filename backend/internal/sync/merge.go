package syncworker

import (
	"sort"
	"strconv"
	"time"
)

// extractVersion returns version from a row payload (e.g. "version" key or column "1").
// Returns 0 if missing or invalid.
func extractVersion(payload map[string]any) int64 {
	if payload == nil {
		return 0
	}
	for _, key := range []string{"version", "1"} {
		switch v := payload[key].(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		case string:
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
		}
	}
	return 0
}

// extractCreatedAt returns CreatedAt time from payload for tie-breaking (version wins first, then LastModified).
// Supports "createdAt" or column "2" as time.Time, RFC3339 string, or Unix seconds.
func extractCreatedAt(payload map[string]any) time.Time {
	if payload == nil {
		return time.Time{}
	}
	for _, key := range []string{"createdAt", "2"} {
		switch v := payload[key].(type) {
		case time.Time:
			return v
		case string:
			t, _ := time.Parse(time.RFC3339, v)
			if !t.IsZero() {
				return t
			}
		}
	}
	if n := extractVersion(payload); n > 0 {
		return time.Unix(n, 0)
	}
	return time.Time{}
}

// MergeSheetRows merges local and remote row changes. For each RowID, the row with the higher version wins;
// on tie, the later createdAt wins. Returns merged rows in stable order (by RowID).
// Idempotent: same inputs produce same output.
func MergeSheetRows(local, remote []SheetRowChange) []SheetRowChange {
	byID := make(map[string]SheetRowChange)
	for _, r := range local {
		byID[r.RowID] = r
	}
	for _, r := range remote {
		cur, ok := byID[r.RowID]
		if !ok {
			byID[r.RowID] = r
			continue
		}
		vCur := extractVersion(cur.Payload)
		vNew := extractVersion(r.Payload)
		if vNew > vCur {
			byID[r.RowID] = r
		} else if vNew == vCur {
			if extractCreatedAt(r.Payload).After(extractCreatedAt(cur.Payload)) {
				byID[r.RowID] = r
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]SheetRowChange, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out
}
