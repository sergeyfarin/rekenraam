import { test, expect } from './support/reference-date';
import { readyForLedger, createCashAccount } from './support/ledger';
import { apiJSON } from './support/api';

for (const control of [
  { name: 'east just after local midnight', instant: '2027-01-01T23:01:00Z', zone: 'Europe/Amsterdam', today: '2027-01-02' },
  { name: 'west while UTC is tomorrow', instant: '2027-01-02T03:00:00Z', zone: 'America/Los_Angeles', today: '2027-01-01' }
]) {
  test.describe(control.name, () => {
    test.use({ referenceInstant: control.instant, timezoneId: control.zone, viewport: { width: 390, height: 844 } });
    test('[acceptance] owner-today account is immediately fetched, listed and selectable', async ({ page }) => {
      const { csrfToken, currencyID } = await readyForLedger(page);
      await apiJSON(page, 'PUT', '/api/v1/settings/preferences', csrfToken, { time_zone: control.zone });
      const session = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
      expect(session.owner_today).toBe(control.today);
      await page.clock.setFixedTime(new Date(control.instant));
      const account = await createCashAccount(page, csrfToken, 'Owner today reference', currencyID);
      const fetched = await apiJSON<{ id: number; opened_on: string; effective_from: string }>(page, 'GET', `/api/v1/accounts/${account.id}`);
      expect(fetched).toMatchObject({ id: account.id, opened_on: control.today, effective_from: control.today });
      const listed = await apiJSON<{ accounts: Array<{ id: number }> }>(page, 'GET', '/api/v1/accounts?q=Owner%20today%20reference');
      expect(listed.accounts.map(item => item.id)).toContain(account.id);
      await page.goto('/app/ledger/transactions');
      await page.getByRole('button', { name: 'Add transaction', exact: true }).click();
      const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction', exact: true }) });
      await expect(form.getByLabel('Date', { exact: true })).toHaveValue(control.today);
      const picker = form.getByRole('combobox', { name: 'Account', exact: true });
      await picker.selectOption({ label: account.name });
      await expect(picker).toHaveValue(String(account.id));
    });
  });
}
