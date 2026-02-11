(function () {
  "use strict";

  var STORAGE_KEY = "ultron-sidebar-collapsed";
  var sidebar = document.getElementById("sidebar");
  var overlay = document.getElementById("sidebar-overlay");
  var toggleBtn = document.getElementById("sidebar-toggle");
  var hamburgerBtn = document.getElementById("hamburger-btn");

  if (!sidebar) return;

  // Read saved state from localStorage
  function isStoredCollapsed() {
    try {
      return localStorage.getItem(STORAGE_KEY) === "true";
    } catch (e) {
      return false;
    }
  }

  function saveState(collapsed) {
    try {
      localStorage.setItem(STORAGE_KEY, collapsed ? "true" : "false");
    } catch (e) {
      // localStorage not available — ignore
    }
  }

  function isMobile() {
    return window.innerWidth < 768;
  }

  function isTablet() {
    return window.innerWidth >= 768 && window.innerWidth < 1024;
  }

  // Apply initial state
  function applyInitialState() {
    if (isMobile()) {
      sidebar.classList.add("sidebar-hidden");
      sidebar.classList.remove("sidebar-collapsed");
    } else if (isTablet()) {
      sidebar.classList.add("sidebar-collapsed");
      sidebar.classList.remove("sidebar-hidden");
    } else {
      // Desktop — use stored preference
      if (isStoredCollapsed()) {
        sidebar.classList.add("sidebar-collapsed");
      } else {
        sidebar.classList.remove("sidebar-collapsed");
      }
      sidebar.classList.remove("sidebar-hidden");
    }
  }

  // Toggle collapse (desktop/tablet)
  function toggleCollapse() {
    sidebar.classList.toggle("sidebar-collapsed");
    saveState(sidebar.classList.contains("sidebar-collapsed"));
  }

  // Toggle mobile menu
  function toggleMobile() {
    sidebar.classList.toggle("sidebar-hidden");
    if (overlay) {
      overlay.classList.toggle("hidden");
    }
  }

  // Close mobile menu when clicking overlay
  function closeMobile() {
    sidebar.classList.add("sidebar-hidden");
    if (overlay) {
      overlay.classList.add("hidden");
    }
  }

  // Bind events
  if (toggleBtn) {
    toggleBtn.addEventListener("click", toggleCollapse);
  }

  if (hamburgerBtn) {
    hamburgerBtn.addEventListener("click", toggleMobile);
  }

  if (overlay) {
    overlay.addEventListener("click", closeMobile);
  }

  // Handle resize
  var resizeTimer;
  window.addEventListener("resize", function () {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(applyInitialState, 150);
  });

  // Initialize
  applyInitialState();

  // Re-init after HTMX page swap
  document.addEventListener("htmx:afterSwap", function () {
    applyInitialState();
  });
})();
