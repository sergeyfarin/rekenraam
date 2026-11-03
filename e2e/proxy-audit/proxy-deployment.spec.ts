/**
 * #268: production reverse-proxy and shared-browser privacy, observed through
 * a real Caddy TLS proxy in front of the supplied production container.
 *
 * Phases follow the deployment's life and are selected by PROXY_AUDIT_PHASE,
 * because the restarts between them happen outside the browser:
 *
 *   deploy        fresh volume: setup-token onboarding, cookies, MFA, tabs,
 *                 back/forward cache and cached API reads
 *   restart       same REKENRAAM_SECRET_KEY: TOTP still signs in
 *   key-changed   rotated or missing key: TOTP refused, recovery code works
 *
 * The secret and recovery codes captured in `deploy` are handed to the later
 * phases through PROXY_AUDIT_STATE, a file in the auditor's scratch space.
 * The book is synthetic; nothing here is real owner data.
 */

import { type BrowserContext, expect, type Page, test } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import process from 'node:process';
import { currentStep, secretFromOTPAuthURI, totpCode } from '../playwright/support/totp';
import { activateBrowserMFA } from '../playwright/support/mfa-enrollment';

const OWNER_USERNAME = 'owner';
const OWNER_PASSWORD = 'proxy-audit-password';
/** A value no other screen would render, so seeing it means the financial view leaked. */
const SENTINEL_ACCOUNT = 'Proxy audit savings 4242';

const phase = process.env.PROXY_AUDIT_PHASE ?? 'deploy';
const statePath = process.env.PROXY_AUDIT_STATE ?? '';
const setupToken = process.env.SETUP_TOKEN ?? '';

type DeviceCookie = Awaited<ReturnType<BrowserContext['cookies']>>[number];
/**
 * `deviceCookie` is the owner's approved-device cookie. The later phases sign
 * in with it, as the owner's own browser would, so their attempts spend that
 * device's throttle budget rather than the shared one every client behind the
 * proxy's address competes for.
 */
type AuditState = { secret: string; recoveryCodes: string[]; lastStep: number; deviceCookie?: DeviceCookie };

function readState(): AuditState {
  return JSON.parse(readFileSync(statePath, 'utf8')) as AuditState;
}

function writeState(state: AuditState) {
  writeFileSync(statePath, JSON.stringify(state), { mode: 0o600 });
}

/**
 * A TOTP code for a step the server's replay guard has not seen. The server
 * accepts one step either side of now, so a run of sign-ins waits for the
 * clock rather than reach past that window.
 */
async function nextCode(state: AuditState): Promise<string> {
  while (state.lastStep + 1 > currentStep() + 1) {
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  const step = Math.max(currentStep(), state.lastStep + 1);
  state.lastStep = step;
  writeState(state);
  return totpCode(state.secret, step);
}

async function apiStatus(page: Page, path: string, init?: RequestInit) {
  return page.evaluate(
    async ({ path, init }) => {
      const res = await fetch(path, init);
      return { status: res.status, cacheControl: res.headers.get('cache-control'), body: await res.text() };
    },
    { path, init }
  );
}

async function csrf(page: Page): Promise<string> {
  const res = await apiStatus(page, '/api/v1/auth/session');
  const body = JSON.parse(res.body) as { csrf_token?: string };
  if (!body.csrf_token) throw new Error(`no csrf token: ${res.body}`);
  return body.csrf_token;
}

async function postJSON(page: Page, path: string, body?: unknown) {
  const token = await csrf(page);
  return apiStatus(page, path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': token },
    body: body === undefined ? undefined : JSON.stringify(body)
  });
}

async function signInWithPassword(page: Page) {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
  await page.getByLabel('Username').fill(OWNER_USERNAME);
  await page.getByLabel('Password').fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByLabel('Authentication code')).toBeVisible();
}

