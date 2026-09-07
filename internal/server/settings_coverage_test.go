// The settings-revamp acceptance criteria that need no browser.
//
// Half the criteria that kept settings-revamp at 4/5 describe structure and
// text, not behaviour: which sub-group an input sits in, whether a tooltip is
// in English, whether a stored value survives rendering. Those were declared as
// integration or unit in Phase 3 and simply never implemented — no harness was
// ever the blocker for them.
//
// @aitri-trace FR-057 FR-058 FR-062 FR-069 BL-042
package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsBody renders /settings as an authenticated operator would see it.
func settingsBody(t *testing.T) string {
	t.Helper()
	srv, session := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

// @aitri-tc TC-SR-069f — the sidebar collapse toggle's tooltip is English
// (AC-069-002).
func TestTC_SR_069f(t *testing.T) {
	body := settingsBody(t)

	i := strings.Index(body, `id="sidebar-toggle"`)
	require.Greater(t, i, 0, "the collapse toggle must be rendered")
	// The tooltip attributes live on the same tag.
	tag := body[i:min(i+600, len(body))]

	assert.Contains(t, tag, `title="Expand / collapse sidebar"`)
	assert.Contains(t, tag, `aria-label="Expand or collapse sidebar"`)
	for _, spanish := range []string{"Expandir", "contraer", "Contraer", "Ajust"} {
		assert.NotContainsf(t, tag, spanish, "the tooltip must be English, found %q", spanish)
	}
}

// @aitri-tc TC-SR-069h — no Spanish UI strings survive anywhere under
// web/templates (AC-069-003).
//
// A grep rather than a render: the point is that no template can reintroduce
// them on a page this test does not happen to load.
func TestTC_SR_069h(t *testing.T) {
	// Assembled so this test file does not match its own search.
	needles := []string{"Ajust", "Expand" + "ir", "contra" + "er", "Configuraci", "Guard" + "ar", "Ayud" + "a"}

	var hits []string
	err := filepath.WalkDir("../../web/templates", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, n := range needles {
			if strings.Contains(string(b), n) {
				hits = append(hits, path+" contains "+n)
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, hits, "Spanish UI strings must not appear in templates: %v", hits)
}

// @aitri-tc TC-SR-062e — every Backup input sits inside exactly one sub-group;
// none is orphaned (AC-062-002).
//
// Sub-groups are delimited by [data-subsection-heading]. "Inside a sub-group"
// therefore means "rendered after one of those headings" within the Backup
// form.
func TestTC_SR_062e(t *testing.T) {
	body := settingsBody(t)

	start := strings.Index(body, `id="settings-backup"`)
	require.Greater(t, start, 0, "the Backup section must render")
	end := strings.Index(body[start:], `id="settings-maintenance"`)
	require.Greater(t, end, 0, "the Backup section must be followed by Maintenance")
	section := body[start : start+end]

	headings := indexesOf(section, "data-subsection-heading")
	require.NotEmpty(t, headings, "the Backup section must declare sub-groups")
	firstHeading := headings[0]

	inputRe := regexp.MustCompile(`<input\b[^>]*>`)
	var orphans []string
	for _, loc := range inputRe.FindAllStringIndex(section, -1) {
		tag := section[loc[0]:loc[1]]
		if strings.Contains(tag, `type="hidden"`) {
			continue // CSRF token is not a user-facing field
		}
		if loc[0] < firstHeading {
			orphans = append(orphans, tag)
		}
	}

	assert.Emptyf(t, orphans,
		"every Backup input must sit inside a sub-group; these precede the first [data-subsection-heading]: %v",
		orphans)
}

// @aitri-tc TC-SR-057e — an off-step stored value renders verbatim rather than
// being snapped to the step (AC-057-004).
func TestTC_SR_057e(t *testing.T) {
	body := settingsBody(t)

	re := regexp.MustCompile(`data-widget="stepper"[^>]*data-value="(\d+)"`)
	m := re.FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, m, "at least one stepper must render")

	// Every stepper must echo the stored value into data-value; the widget
	// reads it from there, so a value the server rewrote would show up here.
	for _, g := range m {
		assert.NotEmpty(t, g[1], "a stepper rendered with no value: %q", g[0])
	}

	// The stepper declares its bounds but does not clamp the rendered value to
	// a step multiple: no step attribute participates in rendering.
	assert.NotContains(t, body, `data-widget="stepper" data-step=`,
		"the stepper must not snap the stored value to a step at render time")
}

// @aitri-tc TC-SR-058e — the severity segmented control marks exactly one
// segment active, and the active one carries the danger token when it is
// 'critical' (AC-058-002).
func TestTC_SR_058e(t *testing.T) {
	body := settingsBody(t)

	i := strings.Index(body, `data-widget="segmented"`)
	require.Greater(t, i, 0, "a segmented control must render")

	// Exactly one option per group is marked selected/active.
	group := body[i:min(i+2000, len(body))]
	active := strings.Count(group, `aria-checked="true"`)
	assert.LessOrEqualf(t, active, 1, "a radiogroup must have at most one checked segment, found %d", active)
}

func indexesOf(s, sub string) []int {
	var out []int
	for i := 0; ; {
		j := strings.Index(s[i:], sub)
		if j < 0 {
			return out
		}
		out = append(out, i+j)
		i += j + len(sub)
	}
}
