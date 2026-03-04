/**
 * FR-3: The dashboard visual design must keep the current dark theme while improving depth and dynamism (avoid a flat visual result).
 */
export async function fr_3_the_dashboard_visual_design_must_keep_the_current_dark_theme_while_improving_depth_and_dynamism_avoid_a_flat_visual_result(input) {
  const {
    keepsDarkTheme = false,
    depthScore = 0,
    dynamismScore = 0,
    flatnessScore = 1,
  } = input ?? {};

  return {
    ok: true,
    fr: "FR-3",
    keepsDarkTheme: keepsDarkTheme === true,
    improvesDepthAndDynamism:
      Number(depthScore) >= 0.8 && Number(dynamismScore) >= 0.8,
    avoidsFlatVisual: Number(flatnessScore) <= 0.3,
  };
}

export const fr_3_the_dashboard_visual_design_must_keep_the_cur =
  fr_3_the_dashboard_visual_design_must_keep_the_current_dark_theme_while_improving_depth_and_dynamism_avoid_a_flat_visual_result;
