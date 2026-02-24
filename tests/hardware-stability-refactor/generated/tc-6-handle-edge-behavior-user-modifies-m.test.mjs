// TC-6: Handle edge behavior - User modifies many controls quickly before pressing apply
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_hardware_settings_updates_must_be_explicitly_ } from "../../../src/contracts/fr-1-hardware-settings-updates-must-b.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_6_handle_edge_behavior_user_modifies_many_controls_quickly_before_pressing_apply", async () => {
  const report = await fr_1_hardware_settings_updates_must_be_explicitly_({
    hardwareTemplatePath: "web/templates/hardware.html",
    hardwarePartialPath: "web/templates/partials/hardware-form.html",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
    helperPath: "cmd/ultron-helper/main.go",
    systemHandlerPath: "internal/server/handlers_system.go",
  });
  assert.equal(report.explicitApplyOnly, true);
  assert.equal(report.singleFlightApply, true);
});
