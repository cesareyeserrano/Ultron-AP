/**
 * FR-4: Privileged execution must stay outside web process, with strict local boundary, validated command parameters, and auditable outcomes.
 */
import fs from "node:fs";

export async function fr_4_privileged_execution_must_stay_outside_web_pr(input) {
  const read = (p) => (fs.existsSync(p) ? fs.readFileSync(p, "utf8") : "");
  const helperService = read(input.helperServicePath);
  const webService = read(input.webServicePath);
  const systemHandler = read(input.systemHandlerPath);
  const helper = read(input.helperPath);

  const isolatedBoundary =
    /User=root/.test(helperService) &&
    /NoNewPrivileges=true/.test(webService);

  const noDirectPrivilegeInWeb = !/exec\.Command\("sudo"/.test(systemHandler);

  const validatedAndAuditable =
    /serviceNameRe/.test(helper) &&
    /invalid service name/.test(helper);

  return {
    isolatedBoundary,
    noDirectPrivilegeInWeb,
    validatedAndAuditable,
  };
}

export async function fr_4_privileged_execution_must_stay_outside_web_process_with_strict_local_boundary_validated_command_parameters_and_auditable_outcomes(input) {
  return fr_4_privileged_execution_must_stay_outside_web_pr(input);
}
