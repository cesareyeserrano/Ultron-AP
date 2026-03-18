/**
 * FR-1: The system must present critical monitoring status clearly at first glance and keep live dashboard updates reliable with explicit degraded-state signaling.
 */
export async function fr_1_the_system_must_present_critical_monitoring_status_clearly_at_first_glance_and_keep_live_dashboard_updates_reliable_with_explicit_degraded_state_signaling(input) {
  const {
    criticalStatusVisibleFirstViewport = false,
    correctiveNavigationInteractions = Number.POSITIVE_INFINITY,
    liveUpdatesReliable = false,
    staleStateShowsTimestamp = false,
    staleStateHasRecoveryPath = false,
    inputValidationWhitelistFirst = false,
    reconnectRateLimitEnabled = false,
    securityRejectsAreAudited = false,
  } = input ?? {};

  return {
    fr: "FR-1",
    criticalStatusVisibleFirstViewport:
      criticalStatusVisibleFirstViewport === true,
    correctiveNavigationWithinTwoInteractions:
      Number(correctiveNavigationInteractions) <= 2,
    liveUpdatesReliable: liveUpdatesReliable === true,
    degradedStateSignaling:
      staleStateShowsTimestamp === true && staleStateHasRecoveryPath === true,
    abuseControlsPresent:
      inputValidationWhitelistFirst === true &&
      reconnectRateLimitEnabled === true &&
      securityRejectsAreAudited === true,
  };
}

export const fr_1_the_system_must_present_critical_monitoring_s =
  fr_1_the_system_must_present_critical_monitoring_status_clearly_at_first_glance_and_keep_live_dashboard_updates_reliable_with_explicit_degraded_state_signaling;
