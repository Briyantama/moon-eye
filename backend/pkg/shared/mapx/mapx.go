package mapx

import (
	"sort"
	"strconv"
)

// OrderedSliceByIndex converts a map with string keys "0", "1", "2", ... into
// a slice ordered by numeric index. Missing indices are filled with empty string.
// Useful for building sheet rows or ordered payloads from keyed maps.
func OrderedSliceByIndex(payload map[string]any) []interface{} {
	if len(payload) == 0 {
		return nil
	}
	indices := make([]int, 0, len(payload))
	for k := range payload {
		n, _ := strconv.Atoi(k)
		indices = append(indices, n)
	}
	sort.Ints(indices)
	row := make([]interface{}, indices[len(indices)-1]+1)
	for _, idx := range indices {
		row[idx] = payload[strconv.Itoa(idx)]
	}
	for i := range row {
		if row[i] == nil {
			row[i] = ""
		}
	}
	return row
}
