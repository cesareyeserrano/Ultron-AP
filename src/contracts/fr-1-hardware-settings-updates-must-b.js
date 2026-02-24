/**
 * FR-1: Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.
 */
import fs from "node:fs";

export async function fr_1_hardware_settings_updates_must_be_explicitly_(input) {
  const read = (p) => fs.readFileSync(p, "utf8");
  const hardwarePage = read(input.hardwareTemplatePath);
  const hardwareForm = read(input.hardwarePartialPath);
  const hardwareHandler = read(input.hardwareHandlerPath);
  const helper = read(input.helperPath);
  const systemHandler = read(input.systemHandlerPath);

  const explicitApplyOnly =
    /hx-post="\/api\/hardware\/apply"/.test(hardwarePage) &&
    /type="submit"/.test(hardwareForm) &&
    !/hx-trigger="change"/.test(hardwarePage);

  const singleFlightApply =
    /hx-sync="this:(drop|replace)"/.test(hardwarePage) &&
    /applyQueue/.test(helper) &&
    /startApplyWorker/.test(helper);

  const noDirectWebPrivilege =
    !/exec\.Command\("sudo"/.test(systemHandler) &&
    !/exec\.Command\("sudo"/.test(hardwareHandler);

  const parameterValidation =
    /sanitizeHex\(/.test(hardwareHandler) &&
    /sanitizeStyle\(/.test(hardwareHandler) &&
    /sanitizeFanLED\(/.test(hardwareHandler) &&
    /sanitizeRotation\(/.test(hardwareHandler);

  return {
    explicitApplyOnly,
    singleFlightApply,
    noDirectWebPrivilege,
    parameterValidation,
  };
}

export async function fr_1_hardware_settings_updates_must_be_explicitly_user_triggered_no_auto_apply_on_field_change_and_only_one_apply_operation_can_execute_at_a_time(input) {
  return fr_1_hardware_settings_updates_must_be_explicitly_(input);
}
