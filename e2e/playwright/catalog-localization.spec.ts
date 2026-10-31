import { expect, test } from './support/fixtures';
import { readFileSync } from 'node:fs';
import { readyForLedger } from './support/ledger';
import { apiJSON, csrfTokenFor } from './support/api';
import { secretFromOTPAuthURI } from './support/totp';
import { activateBrowserMFA } from './support/mfa-enrollment';

test.use({ viewport: { width: 390, height: 844 } });
const messages = (domain: string, locale: string) => JSON.parse(readFileSync(new URL(`../../frontend/messages/${domain}/${locale}.json`, import.meta.url), 'utf8'));

for (const locale of ['es', 'fr', 'nl', 'de', 'ru']) {
  test(`[acceptance] CSV mapping creation, reuse, editing and deletion render in ${locale} at 390px`, async ({ page }) => {
    await readyForLedger(page);
    const m = messages('app', locale);
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/ledger/import');
    const suffix = Date.now();
    const column = (name: string) => `${name}_${locale}_${suffix}`;
    const fixture = {
      name: `localized-${locale}-${suffix}.csv`, mimeType: 'text/csv',
      buffer: Buffer.from(`${column('Date')};${column('Description')};${column('Amount')}\n2020-01-31;Synthetic merchant;-12.34\n`)
    };
    await page.locator('input[type="file"]').setInputFiles(fixture);
    const mapping = page.locator('fieldset').filter({ has: page.getByText(m.import_csv_mapping_title, { exact: true }) });
    await expect(mapping.getByText(m.import_csv_mapping_copy)).toBeVisible();
    const profileName = `Localization ${locale} ${suffix}`;
    await mapping.getByLabel(m.import_csv_profile_name, { exact: true }).fill(profileName);
    await mapping.getByRole('combobox', { name: m.import_csv_date_column, exact: true }).selectOption(column('Date'));
    await mapping.getByRole('combobox', { name: m.import_csv_payee_column, exact: true }).selectOption(column('Description'));
    await mapping.getByRole('combobox', { name: m.import_csv_amount_column, exact: true }).selectOption(column('Amount'));
    await mapping.getByRole('combobox', { name: m.import_csv_date_layout, exact: true }).selectOption('YMD');
    await mapping.getByRole('combobox', { name: m.import_csv_decimal_separator, exact: true }).selectOption('.');
    await mapping.getByRole('combobox', { name: m.import_csv_amount_layout, exact: true }).selectOption('debit_credit');
    await expect(mapping.getByRole('combobox', { name: m.import_csv_debit_column, exact: true })).toBeVisible();
    await expect(mapping.getByRole('combobox', { name: m.import_csv_credit_column, exact: true })).toBeVisible();
    await mapping.getByRole('combobox', { name: m.import_csv_amount_layout, exact: true }).selectOption('single');
    const upload = page.getByRole('button', { name: m.import_upload_submit, exact: true });
    await upload.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('button', { name: m.import_commit_button, exact: true })).toBeVisible();

    // Reopen the file through the real saved profile; no ledger commit is needed.
    await page.goto('/app/ledger/import');
    await page.locator('input[type="file"]').setInputFiles(fixture);
    const saved = page.getByRole('combobox', { name: m.import_csv_saved_profile, exact: true });
    await expect.poll(() => saved.evaluate((select: HTMLSelectElement) => select.selectedOptions[0]?.textContent?.trim())).toBe(m.import_csv_profile_suggested.replace('{name}', profileName));
    await page.getByRole('button', { name: m.import_csv_profile_edit, exact: true }).click();
    await page.getByLabel(m.import_csv_profile_name, { exact: true }).fill(`${profileName} updated`);
    await page.getByRole('button', { name: m.import_csv_profile_save, exact: true }).click();
    await expect.poll(() => saved.evaluate((select: HTMLSelectElement) => select.selectedOptions[0]?.textContent?.trim())).toBe(m.import_csv_profile_suggested.replace('{name}', `${profileName} updated`));
    await page.getByRole('button', { name: m.import_csv_profile_delete, exact: true }).click();
    await expect(page.getByText(m.import_csv_profile_delete_confirm, { exact: true })).toBeVisible();
    await page.getByRole('button', { name: m.import_csv_profile_delete_cancel, exact: true }).click();
    await page.getByRole('button', { name: m.import_csv_profile_delete, exact: true }).click();
    await page.getByRole('button', { name: m.import_csv_profile_delete_confirm_button, exact: true }).click();
    await expect(saved).toHaveValue('');
  });

  test(`[acceptance] MFA enrollment, recovery and sign-in render in ${locale} at 390px`, async ({ page, browser, baseURL }) => {
    await readyForLedger(page);
    const m = messages('settings', locale);
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/settings/security');
    try {
      await expect(page.getByRole('heading', { name: m.security_mfa_title, exact: true })).toBeVisible();
      await expect(page.getByText(m.security_mfa_description, { exact: true })).toBeVisible();
      await page.getByLabel(m.security_mfa_password_label, { exact: true }).fill('test-password');
      await page.getByRole('button', { name: m.security_mfa_enroll, exact: true }).click();
      await expect(page.getByRole('heading', { name: m.security_mfa_enroll_step_title, exact: true })).toBeVisible();
      await expect(page.getByText(m.security_mfa_enroll_step_hint, { exact: true })).toBeVisible();
      const secret = secretFromOTPAuthURI(await page.getByText(/^otpauth:\/\//).innerText());
      // Sign-in uses a recovery code, so no TOTP step needs reserving.
      await activateBrowserMFA(page, secret, {
        code: page.getByLabel(m.security_mfa_code_label, { exact: true }),
        submit: page.getByRole('button', { name: m.security_mfa_activate, exact: true })
      });
      await expect(page.getByRole('heading', { name: m.security_mfa_recovery_title, exact: true })).toBeVisible();
      await expect(page.getByText(m.security_mfa_recovery_warning, { exact: true })).toBeVisible();
      const codes = await page.locator('ul.font-mono li').allInnerTexts();
      expect(codes).toHaveLength(10);
      await page.getByRole('button', { name: m.security_mfa_recovery_dismiss, exact: true }).click();
      await expect(page.getByText(m.security_mfa_codes_remaining.replace('{count}', '10'), { exact: true })).toBeVisible();

      // Keep the original owner session available for cleanup if sign-in fails.
      const loginContext = await browser.newContext({ baseURL, viewport: { width: 390, height: 844 } });
      try {
        const login = await loginContext.newPage();
        await login.goto('/');
        await login.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
        await login.reload();
        await login.getByLabel(m.install_gate_username_label, { exact: true }).fill('owner');
        await login.getByLabel(m.install_gate_password_label, { exact: true }).fill('test-password');
        await login.getByRole('button', { name: m.install_gate_login_submit, exact: true }).click();
        await expect(login.getByText(m.install_gate_mfa_hint, { exact: true })).toBeVisible();
        await expect(login.getByRole('button', { name: m.install_gate_mfa_cancel, exact: true })).toBeVisible();
        await login.getByLabel(m.install_gate_mfa_code_label, { exact: true }).fill(codes[0].trim());
        await login.getByRole('button', { name: m.install_gate_mfa_submit, exact: true }).click();
        await expect(login).toHaveURL(/\/app$/);
      } finally {
        await loginContext.close();
      }
      await page.reload();
      await expect(page.getByText(m.security_mfa_codes_remaining.replace('{count}', '9'), { exact: true })).toBeVisible();
      await page.getByLabel(m.security_mfa_password_label, { exact: true }).fill('test-password');
      await page.getByRole('button', { name: m.security_mfa_disable, exact: true }).click();
      await expect(page.getByText(m.security_mfa_badge_inactive, { exact: true })).toBeVisible();
      await page.goto('/app/settings/preferences');
      await expect(page.getByText(messages('app', locale).language_translation_progress_copy, { exact: true })).toBeVisible();
    } finally {
      const status = await apiJSON<{ status: string }>(page, 'GET', '/api/v1/auth/mfa');
      if (status.status !== 'disabled') {
        await apiJSON(page, 'POST', '/api/v1/auth/mfa/disable', await csrfTokenFor(page), { password: 'test-password' }, [200, 204]);
      }
    }
  });
}
