// TC-3: Enforce security control - UI changes must preserve all existing CSRF/authentication flows and must not expose sensitive configuration values in visible states
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import (
	"os"
	"strings"
	"testing"
)

func TestTc3EnforceSecurityControlOnUiChanges(t *testing.T) {
	settingsPath := repoFile(t, "web", "templates", "settings.html")

	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read %s: %v", settingsPath, err)
	}

	settingsS := string(settings)

	if !strings.Contains(settingsS, "name=\"csrf_token\"") {
		t.Fatal("expected CSRF token field in settings forms")
	}

	// UI must not print raw secret values for sensitive fields.
	if strings.Contains(settingsS, "value=\"{{index .Content.Telegram.Fields \"bot_token\"}}\"") {
		t.Fatal("telegram bot_token must not be rendered as plain value")
	}
	if strings.Contains(settingsS, "value=\"{{index .Content.Email.Fields \"smtp_password\"}}\"") {
		t.Fatal("smtp_password must not be rendered as plain value")
	}
}
