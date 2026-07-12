package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const encPrefix = "enc:v1:"

// missingSecretKeyWarning is logged at startup (BL-007) when ULTRON_SECRET_KEY
// is not configured. Previously emitted lazily on the first notification write;
// surfacing it at boot ensures operators see it in the journal immediately,
// before any sensitive configuration has had a chance to land in plaintext.
const missingSecretKeyWarning = "SECURITY WARNING: ULTRON_SECRET_KEY is not set — saving notification secrets (Telegram bot token, SMTP password) will be REFUSED to avoid storing them in plaintext. Set ULTRON_SECRET_KEY before configuring notifications."

// errSecretKeyRequired is returned when a non-empty notification secret would
// be persisted without ULTRON_SECRET_KEY configured (BG-044). Previously such a
// write silently landed in plaintext; it is now refused.
var errSecretKeyRequired = fmt.Errorf("ULTRON_SECRET_KEY is not set: refusing to store notification secret in plaintext — set ULTRON_SECRET_KEY and retry")

// minSecretKeyLen is the shortest ULTRON_SECRET_KEY we consider adequate. The
// value is stretched with SHA-256, which does NOT add entropy — a short
// passphrase yields a brute-forceable AES-GCM key protecting the stored
// notification secrets (B2).
const minSecretKeyLen = 16

// WarnIfMissingSecretKey emits the plaintext-secrets warning to the standard
// logger if ULTRON_SECRET_KEY is not configured, or a weak-key warning if it is
// set but shorter than minSecretKeyLen. Call this once at startup.
func WarnIfMissingSecretKey() {
	raw := strings.TrimSpace(os.Getenv("ULTRON_SECRET_KEY"))
	if raw == "" {
		log.Println(missingSecretKeyWarning)
		return
	}
	if len(raw) < minSecretKeyLen {
		log.Printf("WARNING: ULTRON_SECRET_KEY is only %d characters; use at least %d random characters — SHA-256 does not add entropy to a short passphrase (B2)", len(raw), minSecretKeyLen)
	}
}

func secretKeyFromEnv() ([]byte, bool) {
	raw := strings.TrimSpace(os.Getenv("ULTRON_SECRET_KEY"))
	if raw == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], true
}

func encryptSecret(plain string) (string, error) {
	key, ok := secretKeyFromEnv()
	if !ok {
		// BG-044: refuse to persist a non-empty secret in plaintext. An empty
		// value (clearing the config) is still allowed so a channel can be
		// disabled/cleared without the key. Operators must set ULTRON_SECRET_KEY
		// before saving real secrets; the startup warning (BL-007) flags this.
		if strings.TrimSpace(plain) == "" {
			return plain, nil
		}
		return "", errSecretKeyRequired
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	raw := append(nonce, ct...)
	return encPrefix + base64.StdEncoding.EncodeToString(raw), nil
}

func decryptSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	key, ok := secretKeyFromEnv()
	if !ok {
		return "", fmt.Errorf("encrypted secret found but ULTRON_SECRET_KEY is not configured")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted payload")
	}
	nonce := data[:gcm.NonceSize()]
	ct := data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
