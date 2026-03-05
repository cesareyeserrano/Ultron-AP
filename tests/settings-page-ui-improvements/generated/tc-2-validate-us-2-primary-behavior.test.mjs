// TC-2: Validate us-2 primary behavior
// Acceptance Criteria: AC-3
// AC-3: Given an authenticated administrator has typed the required confirmation word for shutdown or restart, when the action is submitted, then a visible countdown cancel window is shown before execution and the action can be canceled during that window.
import { fr_2_the_system_must_protect_dangerous_actions_shu } from "../../../src/contracts/fr-2-the-system-must-protect-dangerou.js";
import test from "node:test";
import assert from "node:assert/strict";

test("tc_2_validate_us_2_primary_behavior", async () => {
  const result = await fr_2_the_system_must_protect_dangerous_actions_shu();
  assert.equal(result.ok, true, `FR-2 contract failed: ${result.missing.join(", ")}`);
});
