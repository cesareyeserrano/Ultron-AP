// Sidebar: collapse/expand on desktop, slide-over on mobile.
//
// This file used to capture #sidebar and the buttons ONCE at load and
// hold them in closure variables. hx-boost swaps the whole body on every
// navigation, so after the first click those references pointed at detached
// nodes: the new sidebar came back with the server's classes, nobody reapplied
// the stored state, and the collapse was lost. Now nothing is cached — every
// handler re-queries the DOM — and the state is reapplied on htmx:afterSwap and
// htmx:historyRestore, the same lifecycle the widgets use.
(function () {
  'use strict';

  var STORAGE_KEY = 'ultron-sidebar-collapsed';

  function sidebar() {
    return document.getElementById('sidebar');
  }

  function isStoredCollapsed() {
    try {
      return localStorage.getItem(STORAGE_KEY) === 'true';
    } catch (e) {
      return false;
    }
  }

  function saveState(collapsed) {
    try {
      localStorage.setItem(STORAGE_KEY, collapsed ? 'true' : 'false');
    } catch (e) {
      // localStorage unavailable (private mode) — the sidebar still works,
      // it just forgets the preference between page loads.
    }
  }

  function isMobile() {
    return window.innerWidth < 768;
  }

  function isTablet() {
    return window.innerWidth >= 768 && window.innerWidth < 1024;
  }

  function updateToggleIcons() {
    var el = sidebar();
    var toggleBtn = document.getElementById('sidebar-toggle');
    if (!el || !toggleBtn) return;

    var collapsed = el.classList.contains('sidebar-collapsed');
    var expandedIcon = toggleBtn.querySelector('.sidebar-expanded-icon');
    var collapsedIcon = toggleBtn.querySelector('.sidebar-collapsed-icon');
    if (expandedIcon) expandedIcon.classList.toggle('hidden', collapsed);
    if (collapsedIcon) collapsedIcon.classList.toggle('hidden', !collapsed);
    toggleBtn.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
  }

  // applyState re-derives the sidebar's classes from the viewport and the stored
  // preference. It runs on load AND after every swap, because a swapped-in
  // sidebar arrives with the server's default classes and knows nothing about
  // what the admin chose.
  function applyState() {
    var el = sidebar();
    if (!el) return;

    if (isMobile()) {
      el.classList.add('sidebar-hidden');
      el.classList.remove('sidebar-collapsed');
    } else if (isTablet()) {
      el.classList.add('sidebar-collapsed');
      el.classList.remove('sidebar-hidden');
    } else {
      el.classList.toggle('sidebar-collapsed', isStoredCollapsed());
      el.classList.remove('sidebar-hidden');
    }
    updateToggleIcons();
  }

  function toggleCollapse() {
    var el = sidebar();
    if (!el) return;
    var collapsed = !el.classList.contains('sidebar-collapsed');
    el.classList.toggle('sidebar-collapsed', collapsed);
    // On desktop the slide-over class must not linger, or its md: width rule
    // fights the collapsed width and the sidebar stops resizing.
    el.classList.remove('sidebar-hidden');
    saveState(collapsed);
    updateToggleIcons();
  }

  function toggleMobile() {
    var el = sidebar();
    var overlay = document.getElementById('sidebar-overlay');
    if (!el) return;
    el.classList.toggle('sidebar-hidden');
    if (overlay) overlay.classList.toggle('hidden');
  }

  function closeMobile() {
    var el = sidebar();
    var overlay = document.getElementById('sidebar-overlay');
    if (el) el.classList.add('sidebar-hidden');
    if (overlay) overlay.classList.add('hidden');
  }

  // One delegated listener on the document survives every body swap — the
  // buttons themselves are replaced, so binding to them directly is what broke.
  document.addEventListener('click', function (e) {
    if (!e.target.closest) return;

    if (e.target.closest('#sidebar-toggle')) {
      e.preventDefault();
      toggleCollapse();
      return;
    }
    if (e.target.closest('#hamburger-btn')) {
      e.preventDefault();
      toggleMobile();
      return;
    }
    if (e.target.closest('#sidebar-overlay')) {
      closeMobile();
    }
  });

  var resizeTimer;
  window.addEventListener('resize', function () {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(applyState, 150);
  });

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', applyState);
  } else {
    applyState();
  }

  // A boosted navigation replaces the sidebar with the server's markup, which
  // carries no memory of the admin's choice. Reapply it.
  //
  // afterSettle, not just afterSwap: with hx-boost the body is still being
  // settled when afterSwap fires, so classes written then land on a node htmx
  // is about to discard — the sidebar visibly reverted to expanded on the first
  // navigation and only behaved from the second one on. afterSettle is
  // the last event of the cycle, when the new sidebar is definitively in place.
  // Both are harmless to run twice: applyState is idempotent.
  document.addEventListener('htmx:afterSwap', applyState);
  document.addEventListener('htmx:afterSettle', applyState);
  document.addEventListener('htmx:historyRestore', applyState);
})();
