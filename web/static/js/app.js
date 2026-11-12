// @ts-check
// Global client bootstrap. CSP-safe: no eval, no inline expressions
// (htmx.config.allowEval=false; Alpine is the CSP build).

(function () {
  'use strict';

  function boot() {
    if (window.htmx) {
      window.htmx.config.allowEval = false;
    }
    if (window.Alpine) {
      window.Alpine.data('glPage', function () {
        var note = document.querySelector('.env-note');
        return {
          envLabel: note instanceof HTMLElement ? String(note.dataset.env ?? '') : 'dev'
        };
      });
    }
  }

  window.addEventListener('DOMContentLoaded', boot);
})();
