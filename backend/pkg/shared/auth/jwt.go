package auth

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtKey     []byte
	jwtKeyOnce sync.Once
	jwtKeyErr  error
)

// ErrJWTKeyMissing is returned when JWT_SIGNING_KEY is unset or empty.
var ErrJWTKeyMissing = errors.New("auth: JWT_SIGNING_KEY must be set")

func initJWTKey() {
	jwtKeyOnce.Do(func() {
		raw := os.Getenv("JWT_SIGNING_KEY")
		if raw == "" {
			jwtKeyErr = ErrJWTKeyMissing
			return
		}
		jwtKey = []byte(raw)
	})
}

// Claims holds standard JWT claims plus user identifiers.
type Claims struct {
	jwt.RegisteredClaims
	UserID string `json:"userId"`
	Email  string `json:"email"`
}

// Sign creates a signed JWT access token for the user. Uses HS256 and JWT_SIGNING_KEY.
// exp is the token lifetime (e.g. 15 * time.Minute).
func Sign(userID, email string, exp time.Duration) (string, error) {
	initJWTKey()
	if jwtKeyErr != nil {
		return "", jwtKeyErr
	}
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(exp)),
		},
		UserID: userID,
		Email:  email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtKey)
	if err != nil {
		return "", fmt.Errorf("jwt sign: %w", err)
	}
	return signed, nil
}

// Verify parses and validates the token, returning userID and email.
func Verify(tokenString string) (userID, email string, err error) {
	initJWTKey()
	if jwtKeyErr != nil {
		return "", "", jwtKeyErr
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtKey, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("jwt verify: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", "", errors.New("jwt: invalid token or claims")
	}
	return claims.UserID, claims.Email, nil
}
