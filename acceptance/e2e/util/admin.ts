import type { Page } from '@playwright/test';

/**
 * Admin login helper for the M2 drafts (A-201). ACTIVE util (no tests of its
 * own): it exchanges the ADMIN_TOKEN for an admin session by visiting the
 * one-time login URL, then waits for the redirect that proves the session
 * cookie landed. The token matches .env.test / playwright.config.ts /
 * scripts/holdout-run.sh; ADMIN_TOKEN in the environment (when a runner
 * exports it) wins.
 */
const DEFAULT_ADMIN_TOKEN = 'dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl';

export async function loginAsAdmin(page: Page): Promise<void> {
  const token = process.env.ADMIN_TOKEN ?? DEFAULT_ADMIN_TOKEN;
  await page.goto(`/admin/login?token=${encodeURIComponent(token)}`);
  // The valid-token exchange answers 302 to /admin (A-201); a wrong or
  // replayed token would land on 403 instead and this wait would fail.
  await page.waitForURL(/\/admin(?:\?|$)/);
}
