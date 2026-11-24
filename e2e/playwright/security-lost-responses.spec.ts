/**
 * One-time secrets whose response is lost after the write committed (#661).
 *
 * Activating two-factor authentication, regenerating recovery codes and
 * creating a personal access token each return a secret exactly once. Each
 * case forwards the write to the real app, drops its successful response and
 * checks the page against the saved state. Disabling MFA and revoking a token
 * return no secret, but a repeat is refused, so they re-read the same way
 * (#662). Every test owns its app and
 * database, so an MFA left on never reaches another spec's sign-in.
 */

import { type Page } from '@playwright/test';
import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { ensureBrowserSession } from './support/session';
import { currentStep, secretFromOTPAuthURI, totpCode } from './support/totp';

const OWNER_PASSWORD = 'test-password';
const ACTIVATE_PATH = '/api/v1/auth/mfa/totp/activate';
const REGENERATE_PATH = '/api/v1/auth/mfa/recovery-codes';
const TOKENS_PATH = '/api/v1/auth/tokens';

type MFAStatus = { status: string; recovery_codes_remaining?: number };

/** Commits the write on the server, then fails it in the browser. */
async function dropCommittedResponse(page: Page, path: string, method = 'POST') {
  // Not `times: 1`: a GET of the same path would use it up.
  let committed = false;
  await page.route(`**${path}`, async (route) => {
    if (committed || route.request().method() !== method) return route.continue();
    const response = await route.fetch();
    expect(response.ok(), `the forwarded ${path} write must commit`).toBe(true);
    committed = true;
    await route.abort('failed');
  });
  return () => committed;
}

async function openSecurity(page: Page) {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible();
}

async function startEnrollment(page: Page): Promise<string> {
  await page.getByLabel('Your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Set up' }).click();
  await expect(page.getByRole('heading', { name: 'Add this to your authenticator app' })).toBeVisible();
  return secretFromOTPAuthURI(await page.getByText(/^otpauth:\/\//).innerText());
}

async function activateThroughAPI(page: Page) {
  const csrfToken = await csrfTokenFor(page);
  const enrollment = await apiJSON<{ secret: string }>(
    page, 'POST', '/api/v1/auth/mfa/totp/enroll', csrfToken, { password: OWNER_PASSWORD }
  );
  await apiJSON(page, 'POST', ACTIVATE_PATH, csrfToken, { code: totpCode(enrollment.secret, currentStep()) });
}

test('a lost activation response shows two-factor as on and leads to new recovery codes', async ({ page }) => {
  await openSecurity(page);
  const secret = await startEnrollment(page);
  const committed = await dropCommittedResponse(page, ACTIVATE_PATH);

  await page.getByLabel('Code from your app').fill(totpCode(secret, currentStep()));
  await page.getByRole('button', { name: 'Turn on' }).click();

  const notice = page.getByRole('alert').filter({ hasText: 'recovery codes could not be shown' });
  await expect(notice).toBeVisible();
  expect(committed()).toBe(true);
  expect((await apiJSON<MFAStatus>(page, 'GET', '/api/v1/auth/mfa')).status).toBe('active');
  // The page agrees with the server: on, no enrollment step, no generic error.
  await expect(page.getByText('On', { exact: true })).toBeVisible();
  await expect(page.getByText('Off', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Add this to your authenticator app' })).toHaveCount(0);
  await expect(page.locator('#security-mfa-error')).toHaveCount(0);

  // The path to codes the owner can see starts in the focused password field.
  const password = page.getByLabel('Your password', { exact: true });
  await expect(password).toBeFocused();
  await password.fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'New recovery codes' }).click();
  await expect(page.getByRole('heading', { name: 'Recovery codes' })).toBeVisible();
  expect(await page.locator('ul.font-mono li').count()).toBeGreaterThan(0);
  await expect(notice).toHaveCount(0);
});

test('a lost activation claims neither on nor off until its status read answers', async ({ page }) => {
  await openSecurity(page);
  const secret = await startEnrollment(page);
  const committed = await dropCommittedResponse(page, ACTIVATE_PATH);
  let release!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  let readArrived!: () => void;
  const arrived = new Promise<void>((resolve) => (readArrived = resolve));
  await page.route('**/api/v1/auth/mfa', async (route) => {
    if (!committed() || route.request().method() !== 'GET') return route.continue();
    readArrived();
    await held;
    await route.continue();
  });

  await page.getByLabel('Code from your app').fill(totpCode(secret, currentStep()));
  await page.getByRole('button', { name: 'Turn on' }).click();
  await arrived;

  // The cached status is from before the write; MFA is already active.
  await expect(page.getByText('Checking status...', { exact: true })).toBeVisible();
  await expect(page.getByText('Off', { exact: true })).toHaveCount(0);
  await expect(page.getByText('On', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Turn on' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Set up' })).toHaveCount(0);

  release();
  await expect(page.getByText('On', { exact: true })).toBeVisible();
  await expect(page.getByText('recovery codes could not be shown')).toBeVisible();
});

