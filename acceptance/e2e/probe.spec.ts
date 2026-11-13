// @A-103 @smoke @a11y — /probe counter increments and persists (frozen
// contract, written by T-100 before the feature tasks branch).
//
// ACTIVATED by T-105 from acceptance/drafts/probe.spec.ts.txt (the draft's
// byte-exact text stays auditable in git history). Activation made exactly
// ONE content edit, in the no-JS test at the bottom of this file: the draft
// required the populated probe section to be inlined on / with JS off, but
// the operative feature contract (harness/tasks/T-103.md) renders the home
// section as the empty HTMX shell plus a plain <a href="/probe">Probe</a>
// link — "a <noscript>-friendly static form is NOT needed … the no-JS flow
// reaches the same form via a plain link". The edit follows that link and
// keeps every capability assertion (server-rendered count, plain-form POST,
// 303 -> /, persisted N+1) while adding a check for the link itself. Full
// judgment trail and ratification request: harness/handoffs/T-105.md.
// Contract owners: T-102 (CSRF helpers), T-103 (probe handler + home
// section). Exact strings: harness/tasks/T-100.md and T-103.md (the drafts
// README was removed together with drafts/ at activation).
//
// Visual note (what a correct screenshot shows): below the home-page nav, a
// bordered section with the heading "Probe", the line "Count: N" with the
// digits inside #probe-count, and an "Increment counter" button. Clicking
// ticks the digit up in place — no page reload, no URL change. On the 320px
// iPhone SE viewport the section stacks vertically without overflow.

import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

test('@A-103 @smoke counter increments via HTMX and persists across reloads', async ({ page }) => {
  await page.goto('/');

  // 1. The <section id="probe"> becomes populated: HTMX loads /probe on page
  //    load. Waiting for the heading proves the fragment arrived.
  const section = page.locator('section#probe');
  await expect(section).toBeVisible();
  await expect(section.getByRole('heading', { name: 'Probe', exact: true })).toBeVisible();

  // 2. "Count: N" with N a non-negative integer; #probe-count holds N.
  await expect(section.getByText(/^Count:\s*\d+$/)).toBeVisible();
  const count = page.locator('#probe-count');
  await expect(count).toHaveText(/^\d+$/);
  const before = Number(await count.innerText());

  // 6. Attach the observed counts so deployed runs (make smoke BASE_URL=...)
  //    prove counter persistence across deploys: a later deploy must observe
  //    the "after" value as its "before".
  test.info().annotations.push({ type: 'probe-counter', description: `before=${before}` });

  // 3. Clicking the button increments #probe-count to N+1 with no page
  //    navigation (main frame must not navigate; URL must not change).
  let navigated = false;
  page.on('framenavigated', (frame) => {
    if (frame === page.mainFrame()) {
      navigated = true;
    }
  });
  const urlBefore = page.url();
  await page.getByRole('button', { name: 'Increment counter', exact: true }).click();
  await expect(count).toHaveText(String(before + 1));
  expect(navigated).toBe(false);
  expect(page.url()).toBe(urlBefore);

  // 4. A full reload still shows N+1: the counter lives in SQLite, not in the
  //    page. The probe section repopulates via HTMX after reload.
  await page.reload();
  const sectionAfter = page.locator('section#probe');
  await expect(sectionAfter.getByRole('heading', { name: 'Probe', exact: true })).toBeVisible();
  await expect(page.locator('#probe-count')).toHaveText(String(before + 1));
  test.info().annotations.push({ type: 'probe-counter', description: `after=${before + 1}` });
});

test('@A-103 @a11y home with populated probe has no axe violations', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Probe', exact: true })).toBeVisible();
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  expect(results.violations).toEqual([]);
});

test.describe('probe without JavaScript', () => {
  test.use({ javaScriptEnabled: false });

  // Visual note (JS off, what a correct screenshot shows): / renders the
  // section#probe shell containing the plain "Probe" link; following it
  // shows the server-rendered fragment page — heading "Probe", the line
  // "Count: N" with the digits inside #probe-count, and the "Increment
  // counter" button; submitting performs a full-page load that lands back
  // on / (the shell again), and following the link once more shows N+1.
  // (The frozen draft instead expected the populated section inlined on /
  // with JS off — superseded by T-103's shell+link contract; see header.)

  test('@A-103 no-JS fallback form increments via 303 redirect', async ({ page }) => {
    await page.goto('/');

    // With JS off nothing is fetched client-side: the home section is the
    // server-rendered shell, and its plain link is the no-JS entry point
    // (T-103's contract pins the link inside section#probe).
    const section = page.locator('section#probe');
    await expect(section).toBeVisible();
    const probeLink = section.getByRole('link', { name: 'Probe', exact: true });
    await expect(probeLink).toBeVisible();
    await expect(probeLink).toHaveAttribute('href', '/probe');

    // The link leads to the server-rendered probe fragment page: heading,
    // count, and a plain form — no client-side fetch happened to get here.
    await probeLink.click();
    await expect(page.getByRole('heading', { name: 'Probe', exact: true })).toBeVisible();
    await expect(page.getByText(/^Count:\s*\d+$/)).toBeVisible();
    const count = page.locator('#probe-count');
    await expect(count).toHaveText(/^\d+$/);
    const before = Number(await count.innerText());

    // Submitting the form POSTs /probe; the server answers 303 -> /, so the
    // browser lands back on the home page.
    await page.getByRole('button', { name: 'Increment counter', exact: true }).click();
    await expect(page.locator('section#probe')).toBeVisible();
    expect(new URL(page.url()).pathname).toBe('/');

    // The incremented count is readable on the same no-JS path: the shell's
    // plain link now leads to the fragment showing N+1 (persisted server-side).
    await page.locator('section#probe').getByRole('link', { name: 'Probe', exact: true }).click();
    await expect(page.locator('#probe-count')).toHaveText(String(before + 1));
  });
});
