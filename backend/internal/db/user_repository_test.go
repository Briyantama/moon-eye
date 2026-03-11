package db

import (
	"testing"
)

// TestUserRepository_Interface ensures PGXUserRepository satisfies UserRepository.
// Full DB tests use testcontainers in integration (Phase F).
func TestUserRepository_Interface(t *testing.T) {
	var _ UserRepository = (*PGXUserRepository)(nil)
}

// TestNewUserRepository_ReturnsImplementation ensures the constructor returns a non-nil implementation.
func TestNewUserRepository_ReturnsImplementation(t *testing.T) {
	repo := NewUserRepository(nil, nil)
	if repo == nil {
		t.Fatal("NewUserRepository must not return nil")
	}
}