async function verifyCode(page: Page, code: string) {
  await page.getByLabel('Authentication code').fill(code);
  await page.getByRole('button', { name: 'Verify' }).click();
}

async function cookieByName(context: BrowserContext, name: string) {
  return (await context.cookies()).find((cookie) => cookie.name === name);
}

test.describe.serial('deploy phase', () => {
  test.skip(phase !== 'deploy', 'only on a fresh deployment');

  let context: BrowserContext;
  let page: Page;
  const state: AuditState = { secret: '', recoveryCodes: [], lastStep: 0 };

  test.beforeAll(async ({ browser }) => {
    expect(statePath, 'PROXY_AUDIT_STATE must name a scratch file').not.toEqual('');
    expect(setupToken.length, 'SETUP_TOKEN must be the deployment token').toBeGreaterThanOrEqual(32);
    context = await browser.newContext();
    page = await context.newPage();
  });

  test.afterAll(async () => {
    if (state.secret) {
      state.deviceCookie = await cookieByName(context, '__Host-rekenraam_device');
      writeState(state);
    }
    await context.close();
  });

  test('onboards the owner over HTTPS only with the setup token, under __Host- cookies', async () => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Create the owner account' })).toBeVisible();
    expect(await page.evaluate(() => window.isSecureContext)).toBe(true);

    await page.getByLabel('Username').fill(OWNER_USERNAME);
    await page.getByLabel('Password').fill(OWNER_PASSWORD);
    await page.getByLabel('Setup token').fill('not-the-deployment-token-but-long-enough');
    await page.getByRole('button', { name: 'Create owner' }).click();
    await expect(page.getByRole('alert')).toBeVisible();
    expect(await context.cookies()).toHaveLength(0);

    await page.getByLabel('Setup token').fill(setupToken);
    await page.getByRole('button', { name: 'Create owner' }).click();
    await expect(page.getByRole('heading', { name: 'Choose default currencies' })).toBeVisible();
    await page.getByRole('button', { name: /USD/ }).first().click();
    await page.getByRole('button', { name: 'Save currencies' }).click();
    await expect(page).toHaveURL(/\/app$/);

    // The token is sent as a header, never kept by the browser.
    const storage = await page.evaluate(() => JSON.stringify({ ...localStorage }) + JSON.stringify({ ...sessionStorage }));
    expect(storage).not.toContain(setupToken);

    const names = (await context.cookies()).map((cookie) => cookie.name).sort();
    expect(names).toEqual(['__Host-rekenraam_device', '__Host-rekenraam_session']);
    for (const cookie of await context.cookies()) {
      expect(cookie, cookie.name).toMatchObject({ secure: true, httpOnly: true, sameSite: 'Strict', path: '/', domain: 'rekenraam.test' });
    }
  });

  test('seeds a synthetic financial view', async () => {
    expect((await postJSON(page, '/api/v1/setup/system-accounts')).status).toBe(201);
    expect((await postJSON(page, '/api/v1/setup/categories')).status).toBe(201);
    const currencies = JSON.parse((await apiStatus(page, '/api/v1/currencies')).body) as { currencies: Array<{ id: number }> };
    const created = await postJSON(page, '/api/v1/accounts', {
      name: SENTINEL_ACCOUNT,
      account_class: 'asset',
      account_kind: 'checking',
      default_commodity_id: currencies.currencies[0].id,
      allows_postings: true
    });
    expect(created.status, created.body).toBe(201);

    const read = await apiStatus(page, '/api/v1/accounts');
    expect(read.status).toBe(200);
    expect(read.body).toContain(SENTINEL_ACCOUNT);
    expect(read.cacheControl).toBe('no-store');
  });

  test('enrolls MFA through the proxy', async () => {
    await page.goto('/app/settings/security');
    await expect(page.getByText('Off', { exact: true })).toBeVisible();
    await expect(page.getByText(/needs REKENRAAM_SECRET_KEY/)).toHaveCount(0);
    await page.getByLabel('Your password', { exact: true }).fill(OWNER_PASSWORD);
    await page.getByRole('button', { name: 'Set up' }).click();
    state.secret = secretFromOTPAuthURI(await page.getByText(/^otpauth:\/\//).innerText());
    state.lastStep = await activateBrowserMFA(page, state.secret, {
      code: page.getByLabel('Code from your app'),
      submit: page.getByRole('button', { name: 'Turn on' })
    }, { reserveCurrentStep: true });
    await expect(page.getByRole('heading', { name: 'Recovery codes' })).toBeVisible();
    state.recoveryCodes = (await page.locator('ul.font-mono li').allInnerTexts()).map((code) => code.trim());
    expect(state.recoveryCodes).toHaveLength(10);
    writeState(state);
    await page.getByRole('button', { name: 'I have saved them' }).click();
    await expect(page.getByText('On', { exact: true })).toBeVisible();
  });

  test('signs in through an MFA challenge cookie that grants nothing', async () => {
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
    expect(await cookieByName(context, '__Host-rekenraam_session')).toBeUndefined();

    await signInWithPassword(page);
    const challenge = await cookieByName(context, '__Host-rekenraam_mfa');
    expect(challenge).toMatchObject({ secure: true, httpOnly: true, sameSite: 'Strict', path: '/' });
    expect((await apiStatus(page, '/api/v1/accounts')).status).toBe(401);

    await verifyCode(page, await nextCode(state));
    await expect(page).toHaveURL(/\/app$/);
    expect(await cookieByName(context, '__Host-rekenraam_mfa')).toBeUndefined();
    expect(await cookieByName(context, '__Host-rekenraam_session')).toBeDefined();
  });

  test('sign-out in one tab ends the other tab without showing it new data', async () => {
    const other = await context.newPage();
    await other.goto('/app/ledger/accounts');
    await expect(other.getByText(SENTINEL_ACCOUNT).first()).toBeVisible();

    await page.bringToFront();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();

    // The other tab's cookie is the same jar: its next read is refused.
    expect((await apiStatus(other, '/api/v1/accounts')).status).toBe(401);

    // Until it re-checks, the other tab still shows what it had rendered.
    await expect(other.getByText(SENTINEL_ACCOUNT).first()).toBeVisible();

    // Switching back to a tab fires visibilitychange, which TanStack Query
    // treats as focus; the session query refetches 'always' and the shell
    // leaves. Headless tabs never change visibility, so dispatch it.
    await other.bringToFront();
    await other.evaluate(() => window.dispatchEvent(new Event('visibilitychange')));
    await expect(other.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible({ timeout: 5_000 });
    await expect(other.getByText(SENTINEL_ACCOUNT)).toHaveCount(0);
    await other.close();
  });

  test('back navigation after sign-out does not reveal the financial view', async () => {
    await signInWithPassword(page);
    await verifyCode(page, await nextCode(state));
    await expect(page).toHaveURL(/\/app$/);
    await page.goto('/app/ledger/accounts');
    await expect(page.getByText(SENTINEL_ACCOUNT).first()).toBeVisible();

    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();

    await page.goBack();
    await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(SENTINEL_ACCOUNT)).toHaveCount(0);
    await page.goForward();
    await expect(page.getByText(SENTINEL_ACCOUNT)).toHaveCount(0);
  });

  test('a document restored from the back/forward cache after sign-out elsewhere leaves', async ({ browser }) => {
    await signInWithPassword(page);
    await verifyCode(page, await nextCode(state));
    await expect(page).toHaveURL(/\/app$/);
    await page.goto('/app/ledger/accounts');
    await expect(page.getByText(SENTINEL_ACCOUNT).first()).toBeVisible();

    // Leave the financial document by a cross-document navigation, so the
    // browser may keep it in the back/forward cache, then sign out elsewhere.
    await page.evaluate(() => {
      window.addEventListener('pageshow', (event) => {
        (window as unknown as { restoredFromCache?: boolean }).restoredFromCache = event.persisted;
      });
    });
    await page.goto('/api/v1/setup/status');
    // Sign the same session out from a separate browser context holding a
    // copy of the cookies. A second page in this context would share this
    // page's browsing instance and keep it out of the cache.
    const elsewhere = await browser.newContext({ storageState: await context.storageState() });
    const elsewherePage = await elsewhere.newPage();
    await elsewherePage.goto('/app');
    await elsewherePage.getByRole('button', { name: 'Sign out' }).click();
    await expect(elsewherePage.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
    await elsewhere.close();

    await page.bringToFront();
    // A cache restore fires pageshow, not load.
    await page.goBack({ waitUntil: 'commit' });
    const restoredAt = Date.now();
    // A reloaded document has no listener, so only a cache restore says true.
    const restoredFromCache = await page.evaluate(
      () => (window as unknown as { restoredFromCache?: boolean }).restoredFromCache === true
    );
    test.info().annotations.push({ type: 'restored from back/forward cache', description: String(restoredFromCache) });
    const notRestoredReasons = await page.evaluate(() =>
      JSON.stringify((performance.getEntriesByType('navigation')[0] as unknown as { notRestoredReasons?: unknown })?.notRestoredReasons ?? null)
    );
    test.info().annotations.push({ type: 'not restored reasons', description: notRestoredReasons });
    if (restoredFromCache) {
      // The restored shell must not keep showing the account list: it is
      // visible now only until the session re-check notices the sign-out.
      test.info().annotations.push({
        type: 'sentinel visible on restore',
        description: String(await page.getByText(SENTINEL_ACCOUNT).first().isVisible())
      });
    }
    await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible({ timeout: 15_000 });
    test.info().annotations.push({ type: 'ms until sign-in shown', description: String(Date.now() - restoredAt) });
    await expect(page.getByText(SENTINEL_ACCOUNT)).toHaveCount(0);
  });

  test('sensitive API reads are not served from the HTTP cache after sign-out', async () => {
    for (const path of ['/api/v1/accounts', '/api/v1/auth/session', '/api/v1/auth/events']) {
      const cached = await apiStatus(page, path, { cache: 'force-cache' });
      expect(cached.body, path).not.toContain(SENTINEL_ACCOUNT);
      if (path !== '/api/v1/auth/session') expect(cached.status, path).toBe(401);
      expect(cached.cacheControl, path).toBe('no-store');
    }
  });
});

test.describe.serial('restart phases', () => {
  test.skip(phase === 'deploy', 'only after a restart');

  test('the configured second factor after the restart', async ({ page }) => {
    const state = readState();
    if (state.deviceCookie) await page.context().addCookies([state.deviceCookie]);
    await signInWithPassword(page);

    if (phase === 'restart') {
      await verifyCode(page, await nextCode(state));
      await expect(page).toHaveURL(/\/app$/);
      return;
    }

    expect(phase).toBe('key-changed');
    // A correct authenticator code cannot be checked without the key that
    // sealed the secret; it must be refused, not waved through.
    await verifyCode(page, await nextCode(state));
    await expect(page.getByLabel('Authentication code')).toBeVisible();
    await expect(page.getByRole('alert')).toBeVisible();
    test.info().annotations.push({ type: 'refusal shown to the owner', description: await page.getByRole('alert').innerText() });
    await expect(page).not.toHaveURL(/\/app$/);

    // The recovery code is hashed, not sealed, and is the documented way back.
    const recoveryCode = state.recoveryCodes.shift() ?? '';
    writeState(state);
    await verifyCode(page, recoveryCode);
    await expect(page).toHaveURL(/\/app$/);
    await page.goto('/app/ledger/accounts');
    await expect(page.getByText(SENTINEL_ACCOUNT).first()).toBeVisible();
  });
});
