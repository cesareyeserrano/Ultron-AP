// FR-068 — encryption-key composite picker. On blur of the value input, hits
// the auth-gated /api/settings/encryption-key/probe endpoint and renders the
// ✓/✗ + reason badge. Probe response NEVER includes key bytes/length/hash.
//
// 250 ms debounce after the last keystroke; 3 s timeout per request.
(function () {
  'use strict';

  function setBadge(badge, ok, reason, state) {
    if (!badge) return;
    badge.setAttribute('data-state', state);
    badge.className = 'text-[11px] px-2 py-1 rounded border self-start sm:self-center';
    if (state === 'idle') {
      badge.className += ' border-border/50 text-text-muted';
      badge.textContent = 'idle';
      return;
    }
    if (state === 'probing') {
      badge.className += ' border-border/50 text-text-muted';
      badge.textContent = '…';
      return;
    }
    if (state === 'error') {
      badge.className += ' border-danger/40 text-danger';
      badge.textContent = '✗ probe failed';
      return;
    }
    if (ok) {
      badge.className += ' border-green-400/40 text-green-400';
      badge.textContent = '✓ ' + reason;
    } else {
      badge.className += ' border-danger/40 text-danger';
      badge.textContent = '✗ ' + reason;
    }
  }

  function probe(widget) {
    var schemeSel = widget.querySelector('[data-enc-scheme]');
    var valueInput = widget.querySelector('input[name="encryption_key_ref"]');
    var badge = widget.querySelector('[data-enc-badge]');
    if (!schemeSel || !valueInput || !badge) return;

    var raw = (valueInput.value || '').trim();
    var scheme = schemeSel.value;
    var actualValue = raw;
    // If the value already includes "scheme:..." (legacy stored format),
    // strip the scheme prefix before sending.
    if (scheme === 'env' && raw.indexOf('env:') === 0) actualValue = raw.slice(4);
    if (scheme === 'file' && raw.indexOf('file:') === 0) actualValue = raw.slice(5);
    if (scheme === 'kms' && raw.indexOf('kms://') === 0) actualValue = raw.slice(6);

    if (!actualValue) {
      setBadge(badge, false, 'value required', 'result');
      return;
    }
    setBadge(badge, false, '', 'probing');

    var ctrl = (typeof AbortController !== 'undefined') ? new AbortController() : null;
    var timeout = setTimeout(function () { if (ctrl) ctrl.abort(); }, 3000);

    var url = '/api/settings/encryption-key/probe?scheme=' +
              encodeURIComponent(scheme) +
              '&value=' + encodeURIComponent(actualValue);

    fetch(url, { credentials: 'same-origin', signal: ctrl ? ctrl.signal : undefined })
      .then(function (r) { return r.json().then(function (j) { return { status: r.status, body: j }; }); })
      .then(function (res) {
        clearTimeout(timeout);
        var ok = !!(res.body && res.body.ok);
        var reason = (res.body && res.body.reason) || '';
        setBadge(badge, ok, reason, 'result');
      })
      .catch(function () {
        clearTimeout(timeout);
        setBadge(badge, false, '', 'error');
      });
  }

  function init(widget) {
    if (widget.__encKeyBound) return;
    widget.__encKeyBound = true;
    var valueInput = widget.querySelector('input[name="encryption_key_ref"]');
    if (!valueInput) return;

    var debounceTimer = null;
    valueInput.addEventListener('blur', function () { probe(widget); });
    valueInput.addEventListener('input', function () {
      if (debounceTimer) clearTimeout(debounceTimer);
      debounceTimer = setTimeout(function () { probe(widget); }, 250);
    });
    var schemeSel = widget.querySelector('[data-enc-scheme]');
    if (schemeSel) schemeSel.addEventListener('change', function () { probe(widget); });
  }

  function initAll(root) {
    var widgets = (root || document).querySelectorAll('[data-widget="encryption-key"]');
    for (var i = 0; i < widgets.length; i++) init(widgets[i]);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initAll(); });
  } else {
    initAll();
  }
})();
