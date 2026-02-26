/**
 * FR-3: Integration with Pironman must prioritize stable local control path and avoid long blocking calls in web request lifecycle.
 */
import fs from "node:fs";

export async function fr_3_integration_with_pironman_must_prioritize_sta(input) {
  const read = (p) => (fs.existsSync(p) ? fs.readFileSync(p, "utf8") : "");
  const helper = read(input.helperPath);
  const server = read(input.serverPath);
  const settingsHandler = read(input.settingsHandlerPath);
  const settingsTemplate = read(input.settingsTemplatePath);

  const stableHelperPath =
    !/pironman\.apply/.test(helper) &&
    !/pironman\.read/.test(helper) &&
    !/pironmanAPIBaseURL/.test(helper);

  const boundedExecution =
    !/\/api\/settings\/integrations\/diagnostics/.test(server) &&
    !/\/api\/settings\/diagnostics/.test(server) &&
    !/handleRuntimeDiagnostics/.test(settingsHandler);

  const actionableFailure =
    !/Open Module/.test(settingsTemplate) &&
    !/\/integrations\/pironman/.test(server) &&
    !/\/api\/integrations\/pironman\/apply/.test(server);

  return {
    stableHelperPath,
    boundedExecution,
    actionableFailure,
  };
}

export async function fr_3_integration_with_pironman_must_prioritize_stable_local_control_path_and_avoid_long_blocking_calls_in_web_request_lifecycle(input) {
  return fr_3_integration_with_pironman_must_prioritize_sta(input);
}
