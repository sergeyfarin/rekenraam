import { expect, test } from '@playwright/test';

test('shows backend hello message', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Rekenraam' })).toBeVisible();
  await expect(page.getByText('hello from rekenraam backend')).toBeVisible();
});
