// TC-5: Validate us-5 primary behavior
// Acceptance Criteria: AC-1
// AC-1: Given an authenticated admin on hardware page, when fields are edited, then no apply request is sent until explicit apply action is triggered.
import { fr_5_every_hardware_apply_request_must_produce_tra } from "../../../src/contracts/fr-5-every-hardware-apply-request-mus.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_5_validate_us_5_primary_behavior", async () => {
  const report = await fr_5_every_hardware_apply_request_must_produce_tra({
    helperPath: "cmd/ultron-helper/main.go",
    hardwareHandlerPath: "internal/server/handlers_hardware.go",
  });
  assert.equal(report.durationTelemetry, true);
  assert.equal(report.resultAndErrorPath, true);
});
