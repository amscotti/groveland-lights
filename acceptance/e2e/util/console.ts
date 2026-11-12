import type { Page } from '@playwright/test';

export interface PageProblems {
  consoleErrors: string[];
  cspViolations: string[];
}

/** Collect console errors and CSP violation reports for the page lifetime. */
export function collectPageErrors(page: Page): PageProblems {
  const problems: PageProblems = { consoleErrors: [], cspViolations: [] };
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      problems.consoleErrors.push(msg.text());
    }
  });
  page.on('pageerror', (err) => {
    problems.consoleErrors.push(String(err));
  });
  page.on('securitypolicyviolation' as never, (event: { violatedDirective: string; blockedURI: string }) => {
    problems.cspViolations.push(`${event.violatedDirective}: ${event.blockedURI}`);
  });
  return problems;
}
