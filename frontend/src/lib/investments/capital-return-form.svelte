<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, investmentPositionsQueryOptions, previewCapitalReturn,
    recordCapitalReturn, type CapitalReturnEffect, type CapitalReturnRequest, type GainImpact,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import { parseMoneyMagnitude } from './form-amounts';
  import {
    gainAcknowledgement, gainImpactCurrency, gainImpactRows, hasGainChanges, impactNeedsReview,
    isGainAcknowledgementRefusal
  } from './gain-impact';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { formatExactMoney } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  // Records a return of capital (T-146/T-148): cash on the payment date, and a
  // per-share basis reduction on every lot open on the effective date. The
  // reduction and any unresolved excess are the server's outputs, previewed
  // before recording.
  let { csrfToken, onSaved, onCancel }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const positionsQuery = createQuery(() => investmentPositionsQueryOptions());
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let positionKey = $state('');
  let cashAccountID = $state('');
  let effectiveOn = $state('');
  let paymentOn = $state('');
  let amount = $state('');
  let reference = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let firstInput = $state<HTMLSelectElement | undefined>();
  let didFocus = false;
  let preview = $state<{
    effects: CapitalReturnEffect[];
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: CapitalReturnRequest;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);

  const locale = $derived(getLocale());
  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const holdingIDs = $derived(new Set(accounts.filter((account: AccountResponse) =>
    account.status === 'active' && (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding'))
    .map((account: AccountResponse) => account.id)));
  const cashAccounts = $derived(accounts.filter((account: AccountResponse) =>
    account.account_class === 'asset' && account.status === 'active' && account.allows_postings &&
    account.account_kind !== 'security_holding' && account.account_kind !== 'fund_holding'));
  const positions = $derived((positionsQuery.data?.positions ?? []).filter((position) =>
    coefficientSign(position.quantity_value) > 0 && holdingIDs.has(position.account_id)));
  const position = $derived(positions.find((item) =>
    `${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}` === positionKey));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency) => [currency.id, currency])));
  const currency = $derived(position ? currenciesByID.get(position.cost_commodity_id) : undefined);
  const loading = $derived(accountsQuery.isPending || positionsQuery.isPending ||
    instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || positionsQuery.isError ||
    instrumentsQuery.isError || currenciesQuery.isError);
  const canPreview = $derived(!loading && !loadError && !!position && !!cashAccountID &&
    !!effectiveOn && !!paymentOn && !!amount.trim());
  const modalGainRows = $derived(preview?.gainImpact
    ? gainImpactRows(preview.gainImpact.changes, gainImpactCurrency(currenciesByID), locale) : []);

  $effect(() => {
    if (!loading && !didFocus && firstInput) {
      firstInput.focus();
      didFocus = true;
    }
  });

  function discardPreview() {
    preview = null;
    formError = undefined;
  }

  function money(value: string, scale: number): string {
    return formatExactMoney(value, scale, currency?.standard_scale ?? 2, locale);
  }

  function buildPayload(): CapitalReturnRequest | null {
    if (!position) return null;
    if (effectiveOn > paymentOn) {
      formError = new TranslatedFormError(m.investments_capital_return_dates_error());
      return null;
    }
    const parsed = parseMoneyMagnitude(amount, { maxScale: currency?.standard_scale ?? 2 });
    if (!parsed.ok || parsed.field.value === '0') {
      formError = new TranslatedFormError(m.investments_capital_return_amount_error());
      return null;
    }
    return {
      holding_account_id: position.account_id, commodity_id: position.commodity_id,
      cash_account_id: Number(cashAccountID), currency_id: position.cost_commodity_id,
      effective_on: effectiveOn, payment_on: paymentOn,
      amount_value: parsed.field.value, amount_scale: parsed.field.scale,
      source_evidence: reference.trim() ? { reference: reference.trim() } : undefined,
      memo: memo.trim() || undefined
    };
  }

  async function runPreview(payload: CapitalReturnRequest): Promise<void> {
    const result = await previewCapitalReturn(payload);
    preview = {
      effects: result.effects, impacts: result.impact.affected_checkpoints,
      gainImpact: hasGainChanges(result.impact.gain_impact) ? result.impact.gain_impact : null, payload
    };
  }

  async function handlePreview(event: SubmitEvent) {
    event.preventDefault();
    if (!canPreview) return;
    formError = undefined;
    const payload = buildPayload();
    if (!payload) return;
    pending = true;
    try {
      await runPreview(payload);
    } catch (error) {
      preview = null;
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function commit(override: boolean) {
    if (!preview) return;
    pending = true;
    formError = undefined;
    try {
      const acknowledgement = gainAcknowledgement(preview.gainImpact);
      await recordCapitalReturn({
        ...preview.payload,
        ...(override ? { reconciliation_override: true } : {}),
        ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
      }, csrfToken);
      await invalidateInvestmentReads(queryClient);
      onSaved();
    } catch (error) {
      if (isGainAcknowledgementRefusal(error) && preview) {
        try {
          await runPreview(preview.payload);
          if (preview && impactNeedsReview({ affected_checkpoints: preview.impacts, gain_impact: preview.gainImpact })) {
            reviewModal = { gainRefreshed: !!preview.gainImpact };
          }
        } catch (previewError) {
          preview = null;
          formError = previewError;
        }
      } else {
        formError = error;
      }
    } finally {
      pending = false;
    }
  }

  function handleRecord() {
    if (!preview) return;
    if (impactNeedsReview({ affected_checkpoints: preview.impacts, gain_impact: preview.gainImpact })) {
      reviewModal = { gainRefreshed: false };
      return;
    }
    void commit(false);
  }

  function confirmReview() {
    if (!preview) return;
    const override = preview.impacts.length > 0;
    reviewModal = null;
    void commit(override);
  }
</script>

{#if reviewModal && preview}
  <ReconciliationConfirm impacts={preview.impacts} gainRows={modalGainRows}
    gainRefreshed={reviewModal.gainRefreshed} {pending}
    onCancel={() => (reviewModal = null)} onConfirm={confirmReview} />
{/if}

<form onsubmit={handlePreview} class="space-y-4" aria-busy={pending}>
  <h2 id="capital-return-title" class="text-base font-semibold text-foreground">{m.investments_capital_return_title()}</h2>
  <p class="text-sm text-muted">{m.investments_capital_return_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_transfer_load_error()}</p>
  {:else}
    {#if positions.length === 0}
      <p class="text-sm text-muted" role="status">{m.investments_capital_return_setup_empty()}</p>
    {/if}
    <div>
      <label for="capital-return-position" class="mb-1 block text-sm font-medium text-foreground">{m.investments_capital_return_position()}</label>
      <select id="capital-return-position" bind:this={firstInput} bind:value={positionKey} required onchange={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_internal_select_source()}</option>
        {#each positions as item (`${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}`)}
          {@const account = accounts.find((candidate: AccountResponse) => candidate.id === item.account_id)}
          {@const instrument = (instrumentsQuery.data?.instruments ?? []).find((candidate) => candidate.commodity_id === item.commodity_id)}
          <option value={`${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}`}>
            {account?.name} · {instrument?.display_name ?? instrument?.commodity_code ?? `#${item.commodity_id}`} · {currenciesByID.get(item.cost_commodity_id)?.code ?? `#${item.cost_commodity_id}`}
          </option>
        {/each}
      </select>
    </div>
    <div>
      <label for="capital-return-cash" class="mb-1 block text-sm font-medium text-foreground">{m.investments_capital_return_cash_account()}</label>
      <select id="capital-return-cash" bind:value={cashAccountID} required onchange={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_form_select_account()}</option>
        {#each cashAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <label for="capital-return-effective" class="mb-1 block text-sm font-medium text-foreground">{m.investments_capital_return_effective_date()}</label>
        <input id="capital-return-effective" type="date" bind:value={effectiveOn} required oninput={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
      <div>
        <label for="capital-return-payment" class="mb-1 block text-sm font-medium text-foreground">{m.investments_capital_return_payment_date()}</label>
        <input id="capital-return-payment" type="date" bind:value={paymentOn} required oninput={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
    </div>
    <div>
      <label for="capital-return-amount" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_capital_return_amount()} {currency?.code ?? ''}
      </label>
      <input id="capital-return-amount" type="text" inputmode="decimal" autocomplete="off" bind:value={amount}
        required oninput={discardPreview} aria-describedby="capital-return-amount-help"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
      <p id="capital-return-amount-help" class="mt-1 text-xs text-muted">{m.investments_capital_return_amount_help()}</p>
    </div>
    <div>
      <label for="capital-return-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="capital-return-reference" type="text" bind:value={reference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="capital-return-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="capital-return-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="capital-return-preview-title" aria-live="polite">
        <h3 id="capital-return-preview-title" class="text-sm font-semibold text-foreground">{m.investments_capital_return_preview_title()}</h3>
        <ul class="space-y-1 text-sm text-foreground">
          {#each preview.effects as effect (effect.lot_id)}
            <li class="break-words">{coefficientSign(effect.excess_value) > 0
              ? m.investments_capital_return_effect_excess({ lot: String(effect.lot_id),
                reduction: money(effect.reduction_value, effect.reduction_scale),
                excess: money(effect.excess_value, effect.excess_scale), currency: currency?.code ?? '' })
              : m.investments_capital_return_effect({ lot: String(effect.lot_id),
                reduction: money(effect.reduction_value, effect.reduction_scale), currency: currency?.code ?? '' })}</li>
          {/each}
        </ul>
        {#if preview.effects.some((effect) => coefficientSign(effect.excess_value) > 0)}
          <p class="text-xs text-muted" role="note">{m.investments_capital_return_excess_note()}</p>
        {/if}
      </section>
    {/if}
  {/if}
  <APIFormError error={formError} id="capital-return-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={handleRecord} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_transfer_pending() : m.investments_capital_return_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_transfer_internal_previewing() : m.investments_capital_return_preview()}
      </button>
    {/if}
  </div>
</form>
