import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

async function startBalancedSession(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const day = todayISO();
  const suffix = Date.now();
  const description = `Statement deposit ${suffix}`;
  const account = await createCashAccount(page, csrfToken, `Selection cash ${suffix}`, currencyID, day);
  const income = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, {
    name: `Selection income ${suffix}`, category_type: 'income'
  });
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: day, status: 'posted', description,
    journal_entries: [{ entry_date: day, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '5000', quantity_scale: 2 },
      { account_id: income.id, commodity_id: currencyID, quantity_value: '-5000', quantity_scale: 2 }
    ] }]
  });

  await page.goto(`/app/ledger/reconcile?account=${account.id}`);
  await page.getByLabel('Statement closing date', { exact: true }).fill(day);
  await page.getByLabel('Statement ending balance', { exact: true }).fill('50.00');
  const started = page.waitForResponse(`**/api/v1/accounts/${account.id}/reconciliations/start`);
  await page.getByRole('button', { name: 'Start reconciling', exact: true }).click();
  const { id } = await (await started).json();
  const posting = page.getByRole('checkbox', { name: description, exact: true });
  const finish = page.getByRole('button', { name: 'Finish reconciliation', exact: true });
  const difference = page.getByText('Difference', { exact: true }).locator('..');
  await posting.check();
  await expect(finish).toBeEnabled();
  await page.getByLabel('Reason', { exact: true }).fill('Checked against the statement');

  const checkpoints = async () =>
    (await apiJSON<{ checkpoints: unknown[] }>(page, 'GET', `/api/v1/accounts/${account.id}/reconciliation-checkpoints`)).checkpoints;
  return { posting, finish, difference, checkpoints, sessionPath: `/api/v1/reconciliations/${id}` };
}

test('a pending reconciliation selection cannot finish a checkpoint (#633)', async ({ page }) => {
  const { posting, finish, difference, checkpoints, sessionPath } = await startBalancedSession(page);
  const arrived = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const selectionURL = `**${sessionPath}`;
  let finishRequests = 0;
  page.on('request', (request) => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === `${sessionPath}/finish`) finishRequests++;
  });
  await page.route(selectionURL, async (route) => {
    if (route.request().method() !== 'PATCH') return route.fallback();
    arrived.resolve();
    await release.promise;
    await route.continue();
  });

  try {
    await posting.uncheck();
    await arrived.promise;
    await finish.click({ force: true });
    expect(await checkpoints()).toEqual([]);
    expect(finishRequests).toBe(0);
    await expect(finish).toBeDisabled();

    // Exercise the handler guard independently of native button disabling.
    await finish.evaluate((button: HTMLButtonElement) => {
      button.disabled = false;
      button.click();
      button.disabled = true;
    });
    expect(await checkpoints()).toEqual([]);
    expect(finishRequests).toBe(0);
  } finally {
    release.resolve();
    await page.unrouteAll({ behavior: 'wait' });
  }

  await expect(difference).toHaveText('Difference 50.00');
  await expect(posting).not.toBeChecked();
  await expect(finish).toBeDisabled();
  await posting.check();
  await expect(page.getByText('Balanced', { exact: true })).toBeVisible();
  await expect(finish).toBeEnabled();
  await finish.click();
  await expect(page.getByRole('heading', { name: 'Reconciliation complete', exact: true })).toBeVisible();
  expect(await checkpoints()).toHaveLength(1);
});

for (const committed of [false, true]) {
  test(`a failed selection ${committed ? 'response shows the committed' : 'write restores the saved'} reconciliation selection (#633)`, async ({ page }) => {
    const { posting, finish, difference, checkpoints, sessionPath } = await startBalancedSession(page);
    const selectionURL = `**${sessionPath}`;
    const recoveryStarted = Promise.withResolvers<void>();
    const releaseRecovery = Promise.withResolvers<void>();
    await page.route(selectionURL, async (route) => {
      if (route.request().method() === 'PATCH') {
        if (committed) expect((await route.fetch()).ok()).toBe(true);
        await route.abort('failed');
      } else {
        recoveryStarted.resolve();
        await releaseRecovery.promise;
        await route.continue();
      }
    });

    try {
      await posting.uncheck();
      await expect(page.locator('#reconcile-action-error')).toBeVisible();
      await recoveryStarted.promise;
      await expect(finish).toBeDisabled();
      releaseRecovery.resolve();
      await expect(posting).toBeEnabled();
    } finally {
      releaseRecovery.resolve();
      await page.unrouteAll({ behavior: 'wait' });
    }
    const session = await apiJSON<{ status: string; selected_postings: unknown[] }>(page, 'GET', sessionPath);
    expect(session.status).toBe('open');
    expect(session.selected_postings).toHaveLength(committed ? 0 : 1);
    await expect(posting).toBeChecked({ checked: !committed });
    await expect(difference).toHaveText(committed ? 'Difference 50.00' : 'Difference 0.00');
    if (committed) await expect(finish).toBeDisabled();
    else await expect(finish).toBeEnabled();
    expect(await checkpoints()).toEqual([]);
  });
}
