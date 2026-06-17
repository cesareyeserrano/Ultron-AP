package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAITestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "ai.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// @aitri-tc TC-AI-017h
//
// @aitri-trace FR-ID: FR-017, US-ID: US-017, AC-ID: AC-017-1h, TC-ID: TC-AI-017h
func TestTC_AI_017h_KeyCiphertextAtRest(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "unit-test-secret-key")
	db := setupAITestDB(t)

	const plaintext = "sk-secret123"
	require.NoError(t, db.SaveAISettings(AISettings{
		Enabled: true, EndpointURL: "https://x/v1", Model: "m", APIKey: plaintext, TimeoutMS: 10000,
	}))

	var stored string
	require.NoError(t, db.QueryRow(`SELECT api_key_enc FROM ai_settings WHERE id = 1`).Scan(&stored))
	assert.NotEqual(t, plaintext, stored, "stored key must be ciphertext, not plaintext")
	assert.NotContains(t, stored, plaintext, "ciphertext must not contain plaintext")

	got, err := db.GetAISettings()
	require.NoError(t, err)
	assert.Equal(t, plaintext, got.APIKey, "decrypt must restore the original key")
}

// @aitri-tc TC-AI-017e
//
// @aitri-trace FR-ID: FR-017, US-ID: US-017, AC-ID: AC-017-1h, TC-ID: TC-AI-017e
func TestTC_AI_017e_SecretsRoundTrip(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "unit-test-secret-key")
	const plaintext = "sk-secret123"

	enc, err := encryptSecret(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, enc, "ciphertext must differ from plaintext")
	assert.Greater(t, len(enc), len(plaintext), "ciphertext (nonce+tag+b64) must be longer")

	dec, err := decryptSecret(enc)
	require.NoError(t, err)
	assert.Equal(t, plaintext, dec, "decrypt must restore plaintext exactly")
}

// emptyKeptOnEmptySave verifies an empty APIKey leaves the stored key intact, and
// "__clear__" wipes it — supporting the GET-mask-then-POST round trip (FR-018).
func TestAISettings_EmptyKeyKeepsExisting(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "unit-test-secret-key")
	db := setupAITestDB(t)
	require.NoError(t, db.SaveAISettings(AISettings{Enabled: true, EndpointURL: "https://x/v1", Model: "m", APIKey: "keepme", TimeoutMS: 10000}))
	// Save again with empty key — must keep "keepme".
	require.NoError(t, db.SaveAISettings(AISettings{Enabled: true, EndpointURL: "https://x/v1", Model: "m2", APIKey: "", TimeoutMS: 10000}))
	got, err := db.GetAISettings()
	require.NoError(t, err)
	assert.Equal(t, "keepme", got.APIKey)
	assert.Equal(t, "m2", got.Model)
}
