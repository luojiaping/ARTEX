package credhunt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadOrCreateKey returns a 32-byte AES key from path, creating it with 0600 on
// first use. The key never leaves the server.
func loadOrCreateKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("credhunt key %s: want 32 bytes, got %d", path, len(b))
		}
		return b, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// Lost a race: another process created it; read theirs.
		if b, rerr := os.ReadFile(path); rerr == nil && len(b) == 32 {
			return b, nil
		}
		return nil, err
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return nil, err
	}
	return key, f.Close()
}

// encrypt seals plaintext with AES-256-GCM. Output = nonce || ciphertext.
func encrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decrypt(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// normalizeSecret canonicalizes JSON packages and trims whitespace so the same
// credential found twice hashes identically.
func normalizeSecret(value string) string {
	v := strings.TrimSpace(value)
	if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
		var anyv any
		if json.Unmarshal([]byte(v), &anyv) == nil {
			if b, err := json.Marshal(anyv); err == nil {
				return string(b)
			}
		}
	}
	return v
}

// fingerprint binds a credential value to the server key. Equal values produce
// equal fingerprints; the raw value cannot be correlated without the key.
func fingerprint(key []byte, kind, value string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(normalizeSecret(value)))
	return hex.EncodeToString(mac.Sum(nil))
}

// uidFor derives an opaque stable id from a fingerprint.
func uidFor(fp string) string {
	if len(fp) > 32 {
		fp = fp[:32]
	}
	return "crd_" + fp
}

// maskSecret keeps a short recognizable prefix/suffix. Short values expose only
// the first two characters.
func maskSecret(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if len(v) <= 10 {
		return v[:min(2, len(v))] + "…"
	}
	tail := 4
	if len(v) > 80 {
		tail = 6
	}
	return v[:6] + "…" + v[len(v)-tail:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
