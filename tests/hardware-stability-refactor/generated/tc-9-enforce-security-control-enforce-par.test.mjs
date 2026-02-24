// TC-9: Enforce security control - Enforce parameter allowlists and input validation before any privileged action
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_hardware_settings_updates_must_be_explicitly_ } from "../../../src/contracts/fr-1-hardware-settings-updates-must-b.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_9_enforce_security_control_enforce_parameter_allowlists_and_input_validation_before_any_privileged_action", async () => {
  const report = await fr_1_hardware_settings_updates_must_be_explicitly_({
    hardwareTemplatePath: "web/templates/hardware.html",
    hardwarePartialPath: "web/templates/partials/hardware-form.html",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
    helperPath: "cmd/ultron-helper/main.go",
    systemHandlerPath: "internal/server/handlers_system.go",
  });
  assert.equal(report.parameterValidation, true);
});
