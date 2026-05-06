package help

import "html/template"

// helpPageTemplate is the inline html/template body for /help. It is
// self-contained — the parent layout (sidebar, header, base) is rendered by
// the server's templates package; this template emits only the <main> body
// fragment under the existing dashboard chrome.
//
// Design notes:
//   - Two-voice layout uses Tailwind's responsive grid (md:grid-cols-2),
//     plain voice first in DOM order so mobile shows it first (FR-050).
//   - Each entry wrapper is <article id="entry-{slug}"> with the slug as the
//     stable URL fragment (FR-051, NFR-025).
//   - Filter is a single inline <script> using String.includes against a
//     pre-lowercased data-search attribute — ~25 lines, no framework
//     (FR-054, NFR-022).
//   - :target highlight is pure CSS, ~1.5 s fade (FR-051 AC-002, NFR-024).
//
// @aitri-trace FR-048 FR-050 FR-051 FR-054 NFR-024 NFR-025
const helpPageTemplate = `
{{define "help-page"}}
<style>
.help-entry:target {
  animation: help-target-flash 1.5s ease-out 0s 1 forwards;
  border-radius: 0.5rem;
}
@keyframes help-target-flash {
  0%   { background-color: rgba(0, 188, 212, 0.18); }
  100% { background-color: transparent; }
}
.help-entry.hidden, section.hidden { display: none; }
</style>
<div id="help-page" class="space-y-6">
  <header class="flex flex-col gap-2">
    <h1 class="text-2xl font-semibold text-text">Help &amp; glossary</h1>
    <p class="text-sm text-text-muted">Plain-language explanations of every metric, state, and verdict the dashboard surfaces.</p>
  </header>

  <div class="flex flex-col gap-1">
    <label for="help-filter" class="text-xs uppercase tracking-wider text-text-muted">Filter glossary</label>
    <input
      id="help-filter"
      type="search"
      placeholder="Type to filter (e.g. cpu, gateway, vpn)"
      aria-controls="help-entries"
      class="w-full max-w-md rounded-lg border border-border bg-card/40 px-3 py-2 text-sm text-text placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-accent/40">
  </div>

  <div id="help-entries" class="space-y-8">
    {{range .Categories}}
    <section id="{{.ID}}" data-category="{{trimCatPrefix .ID}}" class="space-y-3">
      <header><h2 class="text-lg font-semibold text-text">{{.Label}}</h2></header>
      <div class="space-y-3">
        {{range .Entries}}
        <article id="{{.ID}}" class="help-entry rounded-lg border border-border bg-card/40 p-4" data-search="{{.Search}}">
          <h3 class="text-base font-semibold text-text">{{.Title}}</h3>
          <div class="mt-3 grid gap-4 md:grid-cols-2">
            <div data-voice="plain">
              <span class="block text-xs uppercase tracking-wider text-text-muted">Plain</span>
              <p class="mt-1 text-sm text-text whitespace-pre-line">{{.Plain}}</p>
            </div>
            <div data-voice="technical">
              <span class="block text-xs uppercase tracking-wider text-text-muted">Technical</span>
              <p class="mt-1 text-sm text-text whitespace-pre-line">{{.Technical}}</p>
            </div>
          </div>
          {{if .Thresholds}}
          <table class="thresholds mt-3 w-full text-xs text-text">
            <thead><tr class="text-text-muted"><th class="text-left font-normal pr-3">Label</th><th class="text-left font-normal pr-3">Value</th><th class="text-left font-normal">Severity</th></tr></thead>
            <tbody>
            {{range .Thresholds}}
              <tr><td class="pr-3 py-0.5">{{.Label}}</td><td class="pr-3 py-0.5">{{.Value}}</td><td class="py-0.5">{{.Severity}}</td></tr>
            {{end}}
            </tbody>
          </table>
          {{end}}
          {{if .SourcePath}}
            <code class="source-path mt-3 inline-block text-[11px] text-text-muted">{{.SourcePath}}</code>
          {{end}}
        </article>
        {{end}}
      </div>
    </section>
    {{end}}
  </div>

  <script>
  (function () {
    var input = document.getElementById('help-filter');
    if (!input) return;
    var entries = document.querySelectorAll('article.help-entry');
    var sections = document.querySelectorAll('section[data-category]');
    input.addEventListener('input', function () {
      var q = (input.value || '').trim().toLowerCase();
      sections.forEach(function (sec) { sec.classList.remove('hidden'); });
      if (!q) {
        entries.forEach(function (e) { e.classList.remove('hidden'); });
        return;
      }
      sections.forEach(function (sec) {
        var any = false;
        sec.querySelectorAll('article.help-entry').forEach(function (e) {
          var hay = e.getAttribute('data-search') || '';
          var hit = hay.indexOf(q) !== -1;
          if (hit) { e.classList.remove('hidden'); any = true; }
          else { e.classList.add('hidden'); }
        });
        if (!any) sec.classList.add('hidden');
      });
    });
  })();
  </script>
</div>
{{end}}
`

// compileTemplate parses helpPageTemplate once. trimCatPrefix is exposed to
// the template so we don't store the bare category string in the view.
func (s *Service) compileTemplate() error {
	funcs := template.FuncMap{
		"trimCatPrefix": func(catID string) string {
			// "cat-system-metrics" → "system-metrics". The view holds the
			// prefixed id (used as element id); the data-category attribute
			// uses the bare value.
			const prefix = "cat-"
			if len(catID) > len(prefix) && catID[:len(prefix)] == prefix {
				return catID[len(prefix):]
			}
			return catID
		},
	}
	t, err := template.New("help").Funcs(funcs).Parse(helpPageTemplate)
	if err != nil {
		return err
	}
	s.tmpl = t
	return nil
}
