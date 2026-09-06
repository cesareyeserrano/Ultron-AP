// Module:       internal/isolation
// Purpose:      Assert the privilege boundary the C2 hardening established:
//
//	the unprivileged web app binary must not be able to reach the
//	Docker daemon socket, directly or transitively.
//
// This lives in its own package on purpose. It asserts a property of the whole
// module rather than of any one package, and putting it under internal/docker
// or internal/server would tie a module-wide invariant to a package that might
// later be split or renamed.
//
// @aitri-trace FR-093, US-093, NFR-092
package isolation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	webAppPkg    = "../../cmd/ultron-ap"
	helperPkg    = "../../cmd/ultron-helper"
	dockerAPIPkg = "github.com/cesareyeserrano/ultron-ap/internal/dockerapi"
)

// deps returns the full transitive dependency closure of a package, as the Go
// toolchain itself computes it.
//
// This is the assertion that matters. A grep for a socket path only sees the
// literal string: it misses an indirect import, a path assembled from
// constants, and a dependency pulled in three packages deep. The dependency
// graph is the actual property — "can this binary reach that code at all".
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pkg)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "go list -deps %s failed: %s", pkg, out)

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.Greaterf(t, len(lines), 10, "go list returned suspiciously few packages for %s", pkg)
	return lines
}

// @aitri-tc TC-DVH-050h — the web app binary depends on neither the Docker SDK
// nor the helper-only transport package (AC-093-002).
func TestTC_DVH_050h(t *testing.T) {
	for _, pkg := range deps(t, webAppPkg) {
		assert.Falsef(t, strings.HasPrefix(pkg, "github.com/docker/"),
			"cmd/ultron-ap must not depend on the Docker SDK, but reaches %s", pkg)
		assert.NotEqualf(t, dockerAPIPkg, pkg,
			"cmd/ultron-ap must not depend on the helper-only Docker transport (%s)", pkg)
	}
}

// @aitri-tc TC-DVH-051e — positive control: the HELPER does depend on the
// transport package.
//
// Without this, TestTC_DVH_050h could be passing vacuously — because go list
// silently returned nothing useful, because the package was renamed, because
// the constant no longer matches. An absence assertion is only worth something
// when paired with proof that it can detect presence (AC-093-002).
func TestTC_DVH_051e(t *testing.T) {
	assert.Contains(t, deps(t, helperPkg), dockerAPIPkg,
		"the helper MUST depend on internal/dockerapi — if it does not, TestTC_DVH_050h proves nothing")
}

// @aitri-tc TC-DVH-052f — the web app's source tree does not name the Docker
// daemon socket anywhere, comments included (AC-093-001).
func TestTC_DVH_052f(t *testing.T) {
	// Assembled rather than written out: this file is part of the tree being
	// scanned, and spelling the path here would make the test fail on itself.
	needle := "docker" + ".sock"

	for _, dir := range []string{
		"../../internal/server",
		"../../internal/docker",
		"../../internal/metrics",
		"../../cmd/ultron-ap",
	} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			assert.NotContainsf(t, string(b), needle,
				"%s names the Docker daemon socket; only the helper may", path)
			return nil
		})
		require.NoErrorf(t, err, "walking %s", dir)
	}
}
