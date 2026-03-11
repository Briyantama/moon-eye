package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	// KeySizeBytes is the required AES-256 key size.
	KeySizeBytes = 32
)

var (
	key     []byte
	keyOnce sync.Once
	keyErr  error
)

// ErrKeyMissing is returned when APP_CRYPTO_KEY is unset or invalid.
var ErrKeyMissing = errors.New("crypto: APP_CRYPTO_KEY must be set and decode to 32 bytes (hex 64 chars or base64)")

// initKey reads APP_CRYPTO_KEY from the environment once. Key must decode to exactly 32 bytes.
func initKey() {
	keyOnce.Do(func() {
		raw := os.Getenv("APP_CRYPTO_KEY")
		if raw == "" {
			keyErr = ErrKeyMissing
			return
		}
		// Try hex (64 chars) then base64.
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == KeySizeBytes {
			key = decoded
			return
		}
		if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == KeySizeBytes {
			key = decoded
			return
		}
		keyErr = fmt.Errorf("%w: key must decode to %d bytes", ErrKeyMissing, KeySizeBytes)
	})
}

// Encrypt encrypts plaintext with AES-GCM using the key from APP_CRYPTO_KEY.
// Returns ciphertext (nonce || tag || ciphertext) or error if key is not configured.
func Encrypt(plaintext []byte) ([]byte, error) {
	initKey()
	if keyErr != nil {
		return nil, keyErr
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext produced by Encrypt. Expects nonce || tag || ciphertext.
func Decrypt(ciphertext []byte) ([]byte, error) {
	initKey()
	if keyErr != nil {
		return nil, keyErr
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// ConstantTimeCompare returns true if a and b are equal, using constant-time comparison
// to avoid timing side channels. Use for tokens and secrets.
func ConstantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var out byte
	for i := range a {
		out |= a[i] ^ b[i]
	}
	return out == 0
}

