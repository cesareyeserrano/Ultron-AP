// Module: ai/wiring
// Purpose: Production constructor that binds the AI service to the database for
//          config reads (FR-020) and supplies notification secret values to the
//          prompt redactor (FR-023). Kept separate so the core ai logic has no
//          hard database dependency in unit tests.
// Dependencies: internal/database.
package ai

import (
	"encoding/json"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// NewFromDB builds a Service whose config is read from the ai_settings row on each
// call and whose redactor also strips the operator's notification secrets.
func NewFromDB(db *database.DB, sources Sources, hc HTTPDoer) *Service {
	store := ConfigFunc(func() (Config, error) {
		c, err := db.GetAISettings()
		if err != nil {
			return Config{}, err
		}
		return Config{
			Enabled:       c.Enabled,
			EndpointURL:   c.EndpointURL,
			Model:         c.Model,
			APIKey:        c.APIKey,
			TelegramPush:  c.TelegramPush,
			TimeoutMS:     c.TimeoutMS,
			AllowInsecure: c.AllowInsecure,
		}, nil
	})
	return New(store, sources, hc, func() []string { return notificationSecrets(db) })
}

// notificationSecrets returns the secret-bearing values from the telegram/email
// notification configs so they are redacted before any prompt leaves the process.
func notificationSecrets(db *database.DB) []string {
	var out []string
	for _, ch := range []string{"telegram", "email"} {
		nc, err := db.GetNotificationConfig(ch)
		if err != nil || nc == nil {
			continue
		}
		var fields map[string]string
		if json.Unmarshal([]byte(nc.Config), &fields) != nil {
			continue
		}
		for k, v := range fields {
			if v == "" {
				continue
			}
			lk := strings.ToLower(k)
			if strings.Contains(lk, "token") || strings.Contains(lk, "password") || strings.Contains(lk, "pass") || strings.Contains(lk, "secret") {
				out = append(out, v)
			}
		}
	}
	return out
}
