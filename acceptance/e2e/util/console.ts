import type { Page } from '@playwright/test';

export interface PageProblems {
  consoleErrors: string[];
}

/**
 * Collect console errors for the page lifetime. CSP violations are included:
 * browsers log "Refused to load ..." refusals as console errors, which is
 * exactly the contract ("no CSP violations reported in the console").
 */
export function collectPageErrors(page: Page): PageProblems {
  const problems: PageProblems = { consoleErrors: [] };
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      problems.consoleErrors.push(msg.text());
    }
  });
  page.on('pageerror', (err) => {
    problems.consoleErrors.push(String(err));
  });
  return problems;
}
