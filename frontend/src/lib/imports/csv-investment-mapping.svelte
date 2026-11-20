<script lang="ts">
  import { accountDisplayName } from '#lib/accounts/account-labels.ts';
  import type { CSVInvestmentMapping } from './csv-profile.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { m } from '#lib/paraglide/messages.js';

  let { value = $bindable<CSVInvestmentMapping | undefined>(), headers, accounts }: {
    value?: CSVInvestmentMapping;
    headers: string[];
    accounts: AccountResponse[];
  } = $props();
  const cash = $derived(accounts.filter(a => a.status === 'active' && a.allows_postings && !a.system_role &&
    ['asset', 'liability'].includes(a.account_class) && !['security_holding', 'fund_holding'].includes(a.account_kind)));
  const income = $derived(accounts.filter(a => a.status === 'active' && a.allows_postings && a.account_class === 'income'));
  const tax = $derived(accounts.filter(a => a.status === 'active' && a.allows_postings && !a.system_role &&
    ['asset', 'liability', 'expense'].includes(a.account_class) && !['security_holding', 'fund_holding'].includes(a.account_kind)));
</script>

<fieldset class="space-y-4 rounded-(--radius-panel) border border-border p-4">
  <legend class="px-1 text-sm font-semibold text-foreground">{m.import_investment_title()}</legend>
  <label class="flex min-h-11 items-center gap-2 text-sm text-foreground">
    <input type="checkbox" checked={value !== undefined} onchange={(e) => {
      value = e.currentTarget.checked ? { kind: 'column', dividend_value: 'DIVIDEND', withholding_value: 'WITHHOLDING', instrument_column: '' } : undefined;
    }} />{m.import_investment_enable()}
  </label>
  {#if value}
    <p class="text-sm text-muted">{m.import_investment_mapping_help()}</p>
    <div class="grid gap-4 sm:grid-cols-2">
      <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_kind()}
        <select bind:value={value.kind} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2">
          <option value="column">{m.import_investment_mixed()}</option>
          <option value="dividend">{m.import_investment_dividend()}</option>
          <option value="withholding">{m.import_investment_tax()}</option>
        </select>
      </label>
      {#if value.kind === 'column'}
        <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_kind_column()}
          <select bind:value={value.kind_column} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2">
            <option value="">{m.import_csv_column_none()}</option>{#each headers as header}<option value={header}>{header}</option>{/each}
          </select>
        </label>
        <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_dividend_value()}
          <input bind:value={value.dividend_value} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2" />
        </label>
        <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_tax_value()}
          <input bind:value={value.withholding_value} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2" />
        </label>
      {/if}
      <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_code_column()}
        <span class="text-xs text-muted">{m.investments_form_optional()}</span>
        <select bind:value={value.instrument_column} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2">
          <option value="">{m.import_csv_column_none()}</option>{#each headers as header}<option value={header}>{header}</option>{/each}
        </select>
      </label>
      {#if value.kind !== 'withholding'}
        <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_gross_column()}
          <select bind:value={value.gross_column} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2">
            <option value="">{m.import_csv_column_none()}</option>{#each headers as header}<option value={header}>{header}</option>{/each}
          </select>
        </label>
        <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_investment_tax_column()}
          <select bind:value={value.withholding_column} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2">
            <option value="">{m.import_investment_no_tax()}</option>{#each headers as header}<option value={header}>{header}</option>{/each}
          </select>
        </label>
      {/if}
      {#each [
        { label: m.investments_form_cash_account(), field: 'cash_account_id' as const, choices: cash },
        { label: m.investments_form_income_account(), field: 'income_account_id' as const, choices: income },
        { label: m.import_investment_tax_account(), field: 'withholding_account_id' as const, choices: tax }
      ] as entry}
        <label class="flex flex-col gap-1 text-sm text-foreground">{entry.label}
          <select value={value[entry.field]} onchange={(e) => { value = { ...value!, [entry.field]: Number(e.currentTarget.value) || undefined }; }} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2">
            <option value="">{m.import_csv_column_none()}</option>
            {#each entry.choices as account}<option value={account.id}>{accountDisplayName(account)}</option>{/each}
          </select>
        </label>
      {/each}
    </div>
  {/if}
</fieldset>
