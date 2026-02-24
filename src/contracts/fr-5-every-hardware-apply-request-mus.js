/**
 * FR-5: Every hardware apply request must produce traceable telemetry (request id, duration, result, error cause) for incident diagnosis.
 */
import fs from "node:fs";

export async function fr_5_every_hardware_apply_request_must_produce_tra(input) {
  const read = (p) => fs.readFileSync(p, "utf8");
  const helper = read(input.helperPath);
  const hardwareHandler = read(input.hardwareHandlerPath);

  const durationTelemetry =
    /apply completed in/.test(helper) &&
    /apply failed after/.test(helper);

  const resultAndErrorPath =
    /showToast/.test(hardwareHandler) &&
    /Failed:/.test(hardwareHandler) &&
    /Hardware settings applied/.test(hardwareHandler);

  return {
    durationTelemetry,
    resultAndErrorPath,
  };
}

export async function fr_5_every_hardware_apply_request_must_produce_traceable_telemetry_request_id_duration_result_error_cause_for_incident_diagnosis(input) {
  return fr_5_every_hardware_apply_request_must_produce_tra(input);
}
