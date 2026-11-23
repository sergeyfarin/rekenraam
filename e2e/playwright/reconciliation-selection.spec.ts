import { expect, test, type Page } from './support/fixtures';
import { apiJSON } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

async function openReconciliation(page: Page) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const now = new Date();
  const day = todayISO(now);
  const earlierDay = daysFromTodayISO(-1, now);
  const suffix = now.getTime();
  const description = `Statement deposit ${suffix}`;
  const account = await createCashAccount(page, csrfToken, `Selection cash ${suffix}`, currencyID, earlierDay);
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
  async function start(statementDate = day, endingBalance = '50.00') {
    await page.getByLabel('Statement closing date', { exact: true }).fill(statementDate);
    await page.getByLabel('Statement ending balance', { exact: true }).fill(endingBalance);
    const started = page.waitForResponse(`**/api/v1/accounts/${account.id}/reconciliations/start`);
    await page.getByRole('button', { name: 'Start reconciling', exact: true }).click();
    const response = await started;
    expect(response.ok()).toBe(true);
    const { id } = await response.json();
    return `/api/v1/reconciliations/${id}`;
  }
  const posting = page.getByRole('checkbox', { name: description, exact: true });
  const finish = page.getByRole('button', { name: 'Finish reconciliation', exact: true });
  const difference = page.getByText('Difference', { exact: true }).locator('..');
  const checkpoints = async () =>
    (await apiJSON<{ checkpoints: unknown[] }>(page, 'GET', `/api/v1/accounts/${account.id}/reconciliation-checkpoints`)).checkpoints;
  return { start, posting, finish, difference, checkpoints, earlierDay, day, accountName: account.name };
}

async function startBalancedSession(page: Page) {
  const fixture = await openReconciliation(page);
  const sessionPath = await fixture.start();
  await fixture.posting.check();
  await expect(fixture.finish).toBeEnabled();
  await page.getByLabel('Reason', { exact: true }).fill('Checked against the statement');
  return { ...fixture, sessionPath };
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

for (const action of ['Finish', 'Discard'] as const) {
  test(`a delayed ${action} keeps the newer reconciliation and its selection open (#642)`, async ({ page }) => {
    const { start, posting, finish, checkpoints, earlierDay } = await openReconciliation(page);
    // The earlier statement ends before the deposit selected in the newer one.
    const earlierSession = await start(earlierDay, '0.00');
    const reason = page.getByLabel('Reason', { exact: true });
    const discard = page.getByRole('button', { name: 'Discard', exact: true });
    const noCheckpoints = page.getByText('This account has not been reconciled yet.', { exact: true });
    await reason.fill('Verified the earlier statement');

    const actionURL = `**${earlierSession}/${action === 'Finish' ? 'finish' : 'void'}`;
    const arrived = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    await page.route(actionURL, async (route) => {
      arrived.resolve();
      await release.promise;
      await route.continue();
    });

    let newerSession: string;
    try {
      await (action === 'Finish' ? finish : discard).click();
      await arrived.promise;
      await page.getByRole('button', { name: 'Reconcile', exact: true }).click();
      newerSession = await start();
      await posting.check();
      await expect(finish).toBeEnabled();
      await reason.fill('Working on the newer statement');
      await expect(noCheckpoints).toBeVisible();

      const result = page.waitForResponse(actionURL);
      const refreshed = page.waitForResponse((response) =>
        response.request().method() === 'GET' && new URL(response.url()).pathname === newerSession
      );
      release.resolve();
      expect((await result).ok()).toBe(true);
      expect(await (await refreshed).json()).toMatchObject({
        status: 'open', selected_postings: [expect.objectContaining({ posting_id: expect.any(Number) })]
      });
    } finally {
      release.resolve();
      await page.unrouteAll({ behavior: 'wait' });
    }

    expect(await apiJSON(page, 'GET', earlierSession)).toMatchObject({
      status: action === 'Finish' ? 'finished' : 'voided'
    });
    if (action === 'Finish') {
      await expect(page.getByRole('cell', { name: earlierDay, exact: true })).toBeVisible();
      await expect(noCheckpoints).toBeHidden();
      expect(await checkpoints()).toEqual([
        expect.objectContaining({ statement_date: earlierDay, status: 'active' })
      ]);
    } else {
      expect(await checkpoints()).toEqual([]);
    }
    await expect(posting).toBeChecked();
    await expect(reason).toHaveValue('Working on the newer statement');

    await discard.click();
    await expect(page.getByRole('heading', { name: 'Start a reconciliation', exact: true })).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: /^(Finished|Discarded) reconciliation/ })).toHaveCount(0);
    expect(await apiJSON(page, 'GET', newerSession)).toMatchObject({ status: 'voided' });
  });
}

