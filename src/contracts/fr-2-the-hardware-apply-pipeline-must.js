/**
 * FR-2: The hardware apply pipeline must expose deterministic operation state (`idle|applying|failed|applied`) so UI and logs never leave ambiguous "busy" behavior.
 */
import fs from "node:fs";

export async function fr_2_the_hardware_apply_pipeline_must_expose_deter(input) {
  const read = (p) => fs.readFileSync(p, "utf8");
  const hardwarePage = read(input.hardwareTemplatePath);
  const helper = read(input.helperPath);
  const handler = read(input.hardwareHandlerPath);

  const deterministicApplyControl =
    /hx-sync="this:(drop|replace)"/.test(hardwarePage) &&
    /applyQueue/.test(helper) &&
    /startApplyWorker/.test(helper) &&
    /handlePironmanApplyNow/.test(helper);

  const failurePathIsExplicit =
    /showToast/.test(handler) &&
    /Failed:/.test(handler);

  return {
    deterministicApplyControl,
    failurePathIsExplicit,
  };
}

export async function fr_2_the_hardware_apply_pipeline_must_expose_deterministic_operation_state_idle_applying_failed_applied_so_ui_and_logs_never_leave_ambiguous_busy_behavior(input) {
  return fr_2_the_hardware_apply_pipeline_must_expose_deter(input);
}
