import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';

// #269 — account and category management at phone width. The API suites prove
// the rules; this proves an owner can reach and complete the ordinary
// management journeys on a 390 px screen without horizontal scrolling.

async function expectNoHorizontalScroll(page: import('@playwright/test').Page): Promise<void> {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
}

test('[acceptance] accounts can be created, renamed and closed at 390 px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const name = `Mobile savings ${suffix}`;
  const renamed = `Mobile reserve ${suffix}`;
  // Parent options are inside the select's wrapping label. A partial "Name"
  // label lookup also matches that select when an account contains "name".
  // Seed the collision here so this journey never depends on worker history.
  await createCashAccount(page, csrfToken, `Name collision ${suffix}`, currencyID);

  await page.goto('/app/ledger/accounts');
  await page.getByRole('button', { name: 'Add account' }).first().click();
  let form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create account' }) });
  await form.getByLabel('Name', { exact: true }).fill(name);
  await form.getByLabel('Account type').selectOption('savings');
  await form.getByLabel('Currency').selectOption(String(currencyID));
  await expectNoHorizontalScroll(page);
  await form.getByRole('button', { name: 'Create account' }).click();
  await expect(page.getByText(name).first()).toBeVisible();
  await expectNoHorizontalScroll(page);

  await page.getByRole('button', { name: `Edit ${name}` }).click();
  form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save account' }) });
  await form.getByLabel('Name', { exact: true }).fill(renamed);
  await form.getByRole('button', { name: 'Save account' }).click();
  await expect(page.getByText(renamed).first()).toBeVisible();

  const accounts = await apiJSON<{ accounts: Array<{ id: number; name?: string; status: string }> }>(
    page,
    'GET',
    '/api/v1/accounts?include_archived=true'
  );
  const saved = accounts.accounts.find((account) => account.name === renamed);
  expect(saved?.status).toBe('active');
  const versions = await apiJSON<{ accounts: Array<{ name?: string }> }>(
    page,
    'GET',
    `/api/v1/accounts/${saved?.id}/versions`
  );
  expect(versions.accounts.map((version) => version.name)).toEqual([renamed, name]);

  await page.getByRole('button', { name: `Close ${renamed}` }).click();
  const dialog = page.getByRole('dialog');
  if (await dialog.isVisible().catch(() => false)) {
    await dialog.getByRole('button', { name: /close/i }).last().click();
  }
  await expect
    .poll(async () => {
      const current = await apiJSON<{ status: string }>(page, 'GET', `/api/v1/accounts/${saved?.id}`);
      return current.status;
    })
    .toBe('closed');
  await expectNoHorizontalScroll(page);
});

test('[acceptance] categories can be created under a parent and archived at 390 px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await readyForLedger(page);
  const name = `Mobile hobby ${Date.now()}`;

  await page.goto('/app/ledger/categories');
  await page.getByRole('button', { name: 'Add category' }).first().click();
  const form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Create category' }) });
  await form.getByLabel('Name', { exact: true }).fill(name);
  await form.getByLabel('Type').selectOption('expense');
  await form.getByLabel('Parent').selectOption({ label: 'Personal' });
  await expectNoHorizontalScroll(page);
  await form.getByRole('button', { name: 'Create category' }).click();
  await expect(page.getByText(name).first()).toBeVisible();

  const categories = await apiJSON<{
    categories: Array<{ id: number; name?: string; code?: string; parent_category_id?: number; status: string }>;
  }>(page, 'GET', '/api/v1/categories');
  const created = categories.categories.find((category) => category.name === name);
  const personal = categories.categories.find((category) => category.code === 'expense_personal');
  expect(created?.parent_category_id).toBe(personal?.id);

  await page.getByRole('button', { name: `Archive ${name}` }).click();
  const dialog = page.getByRole('dialog');
  if (await dialog.isVisible().catch(() => false)) {
    await dialog.getByRole('button', { name: /archive/i }).last().click();
  }
  await expect
    .poll(async () => {
      const current = await apiJSON<{ status: string }>(page, 'GET', `/api/v1/categories/${created?.id}`);
      return current.status;
    })
    .toBe('archived');
  await expectNoHorizontalScroll(page);
});

