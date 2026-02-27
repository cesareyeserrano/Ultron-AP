// TC-3: Validate us-3 primary behavior
// Acceptance Criteria: AC-2
// AC-2: Given the current dark dashboard, when UX/UI stabilization is applied, then the dark theme remains, the interface shows improved visual depth/dynamism, and at least one icon-color option and font-family option are documented for evaluation.
import { fr_3_the_dashboard_visual_design_must_keep_the_cur } from "../../../src/contracts/fr-3-the-dashboard-visual-design-must.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_3_validate_us_3_primary_behavior", async () => {
  const report = await fr_3_the_dashboard_visual_design_must_keep_the_cur({
    keepsDarkTheme: true,
    depthScore: 0.85,
    dynamismScore: 0.82,
    flatnessScore: 0.2,
  });
  assert.equal(report.keepsDarkTheme, true);
  assert.equal(report.improvesDepthAndDynamism, true);
  assert.equal(report.avoidsFlatVisual, true);
});
