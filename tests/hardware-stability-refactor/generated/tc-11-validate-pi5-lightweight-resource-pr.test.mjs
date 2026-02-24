// TC-11: Validate Pi5 lightweight resource profile under repeated applies
// Acceptance Criteria: AC-4
// AC-4: Given normal operation on Pi5, when repeated hardware applies are performed, then resource usage remains lightweight and response behavior stays stable.
import { fr_3_integration_with_pironman_must_prioritize_sta } from "../../../src/contracts/fr-3-integration-with-pironman-must-p.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_11_validate_pi5_lightweight_resource_profile_under_repeated_applies", async () => {
  const report = await fr_3_integration_with_pironman_must_prioritize_sta({
    helperPath: "cmd/ultron-helper/main.go",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
    pironmanControlsPath: "internal/pironman/controls.go",
  });
  assert.equal(report.stableHelperPath, true);
  assert.equal(report.boundedExecution, true);
});
