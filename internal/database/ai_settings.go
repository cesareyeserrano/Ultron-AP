// Module: database/ai_settings
// Purpose: Persist the single-row AI provider configuration for the ai-insights
//          feature, storing the provider API key encrypted at rest via the
//          shared AES-GCM secrets mechanism (FR-017).
// Dependencies: database/sql, internal secrets helpers (secrets.go).
package database

import (
	"database/sql"
	"fmt"
)

// DefaultAITimeoutMS is the default per-request bound for an AI explanation
// (NFR-006). It is also the value seeded into a fresh ai_settings row.
const DefaultAITimeoutMS = 10000

// AISettings is the decrypted, in-memory view of the AI provider configuration.
// APIKey is plaintext here and MUST never be logged or returned to a client; it
// is only ever encrypted on save and decrypted on read.
type AISettings struct {
	Enabled       bool
	EndpointURL   string
	Model         string
	APIKey        string
	TelegramPush  bool
	TimeoutMS     int
	AllowInsecure bool
}

// DefaultAISettings returns the zero-value configuration: AI disabled.
func DefaultAISettings() AISettings {
	return AISettings{TimeoutMS: DefaultAITimeoutMS}
}

// GetAISettings reads the single ai_settings row and returns it with the API key
// decrypted. When no row exists it returns the disabled default (the panel then
// behaves exactly as without AI — FR-019).
//
// @aitri-trace FR-ID: FR-017, US-ID: US-017, AC-ID: AC-017-1h, TC-ID: TC-AI-017h
func (db *DB) GetAISettings() (AISettings, error) {
	cfg := DefaultAISettings()
	var enabled, push, insecure int
	var keyEnc string
	err := db.QueryRow(`SELECT enabled, endpoint_url, model, api_key_enc, telegram_push, timeout_ms, allow_insecure
		FROM ai_settings WHERE id = 1`).Scan(
		&enabled, &cfg.EndpointURL, &cfg.Model, &keyEnc, &push, &cfg.TimeoutMS, &insecure,
	)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("cannot get ai settings: %w", err)
	}
	cfg.Enabled = enabled == 1
	cfg.TelegramPush = push == 1
	cfg.AllowInsecure = insecure == 1
	if keyEnc != "" {
		dec, derr := decryptSecret(keyEnc)
		if derr != nil {
			return cfg, fmt.Errorf("cannot decrypt ai api key: %w", derr)
		}
		cfg.APIKey = dec
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = DefaultAITimeoutMS
	}
	return cfg, nil
}

// AIKeyIsSet reports whether a non-empty API key is stored, without decrypting or
// exposing it — used by GET /api/settings/ai to render api_key_set (FR-017).
func (db *DB) AIKeyIsSet() (bool, error) {
	var keyEnc string
	err := db.QueryRow(`SELECT api_key_enc FROM ai_settings WHERE id = 1`).Scan(&keyEnc)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read ai api key state: %w", err)
	}
	return keyEnc != "", nil
}

// SaveAISettings upserts the single ai_settings row, encrypting the API key at
// rest. An empty APIKey leaves whatever key was previously stored untouched (so a
// GET-mask-then-POST round trip does not wipe the key). The sentinel "__clear__"
// deletes the stored key.
//
// @aitri-trace FR-ID: FR-017, US-ID: US-017, AC-ID: AC-017-1h, TC-ID: TC-AI-017h
func (db *DB) SaveAISettings(cfg AISettings) error {
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = DefaultAITimeoutMS
	}
	keyEnc, err := db.resolveAIKeyCiphertext(cfg.APIKey)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO ai_settings
		(id, enabled, endpoint_url, model, api_key_enc, telegram_push, timeout_ms, allow_insecure, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			enabled=excluded.enabled,
			endpoint_url=excluded.endpoint_url,
			model=excluded.model,
			api_key_enc=excluded.api_key_enc,
			telegram_push=excluded.telegram_push,
			timeout_ms=excluded.timeout_ms,
			allow_insecure=excluded.allow_insecure,
			updated_at=CURRENT_TIMESTAMP`,
		boolToInt(cfg.Enabled), cfg.EndpointURL, cfg.Model, keyEnc,
		boolToInt(cfg.TelegramPush), cfg.TimeoutMS, boolToInt(cfg.AllowInsecure),
	)
	if err != nil {
		return fmt.Errorf("cannot save ai settings: %w", err)
	}
	return nil
}

// resolveAIKeyCiphertext decides what to store in api_key_enc given the incoming
// plaintext key: "" keeps the existing ciphertext, "__clear__" wipes it, anything
// else is encrypted.
func (db *DB) resolveAIKeyCiphertext(plain string) (string, error) {
	switch plain {
	case "":
		var existing string
		err := db.QueryRow(`SELECT api_key_enc FROM ai_settings WHERE id = 1`).Scan(&existing)
		if err != nil && err != sql.ErrNoRows {
			return "", fmt.Errorf("cannot read existing ai api key: %w", err)
		}
		return existing, nil
	case "__clear__":
		return "", nil
	default:
		enc, err := encryptSecret(plain)
		if err != nil {
			return "", fmt.Errorf("cannot encrypt ai api key: %w", err)
		}
		return enc, nil
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
