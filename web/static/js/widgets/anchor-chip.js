// The anchor-chip strip wires clicks to scroll AND expand the target
// accordion. Hash on page load auto-expands the matching accordion before
// first paint. Hash updates use replaceState — no extra Back-button entries.
(function () {
  'use strict';

  function expandAccordion(section) {
    if (!section) return;
    var body = section.querySelector('[data-accordion-body]');
    var toggle = section.querySelector('[data-accordion-toggle]');
    var chevron = section.querySelector('[data-accordion-chevron]');
    if (!body || !toggle) return;
    body.classList.remove('hidden');
    toggle.setAttribute('aria-expanded', 'true');
    if (chevron) chevron.style.transform = 'rotate(180deg)';
  }

  function collapseOthers(targetId) {
    var sections = document.querySelectorAll('[data-settings-section]');
    for (var i = 0; i < sections.length; i++) {
      if (sections[i].id === targetId) continue;
      var body = sections[i].querySelector('[data-accordion-body]');
      var toggle = sections[i].querySelector('[data-accordion-toggle]');
      var chevron = sections[i].querySelector('[data-accordion-chevron]');
      if (body) body.classList.add('hidden');
      if (toggle) toggle.setAttribute('aria-expanded', 'false');
      if (chevron) chevron.style.transform = 'rotate(0deg)';
    }
  }

  function bind() {
    var chips = document.querySelectorAll('[data-anchor]');
    for (var i = 0; i < chips.length; i++) {
      if (chips[i].__anchorChipBound) continue;
      chips[i].__anchorChipBound = true;
      chips[i].addEventListener('click', function (e) {
        var anchor = this.getAttribute('data-anchor');
        if (!anchor) return;
        var sectionId = 'settings-' + anchor;
        var section = document.getElementById(sectionId);
        if (!section) return;
        e.preventDefault();
        // Update hash without pushing a new history entry.
        if (window.history && window.history.replaceState) {
          window.history.replaceState(null, '', '#' + sectionId);
        } else {
          window.location.hash = sectionId;
        }
        expandAccordion(section);
        // Scroll into view; smooth on supported browsers.
        try {
          section.scrollIntoView({ behavior: 'smooth', block: 'start' });
        } catch (_) {
          section.scrollIntoView();
        }
      });
    }
  }

  function expandFromHash() {
    var hash = window.location.hash;
    if (!hash) return;
    var id = hash.replace(/^#/, '');
    var section = document.getElementById(id);
    if (section && section.hasAttribute('data-settings-section')) {
      expandAccordion(section);
      collapseOthers(id);
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { bind(); expandFromHash(); });
  } else {
    bind();
    expandFromHash();
  }

  // Re-bind on hx-boost swaps and history restores — the swapped-in chips are
  // fresh nodes (the per-chip guard keeps re-binding idempotent).
  document.body.addEventListener('htmx:afterSwap', function () { bind(); });
  document.body.addEventListener('htmx:historyRestore', function () { bind(); });
})();
