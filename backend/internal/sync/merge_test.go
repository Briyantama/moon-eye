package syncworker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMergeSheetRows_VersionWins(t *testing.T) {
	local := []SheetRowChange{
		{RowID: "a", Payload: map[string]any{"version": int64(1), "createdAt": time.Now().Add(-time.Hour)}},
	}
	remote := []SheetRowChange{
		{RowID: "a", Payload: map[string]any{"version": int64(10), "createdAt": time.Now().Add(-2 * time.Hour)}},
	}
	merged := MergeSheetRows(local, remote)
	require.Len(t, merged, 1)
	require.Equal(t, "a", merged[0].RowID)
	require.Equal(t, int64(10), merged[0].Payload["version"], "higher version wins")
}

func TestMergeSheetRows_CreatedAtTieBreak(t *testing.T) {
	now := time.Now()
	local := []SheetRowChange{
		{RowID: "b", Payload: map[string]any{"version": int64(5), "createdAt": now.Add(-time.Hour)}},
	}
	remote := []SheetRowChange{
		{RowID: "b", Payload: map[string]any{"version": int64(5), "createdAt": now}},
	}
	merged := MergeSheetRows(local, remote)
	require.Len(t, merged, 1)
	require.Equal(t, "b", merged[0].RowID)
	// Same version; later createdAt wins (remote row)
	require.Equal(t, int64(5), merged[0].Payload["version"])
}

func TestMergeSheetRows_Combined(t *testing.T) {
	local := []SheetRowChange{
		{RowID: "only-local", Payload: map[string]any{"version": int64(1)}},
		{RowID: "conflict", Payload: map[string]any{"version": int64(2)}},
	}
	remote := []SheetRowChange{
		{RowID: "only-remote", Payload: map[string]any{"version": int64(1)}},
		{RowID: "conflict", Payload: map[string]any{"version": int64(3)}},
	}
	merged := MergeSheetRows(local, remote)
	require.Len(t, merged, 3)
	byID := make(map[string]SheetRowChange)
	for _, r := range merged {
		byID[r.RowID] = r
	}
	require.Contains(t, byID, "only-local")
	require.Contains(t, byID, "only-remote")
	require.Contains(t, byID, "conflict")
	require.Equal(t, int64(3), byID["conflict"].Payload["version"], "remote wins conflict")
}
