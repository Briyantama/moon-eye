package stringsx

import "strings"

// NormalizeEmail trims whitespace and lowercases the string.
// Use for normalizing email before lookup or storage.
func NormalizeEmail(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}
