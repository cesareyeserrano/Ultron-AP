// AI explanation panel. Requests a probable-cause + remediation for the current
// system state and renders loading / success / error / empty states. Message text
// is set with textContent (never innerHTML) so model output cannot inject markup.
(function () {
  "use strict";

  var trigger = document.getElementById("ai-explain-trigger");
  var panel = document.getElementById("ai-explain-panel");
  if (!trigger || !panel) return;

  function clear() {
    while (panel.firstChild) panel.removeChild(panel.firstChild);
  }

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }

  function show() {
    panel.classList.remove("hidden");
  }

  function renderLoading() {
    clear();
    panel.dataset.state = "loading";
    panel.appendChild(el("p", "text-text-muted", "Analyzing telemetry…"));
    show();
  }

  function renderError(reason) {
    clear();
    panel.dataset.state = "error";
    panel.appendChild(el("p", "text-[color:var(--color-error-text)] font-medium", "Couldn't reach the AI provider"));
    panel.appendChild(el("p", "text-text-muted mt-1", reason || "Unknown error"));
    panel.appendChild(retryButton());
    show();
  }

  function renderEmpty() {
    clear();
    panel.dataset.state = "empty";
    panel.appendChild(el("p", "text-text-muted", "Not enough recent data to explain this yet. Wait for a few more samples and try again."));
    panel.appendChild(retryButton());
    show();
  }

  function renderSuccess(data) {
    clear();
    panel.dataset.state = "success";

    panel.appendChild(el("p", "text-[11px] uppercase tracking-wider text-text-muted", "Probable cause"));
    panel.appendChild(el("p", "text-text mb-3", data.cause || ""));

    if (data.remediation) {
      panel.appendChild(el("p", "text-[11px] uppercase tracking-wider text-text-muted", "Suggested remediation"));
      panel.appendChild(el("p", "text-text mb-3", data.remediation));
    }

    var signals = data.cited_signals || [];
    var label = data.unverified ? "Unverified (no cited evidence)" : "Cited signals";
    panel.appendChild(el("p", "text-[11px] uppercase tracking-wider text-text-muted", label));
    if (signals.length) {
      var row = el("div", "flex flex-wrap gap-1 mt-1");
      signals.forEach(function (s) {
        row.appendChild(el("span", "px-2 py-0.5 text-[11px] font-mono rounded bg-card border border-border text-text-muted", s));
      });
      panel.appendChild(row);
    }
    panel.appendChild(retryButton());
    show();
  }

  function retryButton() {
    var b = el("button", "mt-3 px-3 py-1.5 text-xs rounded-lg border border-border text-text-muted hover:bg-card min-h-[44px]", "⟳ Retry");
    b.type = "button";
    b.addEventListener("click", run);
    return b;
  }

  function run() {
    renderLoading();
    fetch("/api/ai/explain", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": trigger.dataset.csrf || "" },
      body: JSON.stringify({ scope: trigger.dataset.scope || "system" }),
    })
      .then(function (resp) {
        return resp.json().then(function (body) { return { status: resp.status, body: body }; });
      })
      .then(function (r) {
        if (r.status === 200) renderSuccess(r.body);
        else if (r.status === 422) renderEmpty();
        else renderError((r.body && r.body.error) || ("HTTP " + r.status));
      })
      .catch(function (e) {
        renderError(String(e));
      });
  }

  trigger.addEventListener("click", run);
})();
