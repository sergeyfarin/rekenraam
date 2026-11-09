<script lang="ts">
  import { untrack } from 'svelte';
  import { isValid, parseISO } from 'date-fns';
  import { checkpointOverride, isStaleImpactRefusal, type ReviewedCheckpoints } from '#lib/investments/gain-impact.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import type { InvestmentInstrumentResponse, ReconciliationImpactResponse } from '#lib/api/investments.ts';
  import { importDividendReviewImpact, reviewImportDividend, type ImportDividendReview, type ImportResolution } from '#lib/api/imports.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import InvestmentIncomeFields from './investment-income-fields.svelte';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { parseDecimalAmount } from '#lib/money/amount.ts';
  import { maskFinancialText, formatQuantity } from '#lib/money/format.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';
  let { review, accounts, currencies, instruments, csrfToken, onSaved }: {
    review: ImportDividendReview; accounts: AccountResponse[]; currencies: CurrencyResponse[];
    instruments: InvestmentInstrumentResponse[]; csrfToken: string; onSaved: (id: number, repaired: boolean) => void | Promise<void>;
  } = $props();
  let resolution = $state<ImportResolution>(untrack(() => ({ ...review.resolution })));
  // Bind the token to these editable fields, not a newer list response.
  // Background refetch preserves the form; it must preserve its version too.
  const expectedTransactionVersionID = untrack(() => review.expected_transaction_version_id);
  let reason = $state('');
  let pending = $state(false);
  let error = $state<unknown>();
  let impact = $state.raw<ReconciliationImpactResponse | null>(null);
  let fieldsRevision = $state(0);
  const repaired = $derived(review.state === 'repairable');
  const parsedCash = $derived(parseDecimalAmount(review.source_cash_amount, { locale: 'en' }));
  const cash = $derived(parsedCash ? formatQuantity(parsedCash.value, parsedCash.scale, getLocale()) : maskFinancialText(review.source_cash_amount));
  const sourceDate = $derived(parseISO(review.source_date));
  const date = $derived(isValid(sourceDate) ? new Intl.DateTimeFormat(getLocale(), { dateStyle: 'medium' }).format(sourceDate) : review.source_date);
  const currency = $derived(review.source_currency || currencies.find(c => c.id === resolution.commodity_id)?.code || '—');
  const source = $derived({ cash_valid: !!parsedCash, kind: 'dividend' as const, instrument_code: review.source_instrument, gross_amount: '', withholding_amount: '' });
  function request(reviewed: ReviewedCheckpoints = []) {
    return { resolution, expected_transaction_version_id: expectedTransactionVersionID, reason, reconciliation_override: false, ...checkpointOverride(reviewed) };
  }
  async function save(reviewed: ReviewedCheckpoints = []) {
    pending = true; error = undefined;
    try {
      const transaction = await reviewImportDividend(review, request(reviewed), csrfToken);
      impact = null;
      await onSaved(transaction.id, repaired);
    } catch (err) {
      impact = null;
      // A statement finished since the review: show the current set (#304).
      try {
        const current = isStaleImpactRefusal(err) ? await importDividendReviewImpact(review, request()) : null;
        if (current?.affected_checkpoints.length) impact = current;
        else error = err;
      } catch (previewErr) { error = previewErr; }
    }
    finally { pending = false; }
  }
  async function submit(event: SubmitEvent) {
    event.preventDefault(); pending = true; error = undefined;
    try {
      const preview = await importDividendReviewImpact(review, request());
      if (preview.affected_checkpoints.length) { impact = preview; return; }
      await save();
    } catch (err) { error = err; }
    finally { pending = false; }
  }
</script>

<form onsubmit={submit} aria-busy={pending} class="space-y-4 rounded-(--radius-panel) border border-border bg-surface p-4">
  <h2 class="text-base font-semibold text-foreground">{repaired ? m.import_dividend_repairable() : m.import_dividend_pending()}</h2>
  <p class="text-sm text-muted">{m.import_dividend_source_cash({ date, amount: cash, currency })}</p>
  <button type="button" disabled={pending || !parsedCash} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground disabled:opacity-60" onclick={() => {
    resolution = { ...resolution, gross_amount: review.source_cash_amount, withholding_amount: '0' }; fieldsRevision++;
  }}>{m.import_dividend_declare_no_tax()}</button>
  {#key fieldsRevision}
    <InvestmentIncomeFields rowIndex={review.row_index} {source} {resolution} {accounts} {currencies} {instruments} disabled={pending} fixedDividend fixedSettlement={repaired} sourceInstrumentOption={m.import_dividend_source_resolution()} onchange={(patch) => { resolution = { ...resolution, ...patch }; }} />
  {/key}
  {#if repaired}
    <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_dividend_reason()}
      <input bind:value={reason} required disabled={pending} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2" />
    </label>
  {/if}
  <APIFormError {error} id={`dividend-review-error-${review.row_id}`} />
  <button type="submit" disabled={pending || !csrfToken} class="min-h-11 rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground disabled:opacity-60">
    {repaired ? m.import_dividend_repair() : m.import_dividend_record()}
  </button>
</form>
{#if impact}
  <ReconciliationConfirm impacts={impact.affected_checkpoints} {pending} onCancel={() => { impact = null; }} onConfirm={() => void save(impact?.affected_checkpoints ?? [])} />
{/if}
