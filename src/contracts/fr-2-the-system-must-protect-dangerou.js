/**
 * FR-2: The system must protect dangerous actions (shutdown/restart) with a typed confirmation word in a dedicated confirmation field plus a short cancel window with a visible countdown animation, so accidental execution is prevented while keeping Ultron lightweight.
 */
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export async function fr_2_the_system_must_protect_dangerous_actions_shu(input) {
  void input;
  const here = dirname(fileURLToPath(import.meta.url));
  const templatePath = join(here, "../../web/templates/settings.html");
  const handlerPath = join(here, "../../internal/server/handlers_system.go");
  const html = await readFile(templatePath, "utf8");
  const go = await readFile(handlerPath, "utf8");

  const checks = [
    ["danger guard container exists", html.includes('id="danger-action-guard"')],
    ["typed confirmation field exists", html.includes('name="confirm_word"')],
    ["countdown indicator exists", html.includes('id="danger-countdown-bar"') && html.includes("Safety countdown:")],
    ["countdown ack field exists", html.includes('name="countdown_ack"')],
    ["danger action opener hooks exist", html.includes("data-danger-open") && html.includes("data-danger-action")],
    ["server validates dangerous action", go.includes("validateDangerousAction") && go.includes("confirm_word")],
    ["server audits dangerous-action rejects", go.includes(`"danger_action_reject"`)],
  ];

  const missing = checks.filter(([, ok]) => !ok).map(([name]) => name);
  return {
    ok: missing.length === 0,
    checked: checks.length,
    missing,
  };
}
