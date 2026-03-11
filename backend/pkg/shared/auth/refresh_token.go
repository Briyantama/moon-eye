package auth

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/gofrs/uuid"
)

// EncodeRefreshToken returns the client-facing refresh token string: "id:base64(secret)".
func EncodeRefreshToken(id uuid.UUID, secret []byte) string {
	return id.String() + ":" + base64.StdEncoding.EncodeToString(secret)
}

// DecodeRefreshToken parses "id:base64secret" into id and secret bytes.
// Returns error if format is invalid or decoding fails.
func DecodeRefreshToken(s string) (id uuid.UUID, secret []byte, err error) {
	idx := strings.Index(s, ":")
	if idx <= 0 || idx >= len(s)-1 {
		return uuid.Nil, nil, fmt.Errorf("invalid refresh token format")
	}
	id, err = uuid.FromString(s[:idx])
	if err != nil {
		return uuid.Nil, nil, err
	}
	secret, err = base64.StdEncoding.DecodeString(s[idx+1:])
	if err != nil {
		return uuid.Nil, nil, err
	}
	return id, secret, nil
}
