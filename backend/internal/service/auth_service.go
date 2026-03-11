package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"moon-eye/backend/internal/db"
	"moon-eye/backend/pkg/shared/auth"
	"moon-eye/backend/pkg/shared/crypto"
	"moon-eye/backend/pkg/shared/stringsx"
)

// Default access token lifetime.
const AccessTokenExpiry = 15 * time.Minute

// Default refresh token lifetime (e.g. 7 days).
const RefreshTokenExpiry = 7 * 24 * time.Hour

// ErrInvalidCredentials is returned when email/password do not match.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrRefreshTokenInvalid is returned when the refresh token is invalid, expired, or revoked.
var ErrRefreshTokenInvalid = errors.New("refresh token invalid")

// AuthService handles registration, login, refresh, and revoke.
type AuthService interface {
	Register(ctx context.Context, email, password string) (User, error)
	Authenticate(ctx context.Context, email, password string) (accessToken, refreshToken string, err error)
	Refresh(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, err error)
	Revoke(ctx context.Context, refreshToken string) error
}

// User is the auth-facing user result (no password).
type User struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
}

// authServiceImpl implements AuthService using db repositories and TxRunner.
type authServiceImpl struct {
	users   db.UserRepository
	tokens  db.RefreshTokenRepository
	txRunner db.TxRunner
}

// NewAuthService returns an AuthService that uses the given repositories and tx runner.
func NewAuthService(users db.UserRepository, tokens db.RefreshTokenRepository, txRunner db.TxRunner) AuthService {
	return &authServiceImpl{
		users:   users,
		tokens:  tokens,
		txRunner: txRunner,
	}
}

func (s *authServiceImpl) Register(ctx context.Context, email, password string) (User, error) {
	email = stringsx.NormalizeEmail(email)
	if email == "" || password == "" {
		return User{}, fmt.Errorf("email and password required")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	now := time.Now().UTC()
	u := db.User{
		ID:             uuid.Must(uuid.NewV4()),
		Email:          email,
		HashedPassword: string(hashed),
		CreatedAt:      now,
		UpdatedAt:      now,
		Deleted:        false,
	}
	err = s.txRunner.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.users.Create(ctx, tx, u)
	})
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: u.ID, Email: u.Email, CreatedAt: u.CreatedAt}, nil
}

func (s *authServiceImpl) Authenticate(ctx context.Context, email, password string) (accessToken, refreshToken string, err error) {
	email = stringsx.NormalizeEmail(email)
	if email == "" || password == "" {
		return "", "", ErrInvalidCredentials
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return "", "", ErrInvalidCredentials
		}
		return "", "", fmt.Errorf("get user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.HashedPassword), []byte(password)); err != nil {
		return "", "", ErrInvalidCredentials
	}
	accessToken, err = auth.Sign(u.ID.String(), u.Email, AccessTokenExpiry)
	if err != nil {
		return "", "", fmt.Errorf("sign access token: %w", err)
	}
	refreshTokenID := uuid.Must(uuid.NewV4())
	secret := make([]byte, 32)
	if _, err := randRead(secret); err != nil {
		return "", "", fmt.Errorf("generate refresh secret: %w", err)
	}
	encrypted, err := crypto.Encrypt(secret)
	if err != nil {
		return "", "", fmt.Errorf("encrypt refresh token: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(RefreshTokenExpiry)
	tok := db.RefreshToken{
		ID:             refreshTokenID,
		UserID:         u.ID,
		EncryptedToken: base64.StdEncoding.EncodeToString(encrypted),
		ExpiresAt:      expiresAt,
		Revoked:        false,
		CreatedAt:      now,
	}
	err = s.txRunner.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.tokens.Create(ctx, tx, tok)
	})
	if err != nil {
		return "", "", fmt.Errorf("create refresh token: %w", err)
	}
	refreshToken = auth.EncodeRefreshToken(refreshTokenID, secret)
	return accessToken, refreshToken, nil
}

func (s *authServiceImpl) Refresh(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, err error) {
	tokenID, secret, err := auth.DecodeRefreshToken(refreshToken)
	if err != nil {
		return "", "", ErrRefreshTokenInvalid
	}
	row, err := s.tokens.GetByID(ctx, tokenID)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return "", "", ErrRefreshTokenInvalid
		}
		return "", "", fmt.Errorf("get refresh token: %w", err)
	}
	if row.Revoked {
		return "", "", ErrRefreshTokenInvalid
	}
	if time.Now().After(row.ExpiresAt) {
		return "", "", ErrRefreshTokenInvalid
	}
	encrypted, err := base64.StdEncoding.DecodeString(row.EncryptedToken)
	if err != nil {
		return "", "", ErrRefreshTokenInvalid
	}
	storedSecret, err := crypto.Decrypt(encrypted)
	if err != nil {
		return "", "", ErrRefreshTokenInvalid
	}
	if !crypto.ConstantTimeCompare(storedSecret, secret) {
		return "", "", ErrRefreshTokenInvalid
	}
	u, err := s.users.GetByID(ctx, row.UserID)
	if err != nil {
		return "", "", fmt.Errorf("get user: %w", err)
	}
	newAccessToken, err = auth.Sign(u.ID.String(), u.Email, AccessTokenExpiry)
	if err != nil {
		return "", "", fmt.Errorf("sign access token: %w", err)
	}
	newID := uuid.Must(uuid.NewV4())
	newSecret := make([]byte, 32)
	if _, err := randRead(newSecret); err != nil {
		return "", "", fmt.Errorf("generate refresh secret: %w", err)
	}
	newEncrypted, err := crypto.Encrypt(newSecret)
	if err != nil {
		return "", "", fmt.Errorf("encrypt refresh token: %w", err)
	}
	now := time.Now().UTC()
	newTok := db.RefreshToken{
		ID:             newID,
		UserID:         u.ID,
		EncryptedToken: base64.StdEncoding.EncodeToString(newEncrypted),
		ExpiresAt:      now.Add(RefreshTokenExpiry),
		Revoked:        false,
		CreatedAt:      now,
	}
	err = s.txRunner.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.tokens.Revoke(ctx, tx, tokenID); err != nil {
			return err
		}
		return s.tokens.Create(ctx, tx, newTok)
	})
	if err != nil {
		return "", "", fmt.Errorf("rotate refresh token: %w", err)
	}
	newRefreshToken = auth.EncodeRefreshToken(newID, newSecret)
	return newAccessToken, newRefreshToken, nil
}

func (s *authServiceImpl) Revoke(ctx context.Context, refreshToken string) error {
	tokenID, _, err := auth.DecodeRefreshToken(refreshToken)
	if err != nil {
		return ErrRefreshTokenInvalid
	}
	return s.tokens.Revoke(ctx, nil, tokenID)
}

// randRead fills b with cryptographically random bytes. Override in tests if needed.
var randRead = func(b []byte) (n int, err error) {
	return io.ReadFull(rand.Reader, b)
}
