// CSS7 — settings page controller: accordion sections, per-form dirty/save
// state (FR-065 pill), inline field errors, retry, and the destructive
// System Controls confirm/countdown guard. Extracted from settings.html's
// inline <script>.
//
// Loaded from <head> on every page (BG-038 pattern: hx-boost body swaps never
// re-run head scripts), so binding happens via DOMContentLoaded plus
// htmx:afterSwap/historyRestore. Per-node __*Bound guards make re-init
// idempotent; the htmx result handlers are delegated on document.body and
// registered exactly once, so they survive body swaps without stacking.
(function () {
  'use strict';

  if (window.__settingsBound) return;
  window.__settingsBound = true;

  function settingsShell() {
    return document.getElementById('settings-shell');
  }

  function captureFormState(form) {
    var data = new FormData(form);
    var pairs = [];
    data.forEach(function (value, key) { pairs.push(key + '=' + String(value)); });
    pairs.sort();
    return pairs.join('&');
  }

  function setFormBusy(form, busy) {
    form.querySelectorAll('button, input, select, textarea').forEach(function (el) {
      if (el.type === 'hidden') return;
      el.disabled = busy;
    });
  }

  function setFormState(form, state, message, allowRetry) {
    var shell = settingsShell();
    var name = form.getAttribute('data-settings-form');
    // FR-065: form-state pill is rendered only when state != idle.
    // At idle the pill element is removed from the DOM entirely.
    var host = (name && shell) ? shell.querySelector('[data-form-state-host="' + name + '"]') : null;
    var statusTargetSel = form.getAttribute('data-status-target');
    var statusTarget = statusTargetSel ? document.querySelector(statusTargetSel) : null;

    if (host) {
      // Clear existing pill
      host.innerHTML = '';
      if (state && state !== 'idle') {
        var pill = document.createElement('span');
        pill.setAttribute('data-form-state-pill', name || '');
        pill.setAttribute('data-state', state);
        pill.className = 'text-[10px] px-2 py-0.5 rounded border';
        if (state === 'saving') pill.className += ' border-yellow-400/50 text-yellow-400';
        else if (state === 'applied') pill.className += ' border-green-400/40 text-green-400';
        else if (state === 'failed') pill.className += ' border-danger/40 text-danger';
        else pill.className += ' border-border/50 text-text-muted';
        pill.textContent = state;
        host.appendChild(pill);

        // Auto-remove `applied` after 2.5s — pill fully removed (NOT hidden).
        if (state === 'applied') {
          setTimeout(function () {
            if (host.querySelector('[data-state="applied"]')) host.innerHTML = '';
          }, 2500);
        }
      }
    }

    if (statusTarget && message !== undefined) {
      var levelClass = state === 'failed' ? 'text-danger' : (state === 'applied' ? 'text-green-400' : (state === 'saving' ? 'text-yellow-400' : 'text-text-muted'));
      var retry = allowRetry ? '<button type="button" data-retry-form="' + (name || '') + '" class="ml-2 px-2 py-0.5 rounded border border-border/50 text-text-muted hover:text-text hover:bg-card transition-colors">Retry</button>' : '';
      statusTarget.innerHTML = message ? ('<div class="text-xs py-2 ' + levelClass + '">' + message + retry + '</div>') : '';
    }
  }

  function clearFieldErrors(form) {
    form.querySelectorAll('[data-field-error]').forEach(function (el) { el.remove(); });
    form.querySelectorAll('input,select,textarea').forEach(function (el) { el.classList.remove('border-danger'); });
  }

  function setFieldError(form, fieldName, message) {
    var input = form.querySelector('[name="' + fieldName + '"]');
    if (!input) return false;
    input.classList.add('border-danger');
    var p = document.createElement('p');
    p.setAttribute('data-field-error', 'true');
    p.className = 'text-xs text-danger mt-1';
    p.textContent = message;
    if (input.parentElement) input.parentElement.appendChild(p);
    return true;
  }

  function applyFieldErrorFromResponse(form, text) {
    var lowered = (text || '').toLowerCase();
    if (!lowered) return false;
    if (lowered.indexOf('threshold') >= 0) return setFieldError(form, 'threshold', 'Threshold is invalid.');
    if (lowered.indexOf('cooldown') >= 0) return setFieldError(form, 'cooldown', 'Cooldown is invalid.');
    if (lowered.indexOf('metric') >= 0) return setFieldError(form, 'metric', 'Metric value is invalid.');
    if (lowered.indexOf('operator') >= 0) return setFieldError(form, 'operator', 'Operator value is invalid.');
    if (lowered.indexOf('severity') >= 0) return setFieldError(form, 'severity', 'Severity value is invalid.');
    if (lowered.indexOf('sse') >= 0) return setFieldError(form, 'sse_interval_sec', 'Allowed range: 2-60 seconds.');
    if (lowered.indexOf('disk') >= 0) return setFieldError(form, 'disk_interval_min', 'Allowed range: 1-1440 minutes.');
    if (lowered.indexOf('docker') >= 0) return setFieldError(form, 'docker_interval_sec', 'Allowed range: 5-300 seconds.');
    if (lowered.indexOf('systemd') >= 0) return setFieldError(form, 'systemd_interval_sec', 'Allowed range: 5-300 seconds.');
    return false;
  }

  function syncDirty(form) {
    var saveBtn = form.querySelector('[data-primary-save]');
    var resetBtn = form.querySelector('[data-form-reset]');
    var dirty = captureFormState(form) !== form.getAttribute('data-form-baseline');
    if (saveBtn) {
      saveBtn.classList.toggle('ring', dirty);
      saveBtn.classList.toggle('ring-accent', dirty);
    }
    if (resetBtn) resetBtn.classList.toggle('hidden', !dirty);
    if (dirty) setFormState(form, 'idle', 'Unsaved changes.', false);
  }

  function bindSettingsForm(form) {
    if (form.__settingsFormBound) return;
    form.__settingsFormBound = true;

    form.setAttribute('data-form-baseline', captureFormState(form));
    setFormState(form, 'idle', '', false);

    var resetBtn = form.querySelector('[data-form-reset]');

    form.addEventListener('input', function () { syncDirty(form); });
    form.addEventListener('change', function () { syncDirty(form); });

    if (resetBtn) {
      resetBtn.addEventListener('click', function () {
        form.reset();
        setTimeout(function () {
          clearFieldErrors(form);
          form.setAttribute('data-form-baseline', captureFormState(form));
          syncDirty(form);
          setFormState(form, 'idle', 'Changes reverted locally.', false);
        }, 0);
      });
    }

    form.addEventListener('submit', function () {
      clearFieldErrors(form);
      setFormState(form, 'saving', form.getAttribute('data-saving-label') || 'Applying settings...', false);
    });
  }

  // htmx result handlers — delegated so one registration covers every
  // settings form across every hx-boost body swap.
  document.body.addEventListener('htmx:afterRequest', function (e) {
    var trigger = e.detail && e.detail.elt;
    if (!trigger || !trigger.closest) return;
    var form = trigger.closest('form[data-settings-form]');
    if (!form) return;
    if (trigger.hasAttribute && trigger.hasAttribute('data-settings-ignore-state')) return;
    setFormBusy(form, false);
    if (e.detail.successful) {
      form.setAttribute('data-form-baseline', captureFormState(form));
      syncDirty(form);
      setFormState(form, 'applied', 'Settings applied successfully.', false);
    }
  });

  document.body.addEventListener('htmx:responseError', function (e) {
    var trigger = e.detail && e.detail.elt;
    if (!trigger || !trigger.closest) return;
    var form = trigger.closest('form[data-settings-form]');
    if (!form) return;
    if (trigger.hasAttribute && trigger.hasAttribute('data-settings-ignore-state')) return;
    setFormBusy(form, false);
    var xhr = e.detail && e.detail.xhr;
    var text = xhr && xhr.responseText ? xhr.responseText : 'Request failed.';
    var mapped = applyFieldErrorFromResponse(form, text);
    setFormState(form, 'failed', mapped ? 'Validation failed. Fix highlighted fields.' : 'Save failed. You can retry safely.', true);
  });

  function setAccordionOpen(section, open) {
    var body = section.querySelector('[data-accordion-body]');
    var toggle = section.querySelector('[data-accordion-toggle]');
    var chevron = section.querySelector('[data-accordion-chevron]');
    if (!body || !toggle) return;
    body.classList.toggle('hidden', !open);
    toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (chevron) chevron.style.transform = open ? 'rotate(180deg)' : 'rotate(0deg)';
  }

  function initAccordion(shell) {
    var sections = Array.from(shell.querySelectorAll('[data-settings-section]'));

    sections.forEach(function (section, index) {
      var toggle = section.querySelector('[data-accordion-toggle]');
      if (!toggle) {
        section.classList.remove('p-4', 'md:p-5', 'space-y-3');
        section.classList.add('overflow-hidden');

        var children = Array.from(section.children);
        var heading = children.length ? children[0] : null;
        if (!heading) return;

        toggle = document.createElement('button');
        toggle.type = 'button';
        toggle.setAttribute('data-accordion-toggle', 'true');
        toggle.className = 'w-full px-4 py-3 md:px-5 md:py-4 bg-transparent hover:bg-card/25 transition-colors text-left flex items-center justify-between gap-3';

        var headingWrap = document.createElement('div');
        headingWrap.className = 'flex-1 min-w-0';
        headingWrap.appendChild(heading);

        var chevron = document.createElement('span');
        chevron.setAttribute('data-accordion-chevron', 'true');
        chevron.className = 'text-text-muted text-xs transition-all duration-200';
        chevron.textContent = '▼';

        toggle.appendChild(headingWrap);
        toggle.appendChild(chevron);

        var body = document.createElement('div');
        body.setAttribute('data-accordion-body', 'true');
        body.className = 'px-4 pb-4 md:px-5 md:pb-5 pt-1 space-y-3 border-t border-border/30';
        children.slice(1).forEach(function (node) { body.appendChild(node); });

        section.appendChild(toggle);
        section.appendChild(body);

        setAccordionOpen(section, index === 0);
      }

      // A history-restored DOM arrives already built but with dead listeners,
      // so binding is guarded separately from building.
      if (toggle.__accordionBound) return;
      toggle.__accordionBound = true;
      toggle.addEventListener('click', function () {
        var currentlyOpen = toggle.getAttribute('aria-expanded') === 'true';
        sections.forEach(function (other) { if (other !== section) setAccordionOpen(other, false); });
        setAccordionOpen(section, !currentlyOpen);
      });
    });
  }

  function initDangerGuard(shell) {
    var guardEl = document.getElementById('danger-action-guard');
    var guardForm = document.getElementById('danger-action-form');
    var guardCopy = document.getElementById('danger-action-copy');
    var guardHint = document.getElementById('danger-confirm-hint');
    var wordInput = document.getElementById('danger-confirm-word');
    var submitBtn = document.getElementById('danger-action-submit');
    var countdownBar = document.getElementById('danger-countdown-bar');
    var countdownCopy = document.getElementById('danger-countdown-copy');
    if (!guardEl || !guardForm || !wordInput) return;
    if (guardForm.__dangerBound) return;
    guardForm.__dangerBound = true;

    var expectedWord = '';
    var countdownTimer = null;
    var countdownSec = 3;
    var remaining = 0;

    function resetDangerGuard() {
      if (countdownTimer) clearInterval(countdownTimer);
      countdownTimer = null;
      remaining = 0;
      guardEl.classList.add('hidden');
      guardForm.querySelector('[name="confirm_action"]').value = '';
      guardForm.querySelector('[name="countdown_ack"]').value = '0';
      guardForm.setAttribute('hx-post', '/api/system/restart');
      wordInput.value = '';
      if (submitBtn) submitBtn.disabled = true;
      if (countdownBar) countdownBar.style.width = '0%';
      if (countdownCopy) countdownCopy.textContent = 'Countdown not started.';
      if (guardHint) guardHint.textContent = 'Type the word exactly.';
    }

    function setDangerCountdownState() {
      if (!countdownBar || !countdownCopy || !submitBtn) return;
      var done = remaining <= 0;
      var pct = done ? 100 : (((countdownSec - remaining) / countdownSec) * 100);
      countdownBar.style.width = Math.max(0, Math.min(100, pct)) + '%';
      if (done) {
        countdownCopy.textContent = 'Countdown complete. You may execute now.';
        guardForm.querySelector('[name="countdown_ack"]').value = '1';
        submitBtn.disabled = false;
      } else {
        countdownCopy.textContent = 'Safety countdown: ' + remaining + 's';
      }
    }

    function startDangerCountdown() {
      if (!submitBtn) return;
      if (countdownTimer) clearInterval(countdownTimer);
      remaining = countdownSec;
      guardForm.querySelector('[name="countdown_ack"]').value = '0';
      submitBtn.disabled = true;
      setDangerCountdownState();
      countdownTimer = setInterval(function () {
        remaining -= 1;
        setDangerCountdownState();
        if (remaining <= 0 && countdownTimer) {
          clearInterval(countdownTimer);
          countdownTimer = null;
        }
      }, 1000);
    }

    shell.addEventListener('click', function (e) {
      var trigger = e.target.closest('[data-danger-open]');
      if (trigger) {
        var action = (trigger.getAttribute('data-danger-action') || '').toLowerCase();
        expectedWord = (trigger.getAttribute('data-danger-word') || '').toLowerCase();
        if (!action || !expectedWord) return;
        guardEl.classList.remove('hidden');
        guardForm.querySelector('[name="confirm_action"]').value = action;
        guardForm.setAttribute('hx-post', action === 'shutdown' ? '/api/system/shutdown' : '/api/system/restart');
        if (guardCopy) guardCopy.textContent = 'To ' + action + ' this device, type "' + expectedWord + '" and wait for the safety countdown.';
        if (guardHint) guardHint.textContent = 'Expected word: "' + expectedWord + '"';
        wordInput.focus();
        if (countdownBar) countdownBar.style.width = '0%';
        if (countdownCopy) countdownCopy.textContent = 'Countdown not started.';
        guardForm.querySelector('[name="countdown_ack"]').value = '0';
        if (submitBtn) submitBtn.disabled = true;
        return;
      }
      if (e.target.closest('[data-danger-cancel]')) {
        resetDangerGuard();
      }
    });

    wordInput.addEventListener('input', function () {
      var typed = (wordInput.value || '').trim().toLowerCase();
      if (!typed) {
        if (guardHint) guardHint.textContent = 'Type the word exactly.';
        if (countdownTimer) clearInterval(countdownTimer);
        countdownTimer = null;
        if (countdownBar) countdownBar.style.width = '0%';
        if (countdownCopy) countdownCopy.textContent = 'Countdown not started.';
        guardForm.querySelector('[name="countdown_ack"]').value = '0';
        if (submitBtn) submitBtn.disabled = true;
        return;
      }
      if (typed !== expectedWord) {
        if (guardHint) guardHint.textContent = 'Word does not match. Expected "' + expectedWord + '".';
        if (countdownTimer) clearInterval(countdownTimer);
        countdownTimer = null;
        if (countdownBar) countdownBar.style.width = '0%';
        if (countdownCopy) countdownCopy.textContent = 'Countdown blocked until word matches.';
        guardForm.querySelector('[name="countdown_ack"]').value = '0';
        if (submitBtn) submitBtn.disabled = true;
        return;
      }
      if (guardHint) guardHint.textContent = 'Word confirmed. Waiting safety countdown...';
      if (!countdownTimer && guardForm.querySelector('[name="countdown_ack"]').value !== '1') {
        startDangerCountdown();
      }
    });

    guardForm.addEventListener('htmx:afterRequest', function (e) {
      if (!(e.detail && e.detail.successful)) return;
      resetDangerGuard();
    });
  }

  function initSettings() {
    var shell = settingsShell();
    if (!shell || shell.__settingsInit) return;
    shell.__settingsInit = true;

    initAccordion(shell);

    shell.addEventListener('click', function (e) {
      var btn = e.target.closest('[data-retry-form]');
      if (!btn) return;
      var id = btn.getAttribute('data-retry-form');
      var form = id ? shell.querySelector('form[data-settings-form="' + id + '"]') : null;
      if (!form) return;
      form.requestSubmit();
    });

    shell.querySelectorAll('form[data-settings-form]').forEach(bindSettingsForm);

    initDangerGuard(shell);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initSettings(); });
  } else {
    initSettings();
  }

  // Re-init on hx-boost body swaps and history restores — the swapped-in
  // settings DOM is fresh, so the per-node guards let everything rebind.
  document.body.addEventListener('htmx:afterSwap', function () { initSettings(); });
  document.body.addEventListener('htmx:historyRestore', function () { initSettings(); });
})();
