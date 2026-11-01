import { readFileSync } from 'node:fs';
import { expect, test } from './support/fixtures';
import { todayISO } from './support/dates';
import { createCashAccount, expectSavedTransaction, readyForLedger } from './support/ledger';

const languages = { en: 'English', es: 'Español', fr: 'Français', nl: 'Nederlands', de: 'Deutsch', ru: 'Русский' };

for (const locale of ['en', 'es', 'fr', 'nl', 'de', 'ru'] as const) {
  test(`enters and rehydrates exact amounts in ${locale} on mobile`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const { csrfToken, currencyID } = await readyForLedger(page);
    const account = await createCashAccount(page, csrfToken, `Locale ${locale} ${Date.now()}`, currencyID);
    const messages = JSON.parse(readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8'));
    const description = `Exact locale ${locale} ${Date.now()}`;

    await page.goto('/app/settings/preferences');
    await page.getByRole('button', { name: languages[locale], exact: true }).click();
    await expect(page.getByRole('button', { name: languages[locale], exact: true })).toHaveAttribute('aria-pressed', 'true');
    await page.goto('/app/ledger/transactions');
    await page.getByRole('button', { name: messages.transactions_form_create_title, exact: true }).click();
    let form = page.locator('form').filter({ has: page.getByRole('heading', { name: messages.transactions_form_create_title, exact: true }) });
    await form.getByLabel(messages.transactions_field_date, { exact: true }).fill(todayISO());
    await form.getByLabel(messages.transactions_field_description, { exact: true }).fill(description);
    await form.getByRole('combobox', { name: messages.transactions_field_account, exact: true }).selectOption({ label: account.name });
    await expect(form.getByLabel(messages.transactions_field_amount, { exact: true })).toHaveAttribute('placeholder', locale === 'en' ? '0.00' : '0,00');
    const grouped = new Intl.NumberFormat(locale).format(90071992547409n) + (locale === 'en' ? '.93' : ',93');
    await form.getByLabel(messages.transactions_field_amount, { exact: true }).fill(`-${grouped}`);
    await form.getByPlaceholder(messages.transactions_field_category_placeholder).click();
    await form.locator('ul[role="listbox"] button[role="option"]').first().click();

    const createdResponse = page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/transactions');
    await form.getByRole('button', { name: messages.transactions_form_create, exact: true }).click();
    const created = await createdResponse;
    expect(created.status()).toBe(201);
    const transaction = await created.json();
    const cashPosting = transaction.journal_entries.flatMap((entry: { postings: Array<{ account_id: number; quantity_value: string; quantity_scale: number }> }) => entry.postings).find((posting: { account_id: number }) => posting.account_id === account.id);
    expect(cashPosting).toMatchObject({ quantity_value: '-9007199254740993', quantity_scale: 2 });
    await expectSavedTransaction(page, description);

    // Rehydration and conversion to split entry must use the same locale as
    // parsing, including amounts above JavaScript's safe integer boundary.
    await page.getByRole('complementary').getByRole('button', { name: messages.transactions_panel_edit, exact: true }).click();
    form = page.locator('form').filter({ has: page.getByRole('heading', { name: messages.transactions_form_edit_title, exact: true }) });
    const decimal = locale === 'en' ? '.' : ',';
    await expect(form.getByLabel(messages.transactions_field_amount, { exact: true })).toHaveValue(`-90071992547409${decimal}93`);
    await form.getByRole('button', { name: messages.transactions_tier_switch_split, exact: true }).click();
    await expect(form.getByLabel(messages.transactions_field_posting_amount, { exact: true }).nth(0)).toHaveValue(`-90071992547409${decimal}93`);
    await expect(form.getByLabel(messages.transactions_field_posting_amount, { exact: true }).nth(1)).toHaveValue(`90071992547409${decimal}93`);
    await expect(form.getByText(messages.transactions_split_balance_ok, { exact: true })).toBeVisible();
    await form.getByLabel(messages.transactions_field_description, { exact: true }).fill(`${description} edited`);
    const editedResponse = page.waitForResponse(response => response.request().method() === 'PATCH' && new URL(response.url()).pathname === `/api/v1/transactions/${transaction.id}`);
    await form.getByRole('button', { name: messages.transactions_form_save, exact: true }).click();
    const edited = await editedResponse;
    expect(edited.status()).toBe(200);
    const saved = await edited.json();
    expect(saved.journal_entries[0].postings.map((posting: { quantity_value: string }) => posting.quantity_value)).toEqual(['-9007199254740993', '9007199254740993']);
    await expectSavedTransaction(page, `${description} edited`);
  });
}
