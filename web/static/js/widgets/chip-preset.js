// FR-059 — chip-preset row + custom escape hatch. Click a preset chip to
// set the field's value and highlight the chip. Typing in the custom field
// clears all preset highlights — the custom field is the source of truth.
(function () {
  'use strict';

  function findCustom(widget) {
    return widget.querySelector('input[type="number"]');
  }

  function setActivePreset(widget, value) {
    var presets = widget.querySelectorAll('[data-preset-value]');
    var matched = false;
    for (var i = 0; i < presets.length; i++) {
      var on = presets[i].getAttribute('data-preset-value') === value;
      presets[i].setAttribute('data-active', on ? 'true' : 'false');
      if (on) matched = true;
    }
    return matched;
  }

  function init(widget) {
    if (widget.__chipPresetBound) return;
    widget.__chipPresetBound = true;

    widget.addEventListener('click', function (e) {
      var chip = e.target.closest('[data-preset-value]');
      if (!chip || !widget.contains(chip)) return;
      e.preventDefault();
      var v = chip.getAttribute('data-preset-value');
      var custom = findCustom(widget);
      if (custom) {
        custom.value = v;
        custom.dispatchEvent(new Event('input', { bubbles: true }));
        custom.dispatchEvent(new Event('change', { bubbles: true }));
      }
      setActivePreset(widget, v);
    });

    var custom = findCustom(widget);
    if (custom) {
      custom.addEventListener('input', function () {
        // If custom matches a preset, highlight it; else clear all.
        setActivePreset(widget, custom.value);
      });
    }
  }

  function initAll(root) {
    var widgets = (root || document).querySelectorAll('[data-widget="chip-preset"]');
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
