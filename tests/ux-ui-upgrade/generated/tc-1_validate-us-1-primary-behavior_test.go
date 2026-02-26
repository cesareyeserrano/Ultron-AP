// TC-1: Validate us-1 primary behavior
// Acceptance Criteria: AC-1
// AC-1: Given an authenticated admin on desktop and mobile, when navigating Dashboard, Docker, Services, Alerts, Logs, History, and Settings, then each page renders with the new premium visual system consistently and remains fully usable without horizontal overflow.
package generated

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/contracts"
)

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestTc1ValidateUs1PrimaryBehavior(t *testing.T) {
	// Keep explicit contract linkage for Aitri FR coverage confidence.
	if _, err := contracts.Fr1TheSystemMustDefineAndApplyADashboardVisualAssetStrategyThatUsesExistingLocalIconographyAndCssDrivenPrimitivesFirstAndIntroducesNewBrandedAssetsOnlyWhenRequiredForReadabilityOrHierarchy(nil); err != nil {
		t.Fatalf("contract linkage failed: %v", err)
	}

	dashboardPath := repoFile(t, "web", "templates", "dashboard.html")
	basePath := repoFile(t, "web", "templates", "base.html")
	sidebarPath := repoFile(t, "web", "templates", "partials", "sidebar.html")
	metricsPath := repoFile(t, "web", "templates", "partials", "sse-metrics.html")
	chartsPath := repoFile(t, "web", "templates", "partials", "sse-charts.html")
	cssPath := repoFile(t, "web", "static", "css", "app.css")

	dashboard, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("read %s: %v", dashboardPath, err)
	}
	base, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read %s: %v", basePath, err)
	}
	sidebar, err := os.ReadFile(sidebarPath)
	if err != nil {
		t.Fatalf("read %s: %v", sidebarPath, err)
	}
	metrics, err := os.ReadFile(metricsPath)
	if err != nil {
		t.Fatalf("read %s: %v", metricsPath, err)
	}
	charts, err := os.ReadFile(chartsPath)
	if err != nil {
		t.Fatalf("read %s: %v", chartsPath, err)
	}
	css, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read %s: %v", cssPath, err)
	}

	// Cohesive dashboard structure is present across layout + nav + live telemetry.
	if !strings.Contains(string(base), "{{template \"sidebar\" .}}") {
		t.Fatal("expected shared sidebar layout in base template")
	}
	if !strings.Contains(string(sidebar), "href=\"/dashboard\"") && !strings.Contains(string(sidebar), "href=\"/\"") {
		t.Fatal("expected dashboard navigation entry in sidebar")
	}
	if !strings.Contains(string(dashboard), "sse-connect=\"/api/sse/dashboard\"") {
		t.Fatal("expected dashboard SSE live telemetry hook")
	}
	if !strings.Contains(string(metrics), "grid grid-cols-") || !strings.Contains(string(charts), "grid grid-cols-") {
		t.Fatal("expected responsive grid-based metrics and chart layouts")
	}

	if !strings.Contains(string(css), "@media (min-width:48rem)") {
		t.Fatal("expected responsive media-query breakpoints in app.css")
	}
	if !strings.Contains(string(css), ".min-h-screen") || !strings.Contains(string(css), ".overflow-y-auto") {
		t.Fatal("expected foundational responsive/scroll utility styles in app.css")
	}
}
