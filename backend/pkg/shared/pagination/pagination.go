package pagination

// DefaultLimit is the default page size when limit is missing or invalid.
const DefaultLimit = 50

// MaxLimit is the maximum allowed page size.
const MaxLimit = 200

// Normalize enforces sane defaults and bounds for limit and offset.
// Returns (limit, offset) with limit in [1, MaxLimit] and offset >= 0.
func Normalize(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
