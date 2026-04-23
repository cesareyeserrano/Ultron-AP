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
	"sync"
)

const encPrefix = "enc:v1:"

// missingKeyWarnOnce guarantees the operator sees the "secrets at rest are plaintext"
// warning exactly once per process start — enough to be noticed in logs without
// spamming on every write path.
var missingKeyWarnOnce sync.Once

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
		// BG-001: previously this fell back to plaintext silently, so an operator
		// who forgot to set ULTRON_SECRET_KEY would persist Telegram bot tokens
		// and SMTP passwords unencrypted with no signal. Keep the backward-compatible
		// behavior (still writes plaintext so notification config continues to work),
		// but surface a loud, once-per-process warning to the log.
		missingKeyWarnOnce.Do(func() {
			log.Println("SECURITY WARNING: ULTRON_SECRET_KEY is not set — notification secrets (Telegram bot token, SMTP password) will be stored in the database in plaintext. Set ULTRON_SECRET_KEY before persisting sensitive configuration.")
		})
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
