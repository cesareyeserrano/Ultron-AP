// FR-061 — toggle switch wrapping a hidden checkbox. Vanilla JS.
// Click on the label, the track, or pressing Space when focused toggles state.
// The underlying <input type="checkbox" name="enabled"> remains the form
// submit value (preserves backwards-compat: enabled=on / missing).
(function () {
  'use strict';

  function setState(widget, on) {
    widget.setAttribute('data-state', on ? 'on' : 'off');
    var parts = widget.querySelectorAll('[data-state]');
    for (var i = 0; i < parts.length; i++) {
      parts[i].setAttribute('data-state', on ? 'on' : 'off');
    }
    var input = widget.querySelector('input[data-toggle-input]');
    if (input) {
      input.checked = on;
      input.dispatchEvent(new Event('change', { bubbles: true }));
    }
    var lbl = widget.querySelector('[data-toggle-label]');
    if (lbl) lbl.textContent = on ? 'Enabled' : 'Disabled';
  }

  function init(widget) {
    if (widget.__toggleBound) return;
    widget.__toggleBound = true;

    widget.addEventListener('click', function (e) {
      // Don't double-fire on the hidden input itself.
      if (e.target.closest('input[data-toggle-input]')) return;
      e.preventDefault();
      var on = widget.getAttribute('data-state') === 'on';
      setState(widget, !on);
    });

    widget.addEventListener('keydown', function (e) {
      if (e.key === ' ' || e.key === 'Enter') {
        e.preventDefault();
        var on = widget.getAttribute('data-state') === 'on';
        setState(widget, !on);
      }
    });

    if (!widget.hasAttribute('tabindex')) widget.tabIndex = 0;
  }

  function initAll(root) {
    var widgets = (root || document).querySelectorAll('[data-widget="toggle"]');
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