test('an activation refused before commit keeps the enrollment step', async ({ page }) => {
  await openSecurity(page);
  await startEnrollment(page);

  await page.getByLabel('Code from your app').fill('000000');
  await page.getByRole('button', { name: 'Turn on' }).click();

  await expect(page.locator('#security-mfa-error')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Add this to your authenticator app' })).toBeVisible();
  await expect(page.getByText('Off', { exact: true })).toBeVisible();
  await expect(page.getByText('recovery codes could not be shown')).toHaveCount(0);
  expect((await apiJSON<MFAStatus>(page, 'GET', '/api/v1/auth/mfa')).status).toBe('pending');
});

test('a lost regeneration response warns that the earlier codes may be gone and offers to regenerate', async ({ page }) => {
  await ensureBrowserSession(page);
  await activateThroughAPI(page);
  await openSecurity(page);
  await expect(page.getByText('On', { exact: true })).toBeVisible();
  const committed = await dropCommittedResponse(page, REGENERATE_PATH);

  await page.getByLabel('Your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'New recovery codes' }).click();

  const notice = page.getByRole('alert').filter({ hasText: 'earlier ones may no longer work' });
  await expect(notice).toBeVisible();
  expect(committed()).toBe(true);
  await expect(page.locator('#security-mfa-error')).toHaveCount(0);

  // A refused retry replaces nothing, so the saved codes are still invalid:
  // the warning stays beside the retry's own error.
  await page.getByLabel('Your password', { exact: true }).fill('wrong-password');
  await page.getByRole('button', { name: 'New recovery codes' }).click();
  await expect(page.locator('#security-mfa-error')).toBeVisible();
  await expect(notice).toBeVisible();
  await expect(page.getByText(/unused recovery codes remain/)).toHaveCount(0);

  // Regenerating again is safe and finally shows a set.
  await page.getByLabel('Your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'New recovery codes' }).click();
  await expect(page.getByRole('heading', { name: 'Recovery codes' })).toBeVisible();
  await expect(notice).toHaveCount(0);
});

test('a lost regeneration whose status read also fails keeps its warning through a status retry', async ({ page }) => {
  await ensureBrowserSession(page);
  await activateThroughAPI(page);
  await openSecurity(page);
  const committed = await dropCommittedResponse(page, REGENERATE_PATH);
  // Status reads fail until the API recovers, so the saved state stays unknown.
  let statusReadsFail = true;
  await page.route('**/api/v1/auth/mfa', (route) =>
    statusReadsFail && committed() && route.request().method() === 'GET' ? route.abort('failed') : route.continue());

  await page.getByLabel('Your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'New recovery codes' }).click();

  await expect(page.locator('#security-mfa-status-error')).toBeVisible({ timeout: 15_000 });
  expect(committed()).toBe(true);
  // The codes may be replaced whatever the status read did.
  const notice = page.getByRole('alert').filter({ hasText: 'earlier ones may no longer work' });
  await expect(notice).toBeVisible();
  // An unknown status shows neither badge.
  await expect(page.getByText('On', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Off', { exact: true })).toHaveCount(0);

  // A status-only retry once the API answers keeps the warning.
  statusReadsFail = false;
  await page.getByRole('button', { name: 'Check again', exact: true }).click();
  await expect(page.getByText('On', { exact: true })).toBeVisible();
  await expect(page.locator('#security-mfa-status-error')).toHaveCount(0);
  await expect(notice).toBeVisible();
  await expect(page.getByText(/unused recovery codes remain/)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'New recovery codes' })).toBeVisible();
});

test('the badge claims nothing before the first status read or during an activation', async ({ page }) => {
  await ensureBrowserSession(page);
  // A fresh page whose first status read is still pending.
  let releaseFirst!: () => void;
  const firstHeld = new Promise<void>((resolve) => (releaseFirst = resolve));
  let firstArrived!: () => void;
  const first = new Promise<void>((resolve) => (firstArrived = resolve));
  await page.route('**/api/v1/auth/mfa', async (route) => {
    firstArrived();
    await firstHeld;
    await route.continue();
  }, { times: 1 });
  await page.goto('/app/settings/security');
  await first;
  await expect(page.getByText('Checking status...', { exact: true })).toBeVisible();
  await expect(page.getByText('Off', { exact: true })).toHaveCount(0);
  releaseFirst();
  await expect(page.getByText('Off', { exact: true })).toBeVisible();

  // An activation that has committed but whose response has not arrived.
  const secret = await startEnrollment(page);
  let releaseWrite!: () => void;
  const writeHeld = new Promise<void>((resolve) => (releaseWrite = resolve));
  let writeCommitted!: () => void;
  const committed = new Promise<void>((resolve) => (writeCommitted = resolve));
  await page.route(`**${ACTIVATE_PATH}`, async (route) => {
    expect((await route.fetch()).ok()).toBe(true);
    writeCommitted();
    await writeHeld;
    await route.abort('failed');
  }, { times: 1 });
  await page.getByLabel('Code from your app').fill(totpCode(secret, currentStep()));
  await page.getByRole('button', { name: 'Turn on' }).click();
  await committed;
  expect((await apiJSON<MFAStatus>(page, 'GET', '/api/v1/auth/mfa')).status).toBe('active');
  await expect(page.getByText('Off', { exact: true })).toHaveCount(0);
  await expect(page.getByText('On', { exact: true })).toHaveCount(0);
  releaseWrite();
  await expect(page.getByText('On', { exact: true })).toBeVisible();
});

test('a regeneration refused before commit shows only its error', async ({ page }) => {
  await ensureBrowserSession(page);
  await activateThroughAPI(page);
  await openSecurity(page);

  await page.getByLabel('Your password', { exact: true }).fill('wrong-password');
  await page.getByRole('button', { name: 'New recovery codes' }).click();

  await expect(page.locator('#security-mfa-error')).toBeVisible();
  await expect(page.getByText('earlier ones may no longer work')).toHaveCount(0);
  await expect(page.getByText(/unused recovery codes remain/)).toBeVisible();
});

test('a lost token-create response lists the unheld token so it can be revoked', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  const name = 'Synthetic lost-response client';
  const committed = await dropCommittedResponse(page, TOKENS_PATH);

  await page.getByLabel('Token name', { exact: true }).fill(name);
  await page.getByLabel('Confirm your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();

  await expect(page.getByText(`A token named “${name}” was created, but it could not be shown to you.`, { exact: false })).toBeVisible();
  expect(committed()).toBe(true);
  await expect(page.getByLabel('Your new token', { exact: true })).toHaveCount(0);
  const row = page.locator('li').filter({ has: page.getByText(name, { exact: true }) });
  await expect(row).toBeVisible();
  await expect(row.getByText('Never shown', { exact: true })).toBeVisible();
  const saved = await apiJSON<{ tokens: Array<{ name: string }> }>(page, 'GET', TOKENS_PATH);
  expect(saved.tokens.filter((token) => token.name === name)).toHaveLength(1);

  await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
  await page.getByRole('dialog', { name: 'Revoke token', exact: true })
    .getByRole('button', { name: 'Revoke token', exact: true }).click();
  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
});

test('a token create refused before commit adds no row and no notice', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  const name = 'Synthetic refused client';

  await page.getByLabel('Token name', { exact: true }).fill(name);
  await page.getByLabel('Confirm your password', { exact: true }).fill('wrong-password');
  await page.getByRole('button', { name: 'Create token', exact: true }).click();

  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByText('could not be shown to you')).toHaveCount(0);
  await expect(page.getByText(name, { exact: true })).toHaveCount(0);
  await expect(page.getByText('No personal access tokens yet.')).toBeVisible();
});

test('a lost disable response shows two-factor as off (#662)', async ({ page }) => {
  await ensureBrowserSession(page);
  await activateThroughAPI(page);
  await openSecurity(page);
  const committed = await dropCommittedResponse(page, '/api/v1/auth/mfa/disable');

  await page.getByLabel('Your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Turn off' }).click();

  await expect(page.getByText('Off', { exact: true })).toBeVisible();
  expect(committed()).toBe(true);
  expect((await apiJSON<MFAStatus>(page, 'GET', '/api/v1/auth/mfa')).status).toBe('disabled');
  await expect(page.getByRole('button', { name: 'Set up' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'New recovery codes' })).toHaveCount(0);
});

test('a lost revoke response shows the token as revoked (#662)', async ({ page }) => {
  await ensureBrowserSession(page);
  const name = 'Synthetic lost-revoke client';
  const { credential } = await apiJSON<{ credential: { id: number } }>(page, 'POST', TOKENS_PATH, await csrfTokenFor(page), {
    name, password: OWNER_PASSWORD, scopes: ['ledger:read'], expires_in_days: 30
  });
  await page.goto('/app/settings/security');
  const committed = await dropCommittedResponse(page, `${TOKENS_PATH}/${credential.id}`, 'DELETE');

  const row = page.locator('li').filter({ has: page.getByText(name, { exact: true }) });
  await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Revoke token', exact: true });
  await dialog.getByRole('button', { name: 'Revoke token', exact: true }).click();

  await expect(dialog).toHaveCount(0);
  expect(committed()).toBe(true);
  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
  await expect(page.getByText('Token revoked.', { exact: true })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
});

test('a token refresh after a lost create cannot be cancelled by Show more', async ({ page }) => {
  test.setTimeout(120_000);
  await ensureBrowserSession(page);
  const csrfToken = await csrfTokenFor(page);
  const create = (name: string) => apiJSON<{ credential: { id: number } }>(page, 'POST', TOKENS_PATH, csrfToken, {
    name, password: OWNER_PASSWORD, scopes: ['ledger:read'], expires_in_days: 30
  });
  // More records than one page (50) while staying under the 50 active tokens.
  const ids: number[] = [];
  for (let i = 0; i < 50; i++) ids.push((await create(`Synthetic filler ${i}`)).credential.id);
  for (const id of ids.slice(0, 2)) await apiJSON(page, 'DELETE', `${TOKENS_PATH}/${id}`, csrfToken, undefined, [200, 204]);
  await create('Synthetic filler 50');

  await page.goto('/app/settings/security');
  const showMore = page.getByRole('button', { name: 'Show more tokens', exact: true });
  await expect(showMore).toBeEnabled();

  const committed = await dropCommittedResponse(page, TOKENS_PATH);
  let release!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  let readArrived!: () => void;
  const arrived = new Promise<void>((resolve) => (readArrived = resolve));
  await page.route(`**${TOKENS_PATH}`, async (route) => {
    if (!committed() || route.request().method() !== 'GET') return route.fallback();
    readArrived();
    await held;
    await route.fallback();
  });

  const name = 'Synthetic paged lost-response client';
  await page.getByLabel('Token name', { exact: true }).fill(name);
  await page.getByLabel('Confirm your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  await arrived;

  // Loading the next page now would cancel the refresh that lists the token.
  await expect(showMore).toBeDisabled();
  release();
  const row = page.locator('li').filter({ has: page.getByText(name, { exact: true }) });
  await expect(row).toBeVisible();
  await expect(showMore).toBeEnabled();
  await showMore.click();
  await expect(page.getByText('Synthetic filler 0', { exact: true })).toBeVisible();
  await expect(row).toBeVisible();
});

test('a lost revoke response scrubs the disclosed token it revoked', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  const name = 'Synthetic disclosed lost-revoke client';
  await page.getByLabel('Token name', { exact: true }).fill(name);
  await page.getByLabel('Confirm your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  const secretField = page.getByLabel('Your new token', { exact: true });
  await expect(secretField).toBeVisible();
  const disclosed = await secretField.inputValue();

  const saved = await apiJSON<{ tokens: Array<{ id: number; name: string }> }>(page, 'GET', TOKENS_PATH);
  const id = saved.tokens.find((token) => token.name === name)!.id;
  const committed = await dropCommittedResponse(page, `${TOKENS_PATH}/${id}`, 'DELETE');
  const row = page.locator('li').filter({ has: page.getByText(name, { exact: true }) });
  await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Revoke token', exact: true });
  await dialog.getByRole('button', { name: 'Revoke token', exact: true }).click();

  await expect(dialog).toHaveCount(0);
  expect(committed()).toBe(true);
  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
  await expect(secretField).toHaveCount(0);
  expect(await page.locator('body').innerText()).not.toContain(disclosed);
});

test('a create that never reached the server does not point at an existing token of the same name', async ({ page }) => {
  await ensureBrowserSession(page);
  const name = 'Synthetic same-name client';
  await apiJSON(page, 'POST', TOKENS_PATH, await csrfTokenFor(page), {
    name, password: OWNER_PASSWORD, scopes: ['ledger:read'], expires_in_days: 30
  });
  await page.goto('/app/settings/security');
  const row = page.locator('li').filter({ has: page.getByText(name, { exact: true }) });
  await expect(row).toBeVisible();
  // The request fails in the browser and nothing is written.
  await page.route(`**${TOKENS_PATH}`, (route) =>
    route.request().method() === 'POST' ? route.abort('failed') : route.continue());

  await page.getByLabel('Token name', { exact: true }).fill(name);
  await page.getByLabel('Confirm your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();

  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Create token', exact: true })).toBeEnabled();
  await expect(page.getByText('could not be shown to you')).toHaveCount(0);
  await expect(page.getByText('may have been created')).toHaveCount(0);
  await expect(row.getByText('Never shown', { exact: true })).toHaveCount(0);
  await expect(row.getByRole('button', { name: 'Revoke token', exact: true })).toBeVisible();
});

test('a lost create whose list refresh fails gives neutral guidance', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  await expect(page.getByText('No personal access tokens yet.')).toBeVisible();
  const committed = await dropCommittedResponse(page, TOKENS_PATH);
  await page.route(`**${TOKENS_PATH}`, (route) =>
    committed() && route.request().method() === 'GET' ? route.abort('failed') : route.fallback());

  await page.getByLabel('Token name', { exact: true }).fill('Synthetic unchecked client');
  await page.getByLabel('Confirm your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();

  await expect(page.getByText('The token may have been created, and the list could not be checked.', { exact: false })).toBeVisible({ timeout: 15_000 });
  expect(committed()).toBe(true);
  await expect(page.getByText('could not be shown to you')).toHaveCount(0);
});

test('revoking another token after a lost response keeps the token on screen', async ({ page }) => {
  await ensureBrowserSession(page);
  const older = 'Synthetic older client';
  const { credential } = await apiJSON<{ credential: { id: number } }>(page, 'POST', TOKENS_PATH, await csrfTokenFor(page), {
    name: older, password: OWNER_PASSWORD, scopes: ['ledger:read'], expires_in_days: 30
  });
  await page.goto('/app/settings/security');
  await page.getByLabel('Token name', { exact: true }).fill('Synthetic newly disclosed client');
  await page.getByLabel('Confirm your password', { exact: true }).fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  const secretField = page.getByLabel('Your new token', { exact: true });
  await expect(secretField).toBeVisible();
  const disclosed = await secretField.inputValue();

  const committed = await dropCommittedResponse(page, `${TOKENS_PATH}/${credential.id}`, 'DELETE');
  const row = page.locator('li').filter({ has: page.getByText(older, { exact: true }) });
  await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
  await page.getByRole('dialog', { name: 'Revoke token', exact: true })
    .getByRole('button', { name: 'Revoke token', exact: true }).click();

  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
  expect(committed()).toBe(true);
  // The only copy of the new, still active token stays until dismissed.
  await expect(secretField).toHaveValue(disclosed);
});
