<script lang="ts">
  import { accountDisplayName } from '#lib/accounts/account-labels.ts';
  import { untrack } from 'svelte';
  import type { ImportResolution, NormalizedRow } from '#lib/api/imports.ts';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import type { InvestmentInstrumentResponse } from '#lib/api/investments.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { editableCSVInvestmentAmount, canonicalCSVInvestmentAmount } from './csv-investment.ts';

  let { rowIndex, source, resolution, accounts, currencies, instruments, disabled = false, fixedDividend = false, fixedSettlement = false, sourceInstrumentOption, onchange }: {
    rowIndex: number;
    disabled?: boolean;
    fixedDividend?: boolean;
    fixedSettlement?: boolean;
    sourceInstrumentOption?: string;
    source: NonNullable<NormalizedRow['investment']>;
    resolution: ImportResolution;
    accounts: AccountResponse[]; currencies: CurrencyResponse[];
    instruments: InvestmentInstrumentResponse[];
    onchange: (patch: Partial<ImportResolution>) => void;
  } = $props();
  const initial = untrack(() => ({ source, resolution }));
  let gross = $state(editableCSVInvestmentAmount(initial.resolution.gross_amount ?? initial.source.gross_amount));
  let taxAmount = $state(editableCSVInvestmentAmount(initial.resolution.withholding_amount ?? initial.source.withholding_amount));
  let grossError = $state(false);
  let taxError = $state(false);
  const amountError = $derived(grossError || taxError);
  const kind = $derived(fixedDividend ? 'dividend' : (resolution.investment_kind ?? source.kind));

  const cash = $derived(accounts.filter(a => a.status === 'active' && a.allows_postings && !a.system_role &&
    ['asset', 'liability'].includes(a.account_class) && !['security_holding', 'fund_holding'].includes(a.account_kind)));
  const income = $derived(accounts.filter(a => a.status === 'active' && a.allows_postings && a.account_class === 'income'));
  const taxes = $derived(accounts.filter(a => a.status === 'active' && a.allows_postings && !a.system_role &&
    ['asset', 'liability', 'expense'].includes(a.account_class) && !['security_holding', 'fund_holding'].includes(a.account_kind)));

  function amountChanged(field: 'gross_amount' | 'withholding_amount', value: string) {
    const canonical = canonicalCSVInvestmentAmount(value);
    if (field === 'gross_amount') grossError = canonical === null;
    else taxError = canonical === null;
    onchange({ [field]: canonical ?? '' });
  }
</script>

<fieldset {disabled} class="space-y-3 rounded-(--radius-panel) border border-border p-4">
  <legend class="px-1 text-sm font-semibold text-foreground">{m.import_investment_row({ row: rowIndex + 1 })}</legend>
  <p class="text-sm text-muted">{m.import_investment_source({ code: source.instrument_code })}</p>
  <div class="grid gap-3 sm:grid-cols-2">
    {#if !fixedDividend}
    <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.import_investment_kind()}
      <select value={kind} onchange={(e) => onchange({ investment_kind: e.currentTarget.value as 'dividend' | 'withholding' })} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2">
        <option value="">{m.import_csv_column_none()}</option><option value="dividend">{m.import_investment_dividend()}</option><option value="withholding">{m.import_investment_tax()}</option>
      </select>
    </label>
    {/if}
    <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.investments_form_instrument()}
      {#if !source.instrument_code}<span class="text-xs text-muted">{m.investments_form_optional()}</span>{/if}
      <select value={resolution.instrument_commodity_id || ''} onchange={(e) => onchange({ instrument_commodity_id: Number(e.currentTarget.value) || undefined })} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2">
        <option value="">{sourceInstrumentOption ?? (!source.instrument_code ? m.income_coverage_payer_unassigned() : m.import_csv_column_none())}</option>
        {#each instruments.filter(i => i.status === 'active') as instrument (instrument.commodity_id)}<option value={instrument.commodity_id}>{instrument.commodity_code} · {instrument.display_name}</option>{/each}
      </select>
    </label>
    <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.investments_form_cash_account()}
      <select disabled={fixedSettlement} value={resolution.account_id || ''} onchange={(e) => {
        const id = Number(e.currentTarget.value);
        onchange({ account_id: id || undefined, commodity_id: cash.find(a => a.id === id)?.default_commodity_id ?? undefined });
      }} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2">
        <option value="">{m.import_csv_column_none()}</option>{#each cash as account (account.id)}<option value={account.id}>{accountDisplayName(account)}</option>{/each}
      </select>
    </label>
    <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.import_preview_currency_label()}
      <select disabled={fixedSettlement} value={resolution.commodity_id || ''} onchange={(e) => onchange({ commodity_id: Number(e.currentTarget.value) || undefined })} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2">
        <option value="">{m.import_csv_column_none()}</option>{#each currencies as currency (currency.id)}<option value={currency.id}>{currency.code}</option>{/each}
      </select>
    </label>
    {#if kind !== 'withholding'}
      <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.investments_form_income_account()}
        <select value={resolution.category_id || ''} onchange={(e) => onchange({ category_id: Number(e.currentTarget.value) || undefined })} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2">
          <option value="">{m.import_csv_column_none()}</option>{#each income as account (account.id)}<option value={account.id}>{accountDisplayName(account)}</option>{/each}
        </select>
      </label>
      <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.investments_form_dividend_amount()}
        <input inputmode="decimal" bind:value={gross} oninput={(e) => amountChanged('gross_amount', e.currentTarget.value)} aria-invalid={grossError} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2" />
      </label>
      <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.investments_form_withholding_amount()}
        <input inputmode="decimal" bind:value={taxAmount} oninput={(e) => amountChanged('withholding_amount', e.currentTarget.value)} aria-invalid={taxError} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2" />
      </label>
    {/if}
    <label class="flex min-w-0 flex-col gap-1 text-sm text-foreground">{m.import_investment_tax_account()}
      <select value={resolution.withholding_account_id || ''} onchange={(e) => onchange({ withholding_account_id: Number(e.currentTarget.value) || undefined })} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2">
        <option value="">{m.import_csv_column_none()}</option>{#each taxes as account (account.id)}<option value={account.id}>{accountDisplayName(account)}</option>{/each}
      </select>
    </label>
  </div>
  {#if amountError}<p role="alert" class="text-sm text-warning">{m.investments_form_invalid_number()}</p>{/if}
  {#if !source.cash_valid}<p class="text-sm text-warning">{m.import_investment_cash_invalid()}</p>{/if}
</fieldset>
