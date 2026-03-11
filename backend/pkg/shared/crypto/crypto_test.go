package crypto

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// testKey is 32 bytes for AES-256 (hex-encoded in env).
var testKeyHex = hex.EncodeToString(make([]byte, KeySizeBytes))

func TestMain(m *testing.M) {
	// Set a valid key for tests so Encrypt/Decrypt can run without requiring env in CI.
	os.Setenv("APP_CRYPTO_KEY", testKeyHex)
	os.Exit(m.Run())
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	plaintext := []byte("secret refresh token value")
	ciphertext, err := Encrypt(plaintext)
	require.NoError(t, err)
	require.NotNil(t, ciphertext)
	require.NotEqual(t, plaintext, ciphertext)

	decrypted, err := Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	ciphertext, err := Encrypt(nil)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)

	decrypted, err := Decrypt(ciphertext)
	require.NoError(t, err)
	require.Empty(t, decrypted)
}

func TestDecrypt_InvalidCiphertext(t *testing.T) {
	_, err := Decrypt([]byte("too short"))
	require.Error(t, err)

	_, err = Decrypt(nil)
	require.Error(t, err)
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	plaintext := []byte("data")
	ciphertext, err := Encrypt(plaintext)
	require.NoError(t, err)
	if len(ciphertext) > 0 {
		ciphertext[0] ^= 0xff
	}
	_, err = Decrypt(ciphertext)
	require.Error(t, err)
}

func TestKeySizeBytes(t *testing.T) {
	require.Equal(t, 32, KeySizeBytes)
}

func TestEncryptDecrypt_LongPlaintext(t *testing.T) {
	plaintext := make([]byte, 10000)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}
	ciphertext, err := Encrypt(plaintext)
	require.NoError(t, err)
	decrypted, err := Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}
