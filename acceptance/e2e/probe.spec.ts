// @A-103 @smoke @a11y — /probe counter increments and persists (frozen
// contract, written by T-100 before the feature tasks branch).
//
// INERT DRAFT: the `.spec.ts.txt` extension means no runner loads this file.
// T-105 activates it verbatim, with a rename only and NO content edits:
//
//   git mv acceptance/drafts/probe.spec.ts.txt acceptance/e2e/probe.spec.ts
//
// Contract owners: T-102 (CSRF helpers), T-103 (probe handler + home section).
// The exact strings this spec relies on are listed in acceptance/drafts/README.md.
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

  test('@A-103 no-JS fallback form increments via 303 redirect', async ({ page }) => {
    await page.goto('/');

    // With JS off nothing is fetched client-side: the probe section is
    // server-rendered, with a plain form.
    const section = page.locator('section#probe');
    await expect(section).toBeVisible();
    await expect(section.getByRole('heading', { name: 'Probe', exact: true })).toBeVisible();
    await expect(section.getByText(/^Count:\s*\d+$/)).toBeVisible();
    const count = page.locator('#probe-count');
    await expect(count).toHaveText(/^\d+$/);
    const before = Number(await count.innerText());

    // Submitting the form POSTs /probe; the server answers 303 -> /, so the
    // browser lands back on the home page with the incremented count.
    await page.getByRole('button', { name: 'Increment counter', exact: true }).click();
    await expect(page.locator('#probe-count')).toHaveText(String(before + 1));
    expect(new URL(page.url()).pathname).toBe('/');
  });
});
