package pgtypex

import (
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// NumericFromFloat64 converts a float64 into pgtype.Numeric.
func NumericFromFloat64(f float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(f); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

// Float64FromNumeric converts pgtype.Numeric into float64.
func Float64FromNumeric(n pgtype.Numeric) (float64, error) {
	if !n.Valid || n.NaN {
		return 0, nil
	}

	// Start with the integer coefficient.
	r := new(big.Rat).SetInt(n.Int)

	// Apply the base‑10 exponent.
	if n.Exp < 0 {
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)
		r.Quo(r, new(big.Rat).SetInt(den))
	} else if n.Exp > 0 {
		mul := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)
		r.Mul(r, new(big.Rat).SetInt(mul))
	}

	// Convert rational to float64 (may lose precision, which is acceptable here).
	f, _ := r.Float64()
	return f, nil
}

// TimestamptzFromTime converts time.Time into pgtype.Timestamptz.
func TimestamptzFromTime(t time.Time) (pgtype.Timestamptz, error) {
	var ts pgtype.Timestamptz
	if err := ts.Scan(t); err != nil {
		return pgtype.Timestamptz{}, err
	}
	return ts, nil
}

// TimeFromTimestamptz converts pgtype.Timestamptz into time.Time.
func TimeFromTimestamptz(ts pgtype.Timestamptz) time.Time {
	return ts.Time
}

// TextFromStringPtr converts *string into pgtype.Text.
func TextFromStringPtr(s *string) (pgtype.Text, error) {
	var txt pgtype.Text
	if s == nil {
		return txt, nil
	}
	if err := txt.Scan(*s); err != nil {
		return pgtype.Text{}, err
	}
	return txt, nil
}

// StringPtrFromText converts pgtype.Text into *string.
func StringPtrFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

