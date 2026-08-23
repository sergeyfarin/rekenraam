/**
 * The two-factor journey, end to end in a browser.
 *
 * This is the one flow where a backend-only test cannot tell you the feature
 * works: enrollment spans three screens, the secret is shown exactly once, and
 * the recovery codes are the owner's only way back in. The spec acts as the
 * authenticator app (see `support/totp.ts`) so the codes it submits are
 * produced independently of the server that checks them.
 *
 * Serial and self-cleaning: the run shares one database, so MFA must be off
 * again by the end or every later spec's sign-in would meet a challenge.
 */

import { expect, type Page, test } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './support/api';
import { ensureBrowserSession } from './support/session';
import { currentStep, secretFromOTPAuthURI, totpCode } from './support/totp';

const OWNER_USERNAME = 'owner';
const OWNER_PASSWORD = 'test-password';

/** The shared secret, captured from the one screen that ever shows it. */
let sharedSecret = '';
/** The recovery codes, captured from the one screen that ever shows them. */
let recoveryCodes: string[] = [];
/**
 * The highest TOTP step this spec has already spent.
 *
 * The server's replay guard only accepts a step strictly greater than the last
 * one used, and the whole journey runs inside a few seconds — so several
 * sign-ins in a row would otherwise all present the same step and be refused.
 * The accepted window is one step either side of now, so `spent + 1` is always
 * a code the server will take.
 */
let lastSpentStep = 0;

/** A code the replay guard has not seen, for the authenticator's shared secret. */
function nextAuthenticatorCode(): string {
  const step = Math.max(currentStep(), lastSpentStep + 1);
  lastSpentStep = step;

  return totpCode(sharedSecret, step);
}

async function signInWithPassword(page: Page) {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
  await page.getByLabel('Username').fill(OWNER_USERNAME);
  await page.getByLabel('Password').fill(OWNER_PASSWORD);
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test.describe.serial('two-factor authentication journey', () => {
  test.afterAll(async ({ browser }) => {
    // Whatever happened above, the shared database must be left with MFA off.
    const page = await browser.newPage();
    try {
      await ensureBrowserSessionThroughMFA(page);
      const csrfToken = await csrfTokenFor(page);
      const status = await apiJSON<{ status: string }>(page, 'GET', '/api/v1/auth/mfa');
      if (status.status !== 'disabled') {
        await apiJSON(page, 'POST', '/api/v1/auth/mfa/disable', csrfToken, { password: OWNER_PASSWORD }, [200, 204]);
      }
    } finally {
      await page.close();
    }
  });

  test('enrolls and activates an authenticator, and shows recovery codes once', async ({ page }) => {
    await ensureBrowserSession(page);
    await page.goto('/app/settings/security');

    await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible();
    // A missing REKENRAAM_SECRET_KEY would refuse enrollment rather than store
    // the secret in the clear, so assert the server is actually configured.
    await expect(page.getByText(/needs REKENRAAM_SECRET_KEY/)).toHaveCount(0);
    await expect(page.getByText('Off', { exact: true })).toBeVisible();

    await page.getByLabel('Your password').fill(OWNER_PASSWORD);
    await page.getByRole('button', { name: 'Set up' }).click();

    await expect(page.getByRole('heading', { name: 'Add this to your authenticator app' })).toBeVisible();
    const setupLink = await page.getByText(/^otpauth:\/\//).innerText();
    sharedSecret = secretFromOTPAuthURI(setupLink);
    expect(sharedSecret).not.toEqual('');

    // The previous step's code activates. The current step's would be burned by
    // the replay guard, and the sign-in below needs a step the server has not
    // seen yet.
    const activationStep = currentStep() - 1;
    lastSpentStep = activationStep;
    await page.getByLabel('Code from your app').fill(totpCode(sharedSecret, activationStep));
    await page.getByRole('button', { name: 'Turn on' }).click();

    await expect(page.getByRole('heading', { name: 'Recovery codes' })).toBeVisible();
    recoveryCodes = await page.locator('ul.font-mono li').allInnerTexts();
    expect(recoveryCodes.length).toBeGreaterThan(0);
    for (const code of recoveryCodes) {
      expect(code.trim()).not.toEqual('');
    }

    await page.getByRole('button', { name: 'I have saved them' }).click();
    await expect(page.getByText('On', { exact: true })).toBeVisible();
    await expect(page.getByText(/unused recovery codes remain/)).toBeVisible();
  });

  test('challenges a correct password for an authenticator code', async ({ page }) => {
    expect(sharedSecret, 'enrollment must have run first').not.toEqual('');

    // Each test gets a fresh browser context, so this starts signed out.
    await signInWithPassword(page);

    // The password alone must not be a session.
    await expect(page.getByLabel('Authentication code')).toBeVisible();
    await expect(page).not.toHaveURL(/\/app$/);

    await page.getByLabel('Authentication code').fill('000000');
    await page.getByRole('button', { name: 'Verify' }).click();
    await expect(page.getByLabel('Authentication code')).toBeVisible();
    await expect(page).not.toHaveURL(/\/app$/);

    await page.getByLabel('Authentication code').fill(nextAuthenticatorCode());
    await page.getByRole('button', { name: 'Verify' }).click();

    await expect(page).toHaveURL(/\/app$/);
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
  });

  test('accepts a recovery code once and then refuses it', async ({ page }) => {
    expect(recoveryCodes.length, 'enrollment must have run first').toBeGreaterThan(0);
    const recoveryCode = recoveryCodes[0].trim();

    await signInWithPassword(page);
    await page.getByLabel('Authentication code').fill(recoveryCode);
    await page.getByRole('button', { name: 'Verify' }).click();

    await expect(page).toHaveURL(/\/app$/);
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();

    // A recovery code is single-use: spending it again must not get back in.
    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();
    await signInWithPassword(page);
    await page.getByLabel('Authentication code').fill(recoveryCode);
    await page.getByRole('button', { name: 'Verify' }).click();

    await expect(page.getByLabel('Authentication code')).toBeVisible();
    await expect(page).not.toHaveURL(/\/app$/);
  });
});

/** Signs in, answering an MFA challenge with a fresh authenticator code if one appears. */
async function ensureBrowserSessionThroughMFA(page: Page) {
  await signInWithPassword(page);

  const codeField = page.getByLabel('Authentication code');
  const overviewHeading = page.getByRole('heading', { name: 'Overview' });
  await expect(codeField.or(overviewHeading)).toBeVisible({ timeout: 10_000 });
  if (await codeField.isVisible()) {
    await codeField.fill(nextAuthenticatorCode());
    await page.getByRole('button', { name: 'Verify' }).click();
    await expect(overviewHeading).toBeVisible();
  }
}
