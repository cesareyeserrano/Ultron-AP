/**
 * FR-3: The system must use existing Ultron design tokens and components by default, and any external UI asset is allowed only if it remains lightweight and does not degrade Raspberry Pi performance.
 */
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export async function fr_3_the_system_must_use_existing_ultron_design_to(input) {
  void input;
  const here = dirname(fileURLToPath(import.meta.url));
  const templatePath = join(here, "../../web/templates/settings.html");
  const cssPath = join(here, "../../web/static/css/app.css");
  const html = await readFile(templatePath, "utf8");
  const css = await readFile(cssPath, "utf8");

  const externalScriptRef = /<script[^>]+src=["']https?:\/\//i.test(html);
  const heavyFrameworkHint = /\b(react|vue|angular|svelte)\b/i.test(html);

  const checks = [
    ["uses existing component patterns", html.includes("panel-soft") && html.includes("data-settings-section")],
    ["uses local css variables/tokens", css.includes("--color-base:") && css.includes("--font-sans:")],
    ["no external script CDN dependency", !externalScriptRef],
    ["no heavy frontend framework hints", !heavyFrameworkHint],
  ];

  const missing = checks.filter(([, ok]) => !ok).map(([name]) => name);
  return {
    ok: missing.length === 0,
    checked: checks.length,
    missing,
  };
}
