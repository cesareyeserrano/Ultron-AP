// Per-service log drawer. The fetch is htmx's job (hx-get with
// hx-trigger="click once", so re-opening a drawer never re-hits journalctl);
// this widget owns only the open/close state and aria-expanded.
//
// Same lifecycle as the other widgets: loaded from <head> on every
// page, re-init on htmx:afterSwap and htmx:historyRestore, idempotent guards.
(function () {
  'use strict';

  function init(btn) {
    if (btn.__logsToggleBound) return;
    btn.__logsToggleBound = true;

    btn.addEventListener('click', function () {
      var id = btn.getAttribute('data-logs-target');
      var drawer = id ? document.getElementById(id) : null;
      if (!drawer) return;

      var open = !drawer.classList.contains('hidden');
      drawer.classList.toggle('hidden', open);
      btn.setAttribute('aria-expanded', open ? 'false' : 'true');
    });
  }

  function initAll(root) {
    var buttons = (root || document).querySelectorAll('[data-logs-toggle]');
    for (var i = 0; i < buttons.length; i++) init(buttons[i]);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initAll(); });
  } else {
    initAll();
  }

  document.body.addEventListener('htmx:afterSwap', function (e) { initAll(e.target); });
  document.body.addEventListener('htmx:historyRestore', function () { initAll(); });
})();
