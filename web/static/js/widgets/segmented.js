// FR-058 — 3-button segmented severity control. Vanilla JS.
// One-click selection, keyboard nav (←/→ to cycle, Space/Enter to select).
// The control submits its value through a hidden <input> sibling.
(function () {
  'use strict';

  function setActive(widget, valueAttr) {
    var radios = widget.querySelectorAll('[role="radio"]');
    var hidden = widget.querySelector('input[type="hidden"]');
    for (var i = 0; i < radios.length; i++) {
      var r = radios[i];
      var on = r.getAttribute('data-value') === valueAttr;
      r.setAttribute('data-active', on ? 'true' : 'false');
      r.setAttribute('aria-checked', on ? 'true' : 'false');
      r.tabIndex = on ? 0 : -1;
    }
    if (hidden) {
      hidden.value = valueAttr;
      hidden.dispatchEvent(new Event('change', { bubbles: true }));
    }
  }

  function init(widget) {
    if (widget.__segmentedBound) return;
    widget.__segmentedBound = true;

    widget.addEventListener('click', function (e) {
      var seg = e.target.closest('[role="radio"]');
      if (!seg || !widget.contains(seg)) return;
      e.preventDefault();
      setActive(widget, seg.getAttribute('data-value'));
    });

    widget.addEventListener('keydown', function (e) {
      var seg = e.target.closest('[role="radio"]');
      if (!seg || !widget.contains(seg)) return;
      var radios = Array.prototype.slice.call(widget.querySelectorAll('[role="radio"]'));
      var idx = radios.indexOf(seg);
      if (idx < 0) return;
      if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
        e.preventDefault();
        var prev = radios[(idx - 1 + radios.length) % radios.length];
        setActive(widget, prev.getAttribute('data-value'));
        prev.focus();
      } else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
        e.preventDefault();
        var next = radios[(idx + 1) % radios.length];
        setActive(widget, next.getAttribute('data-value'));
        next.focus();
      } else if (e.key === ' ' || e.key === 'Enter') {
        e.preventDefault();
        setActive(widget, seg.getAttribute('data-value'));
      }
    });
  }

  function initAll(root) {
    var widgets = (root || document).querySelectorAll('[data-widget="segmented"]');
    for (var i = 0; i < widgets.length; i++) init(widgets[i]);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initAll(); });
  } else {
    initAll();
  }
  document.body.addEventListener('htmx:afterSwap', function (e) { initAll(e.target); });
  // History restores re-insert cached DOM without firing afterSwap (BG-038 family).
  document.body.addEventListener('htmx:historyRestore', function () { initAll(); });
})();