// #552. An edit dated before the latest version governs only its past window.
// The editors start from today's values, so they must send just the fields
// the user changed: anything else would copy today's values into the past,
// or assert today's parent as a backdated structural change (refused). There
// is no category version read, so the earlier window's own values are pinned
// by TestCategoryBackdatedPatchKeepsAsOfStructureAndParentValidation.
test('[acceptance] an earlier-dated category rename sends only the name and leaves today\'s version alone at 390 px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { csrfToken } = await readyForLedger(page);
  const suffix = Date.now();
  const create = (name: string, extra: Record<string, unknown> = {}) =>
    apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name, category_type: 'expense', ...extra });
  const oldParent = await create(`Spring parent ${suffix}`);
  const newParent = await create(`Summer parent ${suffix}`);
  const child = await create(`Moved ${suffix}`, { code: `moved_${suffix}`, parent_category_id: oldParent.id });
  await apiJSON(page, 'PATCH', `/api/v1/categories/${child.id}`, csrfToken, {
    parent_category_id: newParent.id,
    effective_from: '2024-06-01'
  });

  await page.goto('/app/ledger/categories');
  await page.getByRole('button', { name: `Edit Moved ${suffix}` }).click();
  const form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save category' }) });
  await form.getByLabel('Name', { exact: true }).fill(`Spring name ${suffix}`);
  await form.getByText('Advanced').click();
  await form.getByLabel('Effective from').fill('2024-03-01');
  await expect(form.getByLabel('Effective from')).toHaveAccessibleDescription(/Only the fields you change are saved/);
  await expectNoHorizontalScroll(page);
  const sent = page.waitForRequest(
    (request) => request.method() === 'PATCH' && request.url().endsWith(`/api/v1/categories/${child.id}`)
  );
  const answered = page.waitForResponse(
    (response) => response.request().method() === 'PATCH' && response.url().endsWith(`/api/v1/categories/${child.id}`)
  );
  await form.getByRole('button', { name: 'Save category' }).click();

  expect((await sent).postDataJSON()).toEqual({ name: `Spring name ${suffix}`, effective_from: '2024-03-01' });
  expect((await answered).status()).toBe(200);
  await expect(form).toBeHidden();
  // Today is still governed by the June version: its name and parent stand.
  const today = await apiJSON<{ name?: string; parent_category_id?: number }>(page, 'GET', `/api/v1/categories/${child.id}`);
  expect(today.name).toBe(`Moved ${suffix}`);
  expect(today.parent_category_id).toBe(newParent.id);
});

test('[acceptance] an earlier-dated institution rename keeps that date\'s kind and the unshown fields at 390 px', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const name = `History bank ${suffix}`;
  const institution = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/institutions', csrfToken, {
    name,
    kind: 'bank',
    address: { city: 'Utrecht' },
    metadata: { tag: 'kept' },
    effective_from: '2024-01-01'
  });
  await apiJSON(page, 'PATCH', `/api/v1/institutions/${institution.id}`, csrfToken, {
    kind: 'brokerage',
    effective_from: '2024-06-01'
  });
  // The accounts tree lists an institution under the accounts that use it.
  await apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `History account ${suffix}`,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    institution_id: institution.id
  });

  await page.goto('/app/ledger/accounts');
  await page.getByRole('button', { name: `Edit ${name}` }).click();
  const form = page.locator('form').filter({ has: page.getByRole('button', { name: 'Save institution' }) });
  await form.getByLabel('Name', { exact: true }).fill(`Spring bank ${suffix}`);
  // Before the first version there is nothing to correct: the refusal says
  // where the record starts, and nothing is written.
  await form.getByLabel('Effective from').fill('2023-12-01');
  await form.getByRole('button', { name: 'Save institution' }).click();
  await expect(form.getByRole('alert')).toContainText('This record starts on');
  await expectNoHorizontalScroll(page);
  await form.getByLabel('Effective from').fill('2024-03-01');
  await expect(form.getByLabel('Effective from')).toHaveAccessibleDescription(/Only the fields you change are saved/);
  const sent = page.waitForRequest(
    (request) => request.method() === 'PATCH' && request.url().endsWith(`/api/v1/institutions/${institution.id}`)
  );
  await form.getByRole('button', { name: 'Save institution' }).click();
  expect((await sent).postDataJSON()).toEqual({ name: `Spring bank ${suffix}`, effective_from: '2024-03-01' });
  await expect(form).toBeHidden();

  type Version = { name: string; kind: string; effective_from: string; address: unknown; metadata: unknown };
  const { institutions: versions } = await apiJSON<{ institutions: Version[] }>(
    page,
    'GET',
    `/api/v1/institutions/${institution.id}/versions`
  );
  const spring = versions.find((version) => version.effective_from === '2024-03-01');
  expect(spring).toMatchObject({ name: `Spring bank ${suffix}`, kind: 'bank', address: { city: 'Utrecht' }, metadata: { tag: 'kept' } });
  const today = await apiJSON<Version>(page, 'GET', `/api/v1/institutions/${institution.id}`);
  expect(today).toMatchObject({ name, kind: 'brokerage', address: { city: 'Utrecht' }, metadata: { tag: 'kept' } });
});