for (const action of ['Finish', 'Discard'] as const) {
  test(`a delayed ${action} reports the earlier statement on the start form (#647)`, async ({ page }) => {
    const { finish, checkpoints, sessionPath, day, earlierDay, accountName } = await startBalancedSession(page);
    const actionURL = `**${sessionPath}/${action === 'Finish' ? 'finish' : 'void'}`;
    const arrived = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    let actionRequests = 0;
    await page.route(actionURL, async (route) => {
      actionRequests++;
      arrived.resolve();
      await release.promise;
      await route.continue();
    });

    try {
      const discard = page.getByRole('button', { name: 'Discard', exact: true });
      await (action === 'Finish' ? finish : discard).click();
      await arrived.promise;
      await page.getByRole('button', { name: 'Reconcile', exact: true }).click();
      const dateInput = page.getByLabel('Statement closing date', { exact: true });
      const balanceInput = page.getByLabel('Statement ending balance', { exact: true });
      await dateInput.fill(earlierDay);
      await balanceInput.fill('17.00');
      await expect(page.getByText('This account has not been reconciled yet.', { exact: true })).toBeVisible();
      release.resolve();

      const outcome = page.getByRole('status').filter({ hasText: action === 'Finish' ? 'Finished reconciliation' : 'Discarded reconciliation' });
      await expect(outcome).toContainText(accountName);
      const formattedDay = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(`${day}T00:00:00Z`));
      await expect(outcome).toContainText(formattedDay);
      await expect(dateInput).toHaveValue(earlierDay);
      await expect(balanceInput).toHaveValue('17.00');
      await expect(balanceInput).toBeFocused();
      expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: action === 'Finish' ? 'finished' : 'voided' });
      if (action === 'Finish') {
        await expect(page.getByRole('cell', { name: day, exact: true })).toBeVisible();
        expect(await checkpoints()).toEqual([expect.objectContaining({ statement_date: day, status: 'active' })]);
      } else {
        expect(await checkpoints()).toEqual([]);
      }
      expect(actionRequests).toBe(1);
    } finally {
      release.resolve();
      await page.unrouteAll({ behavior: 'wait' });
    }
  });
}

for (const action of ['Finish', 'Discard'] as const) {
  test(`a failed ${action} after Back can verify status and return to the saved session (#647)`, async ({ page }) => {
    const { posting, finish, checkpoints, sessionPath, day } = await startBalancedSession(page);
    const discard = page.getByRole('button', { name: 'Discard', exact: true });
    const close = action === 'Finish' ? finish : discard;
    const endpoint = action === 'Finish' ? 'finish' : 'void';
    const arrived = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    const startArrived = Promise.withResolvers<void>();
    const releaseStart = Promise.withResolvers<void>();
    let actionRequests = 0;
    await page.route(`**${sessionPath}/${endpoint}`, async (route) => {
      actionRequests++;
      if (actionRequests > 1) return route.continue();
      arrived.resolve();
      await release.promise;
      await route.abort('failed');
    });

    try {
      await close.click();
      await arrived.promise;
      await page.getByRole('button', { name: 'Reconcile', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Start a reconciliation', exact: true })).toBeVisible();
      await page.route(`**${sessionPath}`, (route) => route.abort('failed'), { times: 1 });
      release.resolve();

      await expect(page.getByRole('alert').filter({ hasText: 'Could not verify the reconciliation result' })).toBeVisible();
      const returnToSession = page.getByRole('button', { name: 'Return to reconciliation', exact: true });
      await expect(returnToSession).toHaveCount(0);
      await page.getByRole('button', { name: 'Check status again', exact: true }).click();
      await expect(page.getByRole('alert').filter({ hasText: 'The reconciliation is still open' })).toBeVisible();
      expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: 'open' });
      expect(await checkpoints()).toEqual([]);
      expect(actionRequests).toBe(1);

      // An in-flight Start owns the next transition. When it fails, Return becomes available again.
      await page.route('**/api/v1/accounts/*/reconciliations/start', async (route) => {
        startArrived.resolve();
        await releaseStart.promise;
        await route.abort('failed');
      }, { times: 1 });
      await page.getByLabel('Statement closing date', { exact: true }).fill(day);
      await page.getByLabel('Statement ending balance', { exact: true }).fill('17.00');
      await page.getByRole('button', { name: 'Start reconciling', exact: true }).click();
      await startArrived.promise;
      await expect(returnToSession).toBeDisabled();
      releaseStart.resolve();

      await returnToSession.click();
      await expect(page.getByRole('heading', { name: 'Clear postings', exact: true })).toBeFocused();
      await expect(posting).toBeChecked();
      await expect(page.getByLabel('Reason', { exact: true })).toHaveValue('Checked against the statement');
      await close.click();
      const destination = action === 'Finish' ? 'Reconciliation complete' : 'Start a reconciliation';
      await expect(page.getByRole('heading', { name: destination, exact: true })).toBeVisible();
      expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: action === 'Finish' ? 'finished' : 'voided' });
      expect(await checkpoints()).toHaveLength(action === 'Finish' ? 1 : 0);
      expect(actionRequests).toBe(2);
    } finally {
      release.resolve();
      releaseStart.resolve();
      await page.unrouteAll({ behavior: 'wait' });
    }
  });
}

