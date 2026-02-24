// TC-7: Handle edge behavior - User sends repeated apply clicks while one apply is still running
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_hardware_settings_updates_must_be_explicitly_ } from "../../../src/contracts/fr-1-hardware-settings-updates-must-b.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_7_handle_edge_behavior_user_sends_repeated_apply_clicks_while_one_apply_is_still_running", async () => {
  const report = await fr_1_hardware_settings_updates_must_be_explicitly_({
    hardwareTemplatePath: "web/templates/hardware.html",
    hardwarePartialPath: "web/templates/partials/hardware-form.html",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
    helperPath: "cmd/ultron-helper/main.go",
    systemHandlerPath: "internal/server/handlers_system.go",
  });
  assert.equal(report.singleFlightApply, true);
});
