/** #357: real TLS, browser storage and an actual cross-document cache restore. */
import { expect, test, type BrowserContext } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import process from 'node:process';

const ownerPath = process.env.PAT_AUDIT_OWNER;
test.skip(!ownerPath, 'Only the disposable PAT audit supplies owner credentials');
test.use({ viewport: { width: 390, height: 844 } });

test('one-time disclosure is absent from storage, real history restore and a signed-out tab', async ({ page, context }) => {
  const owner = JSON.parse(readFileSync(ownerPath!, 'utf8')) as {
    cookies: Awaited<ReturnType<BrowserContext['cookies']>>; password: string;
  };
  await context.addCookies(owner.cookies);
  await page.goto('/app/settings/security');
  await page.getByLabel('Token name', { exact: true }).fill('Independent browser audit');
  await page.getByLabel('Confirm your password', { exact: true }).fill(owner.password);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  const field = page.getByLabel('Your new token', { exact: true });
  await expect(field).toBeVisible();
  const token = await field.inputValue();
  // Assert booleans: even a failure must not print the synthetic credential.
  expect(/^rr_pat_[A-Za-z0-9_-]{43}$/.test(token)).toBe(true);
  const storage = await page.evaluate(async () => {
    const cached: string[] = [];
    for (const name of await caches.keys()) {
      const cache = await caches.open(name);
      for (const request of await cache.keys()) {
        cached.push(request.url);
        const response = await cache.match(request);
        if (response) cached.push(await response.text());
      }
    }
    return JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage }, cached });
  });
  expect(storage.includes(token)).toBe(false);
  await page.evaluate(() => {
    window.addEventListener('pageshow', (event) => {
      (window as unknown as { restored?: boolean }).restored = event.persisted;
    });
  });
  await page.goto('/pat-audit/away');
  await page.goBack({ waitUntil: 'commit' });
  const restored = await page.evaluate(() => (window as unknown as { restored?: boolean }).restored === true);
  expect(restored, 'must observe a real cache restore, not a reload or synthetic event').toBe(true);
  await expect(field).toHaveCount(0);
  expect(await page.evaluate((secret) => document.documentElement.outerHTML.includes(secret), token)).toBe(false);

  // Leave another disclosure visible while the same session signs out elsewhere.
  await page.getByLabel('Token name', { exact: true }).fill('Independent shared-tab audit');
  await page.getByLabel('Confirm your password', { exact: true }).fill(owner.password);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  await expect(field).toBeVisible();
  const sharedToken = await field.inputValue();
  const other = await context.newPage();
  await other.goto('https://rekenraam.test:8443/app');
  await other.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(other.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
  await other.close();
  await page.bringToFront();
  // As in #268, headless tabs do not change visibility. Exercise the
  // documented visibility/session-check boundary, not an instant broadcast.
  // The history restore above is an actual browser event and cache restore.
  await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')));
  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible({ timeout: 15_000 });
  await expect(field).toHaveCount(0);
  expect(await page.evaluate((secret) => document.documentElement.outerHTML.includes(secret), sharedToken)).toBe(false);
  const result = await page.evaluate(async (secret) => {
    const headers = { Authorization: `Bearer ${secret}` };
    const first = await fetch('/api/v1/accounts', { headers, credentials: 'omit' });
    const second = await fetch('/api/v1/accounts', { cache: 'force-cache', credentials: 'omit' });
    return { bearer: first.status, bearerCache: first.headers.get('cache-control'), anonymousCache: second.status };
  }, sharedToken);
  expect(result).toEqual({ bearer: 200, bearerCache: 'no-store', anonymousCache: 401 });
  if (process.env.PAT_AUDIT_BROWSER_OUTPUT) writeFileSync(process.env.PAT_AUDIT_BROWSER_OUTPUT, JSON.stringify({
    status: 'passed', viewport: '390x844', actualBackForwardCacheRestore: restored,
    localSessionCacheStorageSecretAbsent: true, historyDOMSecretAbsent: true,
    signedOutTabSecretAbsent: true, logoutPreservesSeparateBearerCredential: true,
    tabVisibilitySignal: 'dispatched-headless-event',
    sensitiveResponseNoStore: true, anonymousForceCacheStatus: 401
  }, null, 2) + '\n');
});
