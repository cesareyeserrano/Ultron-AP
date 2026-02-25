/**
 * FR-3: Integration with Pironman must prioritize stable local control path and avoid long blocking calls in web request lifecycle.
 */
import fs from "node:fs";

export async function fr_3_integration_with_pironman_must_prioritize_sta(input) {
  const read = (p) => (fs.existsSync(p) ? fs.readFileSync(p, "utf8") : "");
  const helper = read(input.helperPath);
  const hardwareHandler = read(input.hardwareHandlerPath);
  const pironmanControls = read(input.pironmanControlsPath);

  const stableHelperPath =
    /ULTRON_HELPER_SOCKET/.test(helper) &&
    /PironmanApply/.test(pironmanControls) &&
    /pironmanAPIBaseURL/.test(helper) &&
    !/exec\.Command\("pironman5"/.test(helper);

  const boundedExecution =
    /context\.WithTimeout\(context\.Background\(\), 20\*time\.Second\)/.test(pironmanControls) &&
    /context\.WithTimeout\(context\.Background\(\), 25\*time\.Second\)/.test(helper);

  const actionableFailure =
    (/showToast/.test(hardwareHandler) && /Failed:/.test(hardwareHandler)) ||
    (/apply failed after/.test(helper) && /timed out waiting for worker/.test(helper));

  return {
    stableHelperPath,
    boundedExecution,
    actionableFailure,
  };
}

export async function fr_3_integration_with_pironman_must_prioritize_stable_local_control_path_and_avoid_long_blocking_calls_in_web_request_lifecycle(input) {
  return fr_3_integration_with_pironman_must_prioritize_sta(input);
}
