// TC-3: Validate us-3 primary behavior
// Acceptance Criteria: AC-1, AC-3
// AC-1: Given an authenticated admin on hardware page, when fields are edited, then no apply request is sent until explicit apply action is triggered.
// AC-3: Given helper or Pironman timeout/error, when apply fails, then UI receives actionable failure status and system returns to operable idle state.
import { fr_3_integration_with_pironman_must_prioritize_sta } from "../../../src/contracts/fr-3-integration-with-pironman-must-p.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_3_validate_us_3_primary_behavior", async () => {
  const report = await fr_3_integration_with_pironman_must_prioritize_sta({
    helperPath: "cmd/ultron-helper/main.go",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
    pironmanControlsPath: "internal/pironman/controls.go",
  });
  assert.equal(report.stableHelperPath, true);
  assert.equal(report.boundedExecution, true);
  assert.equal(report.actionableFailure, true);
});
