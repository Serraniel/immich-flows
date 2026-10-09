package connect

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadKey derives the encryption key from secret, or from a random key file in dataDir
// (created on first use) if secret is empty.
func LoadKey(secret, dataDir string) ([]byte, error) {
	if secret == "" {
		path := filepath.Join(dataDir, "secret.key")
		b, err := os.ReadFile(path) // #nosec G304 -- path from configuration
		switch {
		case err == nil:
			secret = strings.TrimSpace(string(b))
		case errors.Is(err, os.ErrNotExist):
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return nil, err
			}
			secret = hex.EncodeToString(raw)
			if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
				return nil, fmt.Errorf("write %s: %w", path, err)
			}
		default:
			return nil, err
		}
	}
	return hkdf.Key(sha256.New, []byte(secret), nil, "immich-flows api keys", 32)
}

func seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func open(key, ciphertext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
