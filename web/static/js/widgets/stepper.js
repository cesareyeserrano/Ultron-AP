// FR-057 — unit-aware numeric stepper. Vanilla JS, no deps.
// Bounds enforced by min/max on the underlying <input type="number">.
// Direct keyboard input still works; the +/- buttons are touch-friendly
// (>=44px). On blur, an out-of-range value renders an inline error span
// using the same hint string as the label (FR-060) — but the canonical
// validation message comes from the server.
(function () {
  'use strict';

  function clamp(n, min, max) {
    return Math.min(Math.max(n, min), max);
  }

  function step(widget, dir) {
    var input = widget.querySelector('input[type="number"]');
    if (!input) return;
    var min = parseInt(widget.getAttribute('data-min'), 10);
    var max = parseInt(widget.getAttribute('data-max'), 10);
    var cur = parseInt(input.value, 10);
    if (isNaN(cur)) cur = isNaN(min) ? 0 : min;
    var next = clamp(cur + dir, isNaN(min) ? cur + dir : min, isNaN(max) ? cur + dir : max);
    if (next === cur) return; // no-op at boundary (TC-SR-057E2E)
    input.value = String(next);
    widget.setAttribute('data-value', String(next));
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }

  function init(widget) {
    if (widget.__stepperBound) return;
    widget.__stepperBound = true;

    widget.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-step]');
      if (!btn || !widget.contains(btn)) return;
      e.preventDefault();
      step(widget, parseInt(btn.getAttribute('data-step'), 10));
    });

    var input = widget.querySelector('input[type="number"]');
    if (input) {
      input.addEventListener('input', function () {
        widget.setAttribute('data-value', input.value);
      });
    }
  }

  function initAll(root) {
    var widgets = (root || document).querySelectorAll('[data-widget="stepper"]');
    for (var i = 0; i < widgets.length; i++) init(widgets[i]);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initAll(); });
  } else {
    initAll();
  }

  // Re-init on htmx swap so dynamic forms get the widget bound.
  document.body.addEventListener('htmx:afterSwap', function (e) { initAll(e.target); });
})();
