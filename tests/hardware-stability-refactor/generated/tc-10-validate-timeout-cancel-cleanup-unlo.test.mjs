// TC-10: Validate timeout/cancel cleanup unlock behavior
// Acceptance Criteria: AC-2, AC-3
// AC-2: Given one apply operation in progress, when another apply is requested, then system handles it deterministically without stuck-busy state.
// AC-3: Given external hardware stack instability, Ultron remains operable with no in-app control path.
import { fr_2_the_hardware_apply_pipeline_must_expose_deter } from "../../../src/contracts/fr-2-the-hardware-apply-pipeline-must.js";
import { fr_3_integration_with_pironman_must_prioritize_sta } from "../../../src/contracts/fr-3-integration-with-pironman-must-p.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_10_validate_timeout_cancel_cleanup_unlock_behavior", async () => {
  const fr2 = await fr_2_the_hardware_apply_pipeline_must_expose_deter({
    hardwareTemplatePath: "web/templates/hardware.html",
    helperPath: "cmd/ultron-helper/main.go",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
    serverPath: "internal/server/server.go",
  });
  const fr3 = await fr_3_integration_with_pironman_must_prioritize_sta({
    helperPath: "cmd/ultron-helper/main.go",
    serverPath: "internal/server/server.go",
    settingsHandlerPath: "internal/server/handlers_settings.go",
    settingsTemplatePath: "web/templates/settings.html",
  });
  assert.equal(fr2.deterministicApplyControl, true);
  assert.equal(fr3.boundedExecution, true);
  assert.equal(fr3.actionableFailure, true);
});
