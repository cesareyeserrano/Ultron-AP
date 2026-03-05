// TC-5: Enforce security control - All state-changing settings actions must require a valid authenticated session, CSRF protection, and same-origin validation, and all rejected dangerous-action attempts must be audit-logged
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_the_system_must_provide_a_complete_mobile_fir } from "../../../src/contracts/fr-1-the-system-must-provide-a-comple.js";
import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

test("tc_5_enforce_security_control_all_state_changing_settings_actions_must_require_a_valid_authenticated_session_csrf_protection_and_same_origin_validation_and_all_rejected_dangerous_action_attempts_must_be_audit_logged", async () => {
  const base = dirname(fileURLToPath(import.meta.url));
  const settingsHandlerPath = join(base, "../../../internal/server/handlers_settings.go");
  const systemHandlerPath = join(base, "../../../internal/server/handlers_system.go");
  const settingsGo = await readFile(settingsHandlerPath, "utf8");
  const systemGo = await readFile(systemHandlerPath, "utf8");
  const fr = await fr_1_the_system_must_provide_a_complete_mobile_fir();

  assert.equal(fr.ok, true, "FR-1 precondition should pass");
  assert.equal(settingsGo.includes("validateCSRF"), true, "settings handlers should enforce CSRF/session/origin checks");
  assert.equal(settingsGo.includes(`"csrf_reject"`), true, "csrf rejects should be audited");
  assert.equal(systemGo.includes("validateDangerousAction"), true, "dangerous actions should have dedicated server-side validation");
  assert.equal(systemGo.includes(`"danger_action_reject"`), true, "dangerous action rejects should be audited");
});
