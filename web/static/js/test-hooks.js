// @ts-check
// Test hooks: only served when APP_ENV=test (§5.1). A Go test asserts the prod
// render never includes this file.
(function () {
  'use strict';
  window.__glMapInits = 0;
  window.__map = null;
})();
