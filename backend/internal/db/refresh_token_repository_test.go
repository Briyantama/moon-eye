package db

import (
	"testing"
)

// TestRefreshTokenRepository_Interface ensures PGXRefreshTokenRepository satisfies RefreshTokenRepository.
// Full DB tests use testcontainers in integration (Phase F).
func TestRefreshTokenRepository_Interface(t *testing.T) {
	var _ RefreshTokenRepository = (*PGXRefreshTokenRepository)(nil)
}

// TestNewRefreshTokenRepository_ReturnsImplementation ensures the constructor returns a non-nil implementation.
func TestNewRefreshTokenRepository_ReturnsImplementation(t *testing.T) {
	repo := NewRefreshTokenRepository(nil, nil)
	if repo == nil {
		t.Fatal("NewRefreshTokenRepository must not return nil")
	}
}
