<script lang="ts">
  import { untrack } from 'svelte';
  import { isValid, parseISO } from 'date-fns';
  import { checkpointOverride, isStaleImpactRefusal, type ReviewedCheckpoints } from '#lib/investments/gain-impact.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import type { InvestmentInstrumentResponse, ReconciliationImpactResponse } from '#lib/api/investments.ts';
  import { importDividendReviewImpact, reviewImportDividend, type ImportDividendReview, type ImportResolution } from '#lib/api/imports.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { APIClientError } from '#lib/api/client.ts';
  import InvestmentIncomeFields from './investment-income-fields.svelte';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { parseDecimalAmount } from '#lib/money/amount.ts';
  import { maskFinancialText, formatQuantity } from '#lib/money/format.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';
  // held: another write on the same batch is in flight (an embedding Import
  // screen's Files write, or another row's save); onPending reports this
  // row's own save so the screen can hold its writes in turn (#567 review).
  let { review, accounts, currencies, instruments, csrfToken, onSaved, held = false, onPending }: {
    review: ImportDividendReview; accounts: AccountResponse[]; currencies: CurrencyResponse[];
    instruments: InvestmentInstrumentResponse[]; csrfToken: string; onSaved: (id: number, repaired: boolean) => void | Promise<void>;
    held?: boolean; onPending?: (pending: boolean) => void;
  } = $props();
  let resolution = $state<ImportResolution>(untrack(() => ({ ...review.resolution })));
  // Accounts the review list filled from the stored dividend default. They
  // stay the default's until the owner picks an account, and the marks go
  // back with preview and save so the write checks the default still applies
  // them (#664 review).
  let incomeFromDefault = $state(untrack(() => review.income_account_from_default ?? false));
  let withholdingFromDefault = $state(untrack(() => review.withholding_account_from_default ?? false));
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
    return {
      resolution, expected_transaction_version_id: expectedTransactionVersionID, reason, reconciliation_override: false,
      income_account_from_default: incomeFromDefault && resolution.category_id ? true : undefined,
      withholding_account_from_default: withholdingFromDefault && resolution.withholding_account_id ? true : undefined,
      ...checkpointOverride(reviewed)
    };
  }
  $effect(() => {
    onPending?.(pending);
    return () => onPending?.(false);
  });
  // The default changed or its account was archived since the list filled
  // it: take the automatic accounts back so the owner chooses, as the dividend
  // form does.
  function takeBackChangedDefault(err: unknown): boolean {
    if (!(err instanceof APIClientError) || err.code !== 'DIVIDEND_DEFAULT_CHANGED') return false;
    const patch: Partial<ImportResolution> = {};
    if (incomeFromDefault) patch.category_id = undefined;
    if (withholdingFromDefault) patch.withholding_account_id = undefined;
    resolution = { ...resolution, ...patch };
    incomeFromDefault = false;
    withholdingFromDefault = false;
    fieldsRevision++;
    return true;
  }
  function choose(patch: Partial<ImportResolution>) {
    if ('category_id' in patch && patch.category_id !== resolution.category_id) incomeFromDefault = false;
    if ('withholding_account_id' in patch && patch.withholding_account_id !== resolution.withholding_account_id) withholdingFromDefault = false;
    resolution = { ...resolution, ...patch };
  }
  async function save(reviewed: ReviewedCheckpoints = []) {
    if (held && !pending) return;
    pending = true; error = undefined;
    try {
      const transaction = await reviewImportDividend(review, request(reviewed), csrfToken);
      impact = null;
      await onSaved(transaction.id, repaired);
    } catch (err) {
      impact = null;
      if (takeBackChangedDefault(err)) { error = err; return; }
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
    event.preventDefault();
    if (pending || held) return;
    pending = true; error = undefined;
    try {
      const preview = await importDividendReviewImpact(review, request());
      if (preview.affected_checkpoints.length) { impact = preview; return; }
      await save();
    } catch (err) { takeBackChangedDefault(err); error = err; }
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
    <InvestmentIncomeFields rowIndex={review.row_index} {source} {resolution} {accounts} {currencies} {instruments} disabled={pending} fixedDividend fixedSettlement={repaired} sourceInstrumentOption={m.import_dividend_source_resolution()} onchange={choose} />
  {/key}
  {#if repaired}
    <label class="flex flex-col gap-1 text-sm text-foreground">{m.import_dividend_reason()}
      <input bind:value={reason} required disabled={pending} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2" />
    </label>
  {/if}
  <APIFormError {error} id={`dividend-review-error-${review.row_id}`} />
  <button type="submit" disabled={pending || held || !csrfToken} class="min-h-11 rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground disabled:opacity-60">
    {repaired ? m.import_dividend_repair() : m.import_dividend_record()}
  </button>
</form>
{#if impact}
  <ReconciliationConfirm impacts={impact.affected_checkpoints} pending={pending || held} onCancel={() => { impact = null; }} onConfirm={() => void save(impact?.affected_checkpoints ?? [])} />
{/if}
