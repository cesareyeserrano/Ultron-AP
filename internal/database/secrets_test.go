package database

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func captureLog(fn func()) string {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)
	fn()
	return buf.String()
}

func TestWarnIfMissingSecretKey_EmitsWhenUnset(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "")

	out := captureLog(WarnIfMissingSecretKey)

	assert.Contains(t, out, "ULTRON_SECRET_KEY is not set")
	assert.Contains(t, out, "plaintext")
}

func TestWarnIfMissingSecretKey_SilentWhenSet(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "anything-non-empty")

	out := captureLog(WarnIfMissingSecretKey)

	assert.Empty(t, strings.TrimSpace(out))
}

func TestWarnIfMissingSecretKey_TreatsWhitespaceAsUnset(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "   ")

	out := captureLog(WarnIfMissingSecretKey)

	assert.Contains(t, out, "ULTRON_SECRET_KEY is not set")
}

// BG-044: a non-empty secret must NOT be persisted in plaintext when
// ULTRON_SECRET_KEY is unset — the write is refused.
func TestEncryptSecret_RefusesPlaintextWhenKeyUnset(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "")
	_, err := encryptSecret(`{"bot_token":"123:abc","chat_id":"42"}`)
	assert.Error(t, err, "non-empty secret must be refused without ULTRON_SECRET_KEY")
}

// An empty value (clearing config) is still allowed without the key.
func TestEncryptSecret_AllowsEmptyWhenKeyUnset(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "")
	out, err := encryptSecret("")
	assert.NoError(t, err)
	assert.Equal(t, "", out)
}

// With the key set, the secret is encrypted (enc:v1: prefix) and round-trips.
func TestEncryptSecret_EncryptsAndRoundTripsWhenKeySet(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "a-strong-test-key")
	plain := `{"bot_token":"123:abc"}`
	enc, err := encryptSecret(plain)
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(enc, encPrefix), "stored secret must be encrypted")
	assert.NotContains(t, enc, "123:abc", "plaintext token must not appear in stored value")

	dec, err := decryptSecret(enc)
	assert.NoError(t, err)
	assert.Equal(t, plain, dec)
}
