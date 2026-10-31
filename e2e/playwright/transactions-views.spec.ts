import { expect, test } from './support/fixtures';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';

/**
 * #446: Transactions has three views. All keeps posted and voided entries,
 * Drafts shows only automation-owned drafts, and Deleted replaces the old
 * Settings → Trash page with the same guarded restore behaviour.
 */
test('[acceptance] Transactions separates All, Drafts and Deleted and restores from Deleted', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 844 });
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();
  const account = await createCashAccount(page, csrfToken, `Views checking ${suffix}`, currencyID);
  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; category_type: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const expense = categories.categories.find(
    (item) => item.category_type === 'expense' && item.allows_postings
  );
  if (!expense) throw new Error('seeded expense category not found');
  const { today } = await apiJSON<{ today: string }>(page, 'GET', '/api/v1/recurring/summary');

  const post = (description: string) =>
    apiJSON<{ id: number }>(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: today,
      status: 'posted',
      description,
      journal_entries: [
        {
          entry_date: today,
          postings: [
            { account_id: account.id, commodity_id: currencyID, quantity_value: '-1200', quantity_scale: 2 },
            { account_id: expense.id, commodity_id: currencyID, quantity_value: '1200', quantity_scale: 2 }
          ]
        }
      ]
    });

  const postedDescription = `Views posted ${suffix}`;
  const voidedDescription = `Views voided ${suffix}`;
  const deletedDescription = `Views deleted ${suffix}`;
  const draftDescription = `Views draft ${suffix}`;

  await post(postedDescription);
  const voided = await post(voidedDescription);
  await apiJSON(page, 'POST', `/api/v1/transactions/${voided.id}/void`, csrfToken, {
    change_reason: 'view test'
  });
  const deleted = await post(deletedDescription);
  await apiJSON(page, 'POST', `/api/v1/transactions/${deleted.id}/soft-delete`, csrfToken, {
    change_reason: 'view test'
  });

  const template = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/recurring/templates', csrfToken, {
    name: `Views template ${suffix}`,
    enabled: true,
    frequency: 'daily',
    starts_on: today,
    lead_days: 0,
    description: draftDescription,
    postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '-1200', quantity_scale: 2 },
      { account_id: expense.id, commodity_id: currencyID, quantity_value: '1200', quantity_scale: 2 }
    ]
  }, [201]);

  try {
    await apiJSON(page, 'POST', `/api/v1/recurring/templates/${template.id}/run-now`, csrfToken, {});
    const due = await apiJSON<{ items: Array<{ template_id: number; transaction_id: number }> }>(
      page,
      'GET',
      '/api/v1/recurring/due'
    );
    const draft = due.items.find((item) => item.template_id === template.id);
    if (!draft) throw new Error('expected the recurring draft in the due inbox');

    const row = (description: string) => page.getByRole('row').filter({ hasText: description });

    await page.goto('/app/ledger/transactions');
    await expect(page.getByTestId('transactions-view-all')).toHaveAttribute('aria-current', 'page');
    await expect(row(postedDescription)).toHaveCount(1);
    await expect(row(voidedDescription)).toHaveCount(1);
    await expect(row(deletedDescription)).toHaveCount(0);
    await expect(row(draftDescription)).toHaveCount(0);
    await expectNoAccessibilityViolations(page, 'transactions All');

    await page.getByTestId('transactions-view-drafts').click();
    await expect(page).toHaveURL('/app/ledger/transactions?view=drafts');
    await expect(page.getByTestId('transactions-view-drafts')).toHaveAttribute('aria-current', 'page');
    await expect(row(draftDescription)).toHaveCount(1);
    await expect(row(postedDescription)).toHaveCount(0);
    await expectNoAccessibilityViolations(page, 'transactions Drafts');

    await page.getByTestId('transactions-view-deleted').click();
    await expect(page).toHaveURL('/app/ledger/transactions?view=deleted');
    await expect(page.getByTestId('transactions-view-deleted')).toHaveAttribute('aria-current', 'page');
    await expect(row(deletedDescription)).toHaveCount(1);
    await expect(row(postedDescription)).toHaveCount(0);
    await expectNoAccessibilityViolations(page, 'transactions Deleted');

    await row(deletedDescription).getByRole('button', { name: 'Restore', exact: true }).click();
    await expect(row(deletedDescription)).toHaveCount(0);

    await page.getByTestId('transactions-view-all').click();
    await expect(page).toHaveURL('/app/ledger/transactions');
    await expect(row(deletedDescription)).toHaveCount(1);
  } finally {
    await apiJSON(page, 'POST', `/api/v1/recurring/templates/${template.id}/archive`, csrfToken, {});
  }
});
