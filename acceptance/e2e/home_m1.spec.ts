// M1 acceptance specs (T-100): A-101, A-102.
//
// Both are regression guards against the deployed skeleton and must stay green
// at every gate. They run on all four Playwright projects (desktop-chromium,
// pixel-9, iphone-16, iphone-se-320) because they are plain `test()` blocks in
// testDir 'acceptance/e2e' with no project filter.
//
// Visual note (A-102, what a correct screenshot shows): the Groveland Lights
// home page — header nav with "Groveland Lights" brand and Map/List/About
// links, an empty main container, and the "A community project…" footer. No
// browser error overlay, no unstyled flash (CSS loads from 'self'), and (once
// T-103 lands) a populated probe section below the heading. Nothing blocked by
// CSP: all scripts/styles come from /static on the same origin.

import { test, expect } from '@playwright/test';
import { collectPageErrors } from './util/console.js';

test('@A-102 home renders with strict CSP, zero console errors and zero CSP violations', async ({
  page,
}) => {
  // Collect console/page errors for the page lifetime (A-000 style)…
  const problems = collectPageErrors(page);

  // …and register the CSP-violation listener BEFORE goto, so a violation
  // triggered while loading the document itself is still captured. The
  // init script runs before any page script on every navigation.
  await page.addInitScript(() => {
    const w = window as unknown as { __cspViolations: string[] };
    w.__cspViolations = [];
    document.addEventListener('securitypolicyviolation', (event) => {
      w.__cspViolations.push(event.violatedDirective);
    });
  });

  const response = await page.goto('/');
  expect(response?.status()).toBe(200);

  const csp = response?.headers()['content-security-policy'] ?? '';
  expect(csp).toContain("default-src 'self'");
  expect(csp).toContain("form-action 'self'");
  expect(csp).toContain("frame-ancestors 'none'");
  expect(csp).toContain("base-uri 'none'");

  // goto waits for 'load', so deferred scripts (htmx, alpine, app.js) have
  // run and any error they raise is already recorded.
  expect(problems.consoleErrors).toEqual([]);

  const violations = (await page.evaluate(() =>
    (window as unknown as { __cspViolations?: string[] }).__cspViolations,
  )) as string[];
  expect(violations).toEqual([]);
});

test('@A-101 @smoke healthz answers ok', async ({ request }) => {
  const res = await request.get('/healthz');
  expect(res.status()).toBe(200);
  expect(await res.text()).toBe('ok');
});
