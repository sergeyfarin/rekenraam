/**
 * In-page API access for e2e runs.
 *
 * Requests go through `page.evaluate` rather than Playwright's request context
 * so they carry the browser's session cookie — the same one the UI under test
 * uses — instead of a second, separately authenticated session.
 */

import type { Page } from '@playwright/test';

/**
 * Calls the API from inside the page and returns the decoded JSON body.
 *
 * `csrfTokenOrExpected` is overloaded: pass the CSRF token for mutations, or
 * the accepted status codes when no token is needed. Setup endpoints that are
 * idempotent-by-conflict are called as `apiJSON(page, 'POST', path, token,
 * undefined, [201, 409])`.
 */
export async function apiJSON<T = unknown>(
  page: Page,
  method: 'GET' | 'POST',
  path: string,
  csrfTokenOrExpected?: string | number[],
  body?: unknown,
  expectedStatuses: number[] = method === 'POST' ? [200, 201] : [200]
): Promise<T> {
  const csrfToken = typeof csrfTokenOrExpected === 'string' ? csrfTokenOrExpected : undefined;
  const statuses = Array.isArray(csrfTokenOrExpected) ? csrfTokenOrExpected : expectedStatuses;
  const response = await page.evaluate(
    async ({ method, path, csrfToken, body }) => {
      const headers: Record<string, string> = {};
      if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
      if (body !== undefined) headers['Content-Type'] = 'application/json';
      const res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
      return { status: res.status, text: await res.text() };
    },
    { method, path, csrfToken, body }
  );
  if (!statuses.includes(response.status)) throw new Error(`${method} ${path} failed with ${response.status}: ${response.text}`);
  return response.status === 204 || response.text === '' ? (undefined as T) : (JSON.parse(response.text) as T);
}

/**
 * The current session's CSRF token.
 *
 * The token rotates on privilege-changing requests, so re-read it after each
 * mutation rather than caching one for a whole test.
 */
export async function csrfTokenFor(page: Page): Promise<string> {
  const session = await apiJSON<{ authenticated: boolean; csrf_token?: string }>(page, 'GET', '/api/v1/auth/session');
  if (!session.authenticated || !session.csrf_token) throw new Error('expected authenticated session with CSRF token');
  return session.csrf_token;
}
