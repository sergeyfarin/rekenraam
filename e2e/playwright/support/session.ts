/**
 * Getting a run to a signed-in `/app` session.
 *
 * The e2e database is reused within a run and reset between runs, so a spec
 * cannot assume which stage the app is at: it may need to bootstrap the owner,
 * sign in an existing owner, or already be signed in.
 */

import { expect, type Page } from '@playwright/test';

const OWNER_USERNAME = 'owner';
const OWNER_PASSWORD = 'test-password';

/** Signs in (bootstrapping the owner and currencies if needed) and lands on `/app`. */
export async function ensureBrowserSession(page: Page) {
  await page.goto('/');

  const freshHeading = page.getByRole('heading', { name: 'Create the owner account' });
  const loginHeading = page.getByRole('heading', { name: 'Sign in to continue' });
  const currencyHeading = page.getByRole('heading', { name: 'Choose default currencies' });
  const overviewHeading = page.getByRole('heading', { name: 'Overview' });

  await expect(freshHeading.or(loginHeading).or(currencyHeading).or(overviewHeading)).toBeVisible({ timeout: 10_000 });

  if (await freshHeading.isVisible()) {
    await page.getByLabel('Username').fill(OWNER_USERNAME);
    await page.getByLabel('Password').fill(OWNER_PASSWORD);
    await page.getByRole('button', { name: 'Create owner' }).click();
  } else if (await loginHeading.isVisible()) {
    await page.getByLabel('Username').fill(OWNER_USERNAME);
    await page.getByLabel('Password').fill(OWNER_PASSWORD);
    await page.getByRole('button', { name: 'Sign in' }).click();
  }

  await completeCurrencySetupIfNeeded(page);

  await expect(page).toHaveURL(/\/app$/);
  await expect(overviewHeading).toBeVisible();
}

/**
 * Clears the default-currency step when it is shown.
 *
 * A freshly bootstrapped owner is sent here before `/app`; an owner that has
 * already chosen currencies goes straight to the overview.
 */
export async function completeCurrencySetupIfNeeded(page: Page) {
  const currencyHeading = page.getByRole('heading', { name: 'Choose default currencies' });
  const overviewHeading = page.getByRole('heading', { name: 'Overview' });

  await expect(currencyHeading.or(overviewHeading)).toBeVisible({ timeout: 10_000 });
  if (await currencyHeading.isVisible()) {
    await page.getByRole('button', { name: /USD/ }).first().click();
    await page.getByRole('button', { name: 'Save currencies' }).click();
  }
}
