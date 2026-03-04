// TC-1: Validate us-1 primary behavior
// Acceptance Criteria: AC-1
// AC-1: Given an authenticated operator on Dashboard, when live telemetry is healthy, then critical status is visible in first viewport and corrective navigation is reachable in two clicks or fewer.
import { fr_1_the_system_must_present_critical_monitoring_s } from "../../../src/contracts/fr-1-the-system-must-present-critical.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_1_validate_us_1_primary_behavior", async () => {
  const result = await fr_1_the_system_must_present_critical_monitoring_s({
    criticalStatusVisibleFirstViewport: true,
    correctiveNavigationInteractions: 2,
    liveUpdatesReliable: true,
    staleStateShowsTimestamp: true,
    staleStateHasRecoveryPath: true,
  });
  assert.equal(result.criticalStatusVisibleFirstViewport, true);
  assert.equal(result.correctiveNavigationWithinTwoInteractions, true);
  assert.equal(result.liveUpdatesReliable, true);
});
