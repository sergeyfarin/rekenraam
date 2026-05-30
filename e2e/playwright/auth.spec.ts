import { expect, test } from '@playwright/test';

test('bootstraps the owner account, signs out, and signs back in', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Create the owner account' })).toBeVisible();

  await page.getByLabel('Username').fill('owner');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Create owner' }).click();

  await expect(page.getByText('Signed in as owner')).toBeVisible();

  await page.getByRole('button', { name: 'Sign out' }).click();

  await expect(page.getByRole('heading', { name: 'Sign in to continue' })).toBeVisible();

  await page.getByLabel('Username').fill('owner');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page.getByText('Signed in as owner')).toBeVisible();
});