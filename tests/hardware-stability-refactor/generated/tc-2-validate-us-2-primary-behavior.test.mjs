// TC-2: Validate us-2 primary behavior
// Acceptance Criteria: AC-2
// AC-2: Given one apply operation in progress, when another apply is requested, then system handles it deterministically without stuck-busy state.
import { fr_2_the_hardware_apply_pipeline_must_expose_deter } from "../../../src/contracts/fr-2-the-hardware-apply-pipeline-must.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_2_validate_us_2_primary_behavior", async () => {
  const report = await fr_2_the_hardware_apply_pipeline_must_expose_deter({
    hardwareTemplatePath: "web/templates/hardware.html",
    helperPath: "cmd/ultron-helper/main.go",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
  });
  assert.equal(report.deterministicApplyControl, true);
  assert.equal(report.failurePathIsExplicit, true);
});
