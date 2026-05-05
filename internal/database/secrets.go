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
const missingSecretKeyWarning = "SECURITY WARNING: ULTRON_SECRET_KEY is not set — notification secrets (Telegram bot token, SMTP password) will be stored in the database in plaintext. Set ULTRON_SECRET_KEY before persisting sensitive configuration."

// WarnIfMissingSecretKey emits the plaintext-secrets warning to the standard
// logger if ULTRON_SECRET_KEY is not configured. Call this once at startup.
func WarnIfMissingSecretKey() {
	if _, ok := secretKeyFromEnv(); !ok {
		log.Println(missingSecretKeyWarning)
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
		// Backward-compatible fallback: still write plaintext so notification
		// config continues to work for operators who haven't migrated. The loud
		// warning is surfaced at startup via WarnIfMissingSecretKey (BL-007).
		return plain, nil
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
