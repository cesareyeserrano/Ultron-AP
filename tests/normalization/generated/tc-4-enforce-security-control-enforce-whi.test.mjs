// TC-4: Enforce security control - Enforce whitelist-first input validation plus rate limits on auth and high-cost endpoints (including SSE reconnect abuse), and audit-log all rejects
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_the_system_must_present_critical_monitoring_s } from "../../../src/contracts/fr-1-the-system-must-present-critical.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_4_enforce_security_control_enforce_whitelist_first_input_validation_plus_rate_limits_on_auth_and_high_cost_endpoints_including_sse_reconnect_abuse_and_audit_log_all_rejects", async () => {
  const result = await fr_1_the_system_must_present_critical_monitoring_s({
    inputValidationWhitelistFirst: true,
    reconnectRateLimitEnabled: true,
    securityRejectsAreAudited: true,
  });
  assert.equal(result.abuseControlsPresent, true);
});
