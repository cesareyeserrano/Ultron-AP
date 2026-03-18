/**
 * FR-1: The system must provide a complete, mobile-first Settings UI where administrators can find, understand, and update configuration quickly through a compact, clearly structured interface with consistent components and feedback states.
 */
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export async function fr_1_the_system_must_provide_a_complete_mobile_fir(input) {
  void input;
  const here = dirname(fileURLToPath(import.meta.url));
  const templatePath = join(here, "../../web/templates/settings.html");
  const html = await readFile(templatePath, "utf8");

  const checks = [
    ["settings shell exists", html.includes('id="settings-shell"')],
    ["mobile-first compact stack exists", html.includes('class="space-y-3"') && html.includes("grid grid-cols-1")],
    ["accordion structure exists", html.includes("data-accordion-toggle") && html.includes("data-accordion-body") && html.includes("setAccordionOpen(")],
    ["legacy top/side navigation removed", !html.includes("data-settings-nav") && !html.includes("Settings Command Deck")],
    ["section anchors exist", html.includes('id="settings-alerts"') && html.includes('id="settings-controls"')],
    ["deterministic form states exist", html.includes("data-form-state-pill") && html.includes("setFormState(form, 'saving'")],
    ["field level validation feedback exists", html.includes("setFieldError(") && html.includes("data-field-error")],
  ];

  const missing = checks.filter(([, ok]) => !ok).map(([name]) => name);
  return {
    ok: missing.length === 0,
    checked: checks.length,
    missing,
  };
}
