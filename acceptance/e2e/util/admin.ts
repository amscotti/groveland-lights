import { request, type Page } from '@playwright/test';
import { readFileSync, rmSync, writeFileSync } from 'node:fs';

/**
 * Admin login helper for the M2 drafts (A-201). ACTIVE util (no tests of its
 * own).
 *
 * Contract shape (T-203): ADMIN_TOKEN exchanges for an admin session via
 * GET /admin/login?token=..., the token is one-time GLOBALLY (the login_tokens
 * row's used_at is DB state — a replay answers 403 from any client), and the
 * minted gl_session cookie is valid for 24 h. Playwright gives every test a
 * FRESH context (fresh cookie jar) while the webServer shares one persistent
 * ./test-run.db across the whole run AND across the e2e → a11y invocations of
 * `make check` — so a naive "visit the token URL per test" login burns the
 * one-time token on the first test and 403s every login after it.
 *
 * This helper exchanges the token AT MOST ONCE per test-run.db lifetime and
 * reuses the session everywhere:
 *
 *   1. an in-process memo (one exchange per Playwright invocation), then
 *   2. a cookie cache file next to the DB (test-run.db.session.json — the
 *      .gitignore pattern `test-run.db*` covers it, so it lives and dies
 *      with the DB it belongs to and lets `make check`'s second Playwright
 *      invocation (a11y) reuse the e2e invocation's session), and
 *   3. a validation GET /admin before adoption — a dead session (expired,
 *      or the DB was recreated) is detected without consuming anything and
 *      falls through to a fresh exchange.
 *
 * When the token is already consumed AND no valid session can be recovered
 * (e.g. test-run.db outlived the 24 h session TTL), the helper fails with
 * the operator fix: `rm -f test-run.db*` and rerun.
 */

const DEFAULT_ADMIN_TOKEN = 'dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl';
const SESSION_COOKIE = 'gl_session';
/** Mirrors playwright.config.ts (the port MUST match in every process). */
const ORIGIN =
  process.env.BASE_URL ?? `http://127.0.0.1:${Number(process.env.TEST_PORT ?? 8931)}`;
/** Cache path shares the DB's gitignore pattern (test-run.db*) and lifetime. */
const CACHE_PATH = 'test-run.db.session.json';

interface SessionCache {
  origin: string;
  cookie: string;
  savedAt: number;
}

let memo: SessionCache | null = null;
let inFlight: Promise<SessionCache> | null = null;

/**
 * loginAsAdmin leaves `page` on an authenticated /admin. Safe to call from
 * every test on every project: no token is consumed when a reusable session
 * exists.
 */
export async function loginAsAdmin(page: Page): Promise<void> {
  await injectSessionCookie(page, await currentSession());
  await page.goto('/admin');
  if (!page.url().endsWith('/admin/login')) {
    return;
  }
  // The adopted session died between validation and use (DB swapped under
  // us, or the 24 h TTL expired mid-run). Invalidate and try exactly once
  // more with a forced re-validation.
  memo = null;
  clearCache();
  await injectSessionCookie(page, await currentSession());
  await page.goto('/admin');
  if (page.url().endsWith('/admin/login')) {
    throw new Error(
      'loginAsAdmin: no valid admin session and the token exchange did not ' +
        'mint one. If ADMIN_TOKEN was already consumed in this server\'s DB ' +
        '(one-time, A-201), remove the runtime DB and cache ' +
        `(rm -f ${CACHE_PATH} test-run.db*) and rerun.`,
    );
  }
}

/** Returns a session proven valid for this origin, exchanging at most once. */
async function currentSession(): Promise<SessionCache> {
  if (inFlight) {
    return inFlight;
  }
  inFlight = (async () => {
    const candidates = [memo, readCache()];
    for (const candidate of candidates) {
      if (candidate && candidate.origin === ORIGIN && (await sessionWorks(candidate))) {
        memo = candidate;
        return candidate;
      }
    }
    memo = await exchangeToken();
    writeCache(memo);
    return memo;
  })().finally(() => {
    inFlight = null;
  });
  return inFlight;
}

/** A session works when /admin accepts it instead of bouncing to the login. */
async function sessionWorks(session: SessionCache): Promise<boolean> {
  const api = await request.newContext({
    baseURL: ORIGIN,
    extraHTTPHeaders: { Cookie: `${SESSION_COOKIE}=${session.cookie}` },
  });
  try {
    const res = await api.get('/admin', { maxRedirects: 0 });
    return (
      res.status() === 200 &&
      !(res.headers()['location'] ?? '').includes('/admin/login')
    );
  } catch {
    return false;
  } finally {
    await api.dispose();
  }
}

/**
 * The one-time exchange: GET /admin/login?token=... must answer 302 to /admin
 * with a gl_session Set-Cookie (A-201). Called at most once per DB lifetime.
 */
async function exchangeToken(): Promise<SessionCache> {
  const token = process.env.ADMIN_TOKEN ?? DEFAULT_ADMIN_TOKEN;
  const api = await request.newContext({ baseURL: ORIGIN });
  try {
    const res = await api.get(`/admin/login?token=${encodeURIComponent(token)}`, {
      maxRedirects: 0,
    });
    const setCookie = res.headers()['set-cookie'] ?? '';
    const match = setCookie.match(new RegExp(`${SESSION_COOKIE}=([^;]+)`));
    if (res.status() !== 302 || !match) {
      throw new Error(
        `loginAsAdmin: token exchange failed (status ${res.status()}). ` +
          'Either ADMIN_TOKEN is wrong for this server, or the token was ' +
          'already consumed (one-time, A-201). Remove the runtime DB and ' +
          `cache (rm -f ${CACHE_PATH} test-run.db*) and rerun.`,
      );
    }
    return { origin: ORIGIN, cookie: match[1], savedAt: Date.now() };
  } finally {
    await api.dispose();
  }
}

async function injectSessionCookie(page: Page, session: SessionCache): Promise<void> {
  await page.context().addCookies([
    {
      name: SESSION_COOKIE,
      value: session.cookie,
      url: ORIGIN,
      httpOnly: true,
      sameSite: 'Lax',
    },
  ]);
}

function readCache(): SessionCache | null {
  try {
    const parsed = JSON.parse(readFileSync(CACHE_PATH, 'utf8')) as SessionCache;
    return typeof parsed.cookie === 'string' && parsed.cookie !== ''
      ? parsed
      : null;
  } catch {
    return null;
  }
}

function writeCache(session: SessionCache): void {
  try {
    writeFileSync(CACHE_PATH, JSON.stringify(session), { mode: 0o600 });
  } catch (err) {
    // NOT a harmless degradation: ADMIN_TOKEN is one-time GLOBAL, so the
    // next Playwright invocation sharing this DB (make check: e2e then
    // a11y) cannot re-exchange and will fail its login. The invocation at
    // hand still succeeds via the in-process memo; make check therefore
    // REQUIRES a writable cache path (the repo root, next to test-run.db).
    // Surface the reason on stderr instead of failing the passing run.
    process.stderr.write(
      `loginAsAdmin: cannot write ${CACHE_PATH} ` +
        `(${err instanceof Error ? err.message : String(err)}). The current run ` +
        'succeeds via its in-process session, but the next invocation against ' +
        'this DB (make check: a11y after e2e) will find no cached session and ' +
        'the one-time token is already consumed. Fix the write or remove ' +
        'test-run.db* for a clean cycle.\n',
    );
  }
}

function clearCache(): void {
  try {
    rmSync(CACHE_PATH);
  } catch {
    // Best effort only; see writeCache.
  }
}
