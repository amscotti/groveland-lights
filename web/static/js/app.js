// @ts-check
// Global client bootstrap. CSP-safe: no eval, no inline expressions
// (htmx.config.allowEval=false; Alpine is the CSP build).

(function () {
  'use strict';

  window.addEventListener('DOMContentLoaded', function () {
    if (window.htmx) {
      window.htmx.config.allowEval = false;
    }
    if (window.Alpine) {
      window.Alpine.data('glPage', function () {
        return {
          envLabel: (document.querySelector('.env-note') || {}).dataset ? document.querySelector('.env-note').dataset.env : 'dev'
        };
      });
    }
  });
})();
