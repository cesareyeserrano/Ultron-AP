// TC-5: Handle edge behavior - On small screens or low-performance devices, key indicators can become hidden, truncated, or delayed, making critical status unreadable
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_the_system_must_present_the_most_critical_das } from "../../../src/contracts/fr-1-the-system-must-present-the-most.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_5_handle_edge_behavior_on_small_screens_or_low_performance_devices_key_indicators_can_become_hidden_truncated_or_delayed_making_critical_status_unreadable", async () => {
  const report = await fr_1_the_system_must_present_the_most_critical_das({
    indicatorsInFirstViewport: true,
    navigationInteractions: 2,
    supportsDesktop: true,
    supportsMobile: true,
    baselineRegression: false,
    smallScreenIndicatorsReadable: true,
    lowPerformanceDelayMs: 250,
  });
  assert.equal(report.handlesLowPerformanceSmallScreens, true);
});
