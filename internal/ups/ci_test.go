package ups

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TC-UPS-050h (NFR-021): the CI workflow runs the Go test suite (covering
// ./internal/ups/...) on push to the main branch.
func TestTC_UPS_050h_CIRunsTests(t *testing.T) {
	// @aitri-tc TC-UPS-050h
	// The workflow lives at the repo root; this test runs from internal/ups.
	path := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CI workflow %s: %v", path, err)
	}
	content := string(data)

	if !strings.Contains(content, "go test") {
		t.Error("CI workflow does not run 'go test'")
	}
	// Must cover the whole module (./...) so internal/ups is included.
	if !strings.Contains(content, "./...") && !strings.Contains(content, "./internal/ups/") {
		t.Error("CI workflow does not run tests over ./... (would miss internal/ups)")
	}
	// Must trigger on the main branch.
	if !strings.Contains(content, "push") || !strings.Contains(content, "main") {
		t.Error("CI workflow does not trigger on push to main")
	}
}
