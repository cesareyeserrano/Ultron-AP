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
