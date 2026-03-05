// TC-4: Handle edge behavior - On small mobile screens, long setting labels, helper text, and validation errors can overlap or push critical controls off-screen, causing accidental taps or missed confirmations
// Acceptance Criteria: none
// No AC mapped to this TC.
import { fr_1_the_system_must_provide_a_complete_mobile_fir } from "../../../src/contracts/fr-1-the-system-must-provide-a-comple.js";
import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

test("tc_4_handle_edge_behavior_on_small_mobile_screens_long_setting_labels_helper_text_and_validation_errors_can_overlap_or_push_critical_controls_off_screen_causing_accidental_taps_or_missed_confirmations", async () => {
  const base = dirname(fileURLToPath(import.meta.url));
  const templatePath = join(base, "../../../web/templates/settings.html");
  const html = await readFile(templatePath, "utf8");
  const fr = await fr_1_the_system_must_provide_a_complete_mobile_fir();

  assert.equal(fr.ok, true, `FR-1 baseline missing: ${fr.missing.join(", ")}`);
  assert.equal(html.includes("min-h-[2rem]"), true, "status containers should reserve height to avoid jump/overlap");
  assert.equal(html.includes("clearFieldErrors("), true, "inline validation error reset should exist");
  assert.equal(html.includes("setFieldError("), true, "inline field error mapping should exist");
  assert.equal(html.includes("grid grid-cols-1"), true, "mobile-first single-column grid should exist");
});