test('the start form reports a committed Finish when its response is lost (#647)', async ({ page }) => {
  const { finish, checkpoints, sessionPath, accountName } = await startBalancedSession(page);
  const arrived = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  let finishRequests = 0;
  await page.route(`**${sessionPath}/finish`, async (route) => {
    finishRequests++;
    arrived.resolve();
    await release.promise;
    expect((await route.fetch()).ok()).toBe(true);
    await route.abort('failed');
  });

  try {
    await finish.click();
    await arrived.promise;
    await page.getByRole('button', { name: 'Reconcile', exact: true }).click();
    await expect(page.getByText('This account has not been reconciled yet.', { exact: true })).toBeVisible();
    release.resolve();
    await expect(page.getByRole('status').filter({ hasText: 'Finished reconciliation' })).toContainText(accountName);
    expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: 'finished' });
    expect(await checkpoints()).toHaveLength(1);
    expect(finishRequests).toBe(1);
  } finally {
    release.resolve();
    await page.unrouteAll({ behavior: 'wait' });
  }
});

for (const action of ['Finish', 'Discard'] as const) {
  test(`a lost ${action} response on the session shows the saved result without another ${action} (#644)`, async ({ page }) => {
    const { posting, finish, checkpoints, sessionPath, day } = await startBalancedSession(page);
    const discard = page.getByRole('button', { name: 'Discard', exact: true });
    const readArrived = Promise.withResolvers<void>();
    const releaseRead = Promise.withResolvers<void>();
    let actionRequests = 0;
    await page.route(`**${sessionPath}/${action === 'Finish' ? 'finish' : 'void'}`, async (route) => {
      actionRequests++;
      expect((await route.fetch()).ok()).toBe(true);
      await route.abort('failed');
    });
    await page.route(`**${sessionPath}`, async (route) => {
      readArrived.resolve();
      await releaseRead.promise;
      await route.continue();
    }, { times: 1 });

    try {
      await (action === 'Finish' ? finish : discard).click();
      await readArrived.promise;
      await expect(page.getByRole('status').filter({ hasText: 'Checking the reconciliation result' })).toBeVisible();
      await expect(finish).toBeDisabled();
      await expect(discard).toBeDisabled();
      await expect(posting).toBeDisabled();
      releaseRead.resolve();

      if (action === 'Finish') {
        await expect(page.getByRole('heading', { name: 'Reconciliation complete', exact: true })).toBeVisible();
        await expect(page.getByRole('cell', { name: day, exact: true })).toBeVisible();
      } else {
        await expect(page.getByRole('heading', { name: 'Start a reconciliation', exact: true })).toBeVisible();
      }
    } finally {
      releaseRead.resolve();
      await page.unrouteAll({ behavior: 'wait' });
    }
    expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: action === 'Finish' ? 'finished' : 'voided' });
    expect(await checkpoints()).toHaveLength(action === 'Finish' ? 1 : 0);
    expect(actionRequests).toBe(1);
  });
}

for (const action of ['Finish', 'Discard'] as const) {
  test(`a ${action} that failed before saving keeps the session ready to retry after a failed status read (#644)`, async ({ page }) => {
    const { posting, finish, checkpoints, sessionPath } = await startBalancedSession(page);
    const discard = page.getByRole('button', { name: 'Discard', exact: true });
    const close = action === 'Finish' ? finish : discard;
    const unknown = page.getByRole('alert').filter({ hasText: 'Could not verify the reconciliation result' });
    let actionRequests = 0;
    await page.route(`**${sessionPath}/${action === 'Finish' ? 'finish' : 'void'}`, async (route) => {
      actionRequests++;
      if (actionRequests > 1) return route.continue();
      await route.abort('failed');
    });
    await page.route(`**${sessionPath}`, (route) => route.abort('failed'), { times: 1 });

    try {
      await close.click();
      await expect(unknown).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Clear postings', exact: true })).toBeVisible();
      await expect(finish).toBeDisabled();
      await expect(discard).toBeDisabled();
      await expect(posting).toBeDisabled();

      await page.getByRole('button', { name: 'Check status again', exact: true }).click();
      await expect(unknown).toHaveCount(0);
      await expect(page.locator('#reconcile-action-error')).toBeVisible();
      await expect(posting).toBeChecked();
      await expect(posting).toBeEnabled();
      await expect(page.getByLabel('Reason', { exact: true })).toHaveValue('Checked against the statement');
      await expect(finish).toBeEnabled();
      await expect(discard).toBeEnabled();
      expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: 'open' });
      expect(await checkpoints()).toEqual([]);
      expect(actionRequests).toBe(1);

      await close.click();
      const destination = action === 'Finish' ? 'Reconciliation complete' : 'Start a reconciliation';
      await expect(page.getByRole('heading', { name: destination, exact: true })).toBeVisible();
    } finally {
      await page.unrouteAll({ behavior: 'wait' });
    }
    expect(await apiJSON(page, 'GET', sessionPath)).toMatchObject({ status: action === 'Finish' ? 'finished' : 'voided' });
    expect(await checkpoints()).toHaveLength(action === 'Finish' ? 1 : 0);
    expect(actionRequests).toBe(2);
  });
}
