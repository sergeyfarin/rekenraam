<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import { forecastQueryKey } from '$lib/api/forecast';
  import { accountRegisterQueryKey, transactionsQueryKey } from '$lib/api/transactions';
  import {
    investmentGainsQueryKey, investmentInstrumentsQueryOptions, investmentLotsQueryKey,
    investmentPositionsQueryKey, investmentPositionsQueryOptions, previewInvestmentSplit,
    recordInvestmentSplit, type GainImpact, type InvestmentSplitPlan, type InvestmentSplitRequest,
    type ReconciliationImpactResponse
  } from '$lib/api/investments';
  import {
    gainAcknowledgement, gainImpactCurrency, gainImpactRows, hasGainChanges, impactNeedsReview,
    isGainAcknowledgementRefusal
  } from './gain-impact';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { parseSplitRatio } from './split-ratio';
  import { TranslatedFormError } from '$lib/form-errors';
  import { coefficientSign } from '$lib/money/amount';
  import { formatQuantity } from '$lib/money/format';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';

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

  let effectiveOn = $state('');
  let holdingKey = $state('');
  let newUnits = $state('');
  let oldUnits = $state('');
  let sourceReference = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let dateInput = $state<HTMLInputElement | undefined>();
  let didFocus = false;
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the plan they saw.
  let preview = $state<{
    plan: InvestmentSplitPlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: InvestmentSplitRequest;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);

  const locale = $derived(getLocale());
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    account.status === 'active' && account.allows_postings &&
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding')));
  // A split applies to the whole holding of a security in one account,
  // across every basis currency its lots carry.
  const holdings = $derived.by(() => {
    const seen = new Map<string, { accountID: number; commodityID: number }>();
    for (const position of positionsQuery.data?.positions ?? []) {
      if (coefficientSign(position.quantity_value) <= 0) continue;
      if (!accounts.some((account) => account.id === position.account_id)) continue;
      const key = `${position.account_id}:${position.commodity_id}`;
      if (!seen.has(key)) seen.set(key, { accountID: position.account_id, commodityID: position.commodity_id });
    }
    return [...seen.entries()].map(([key, value]) => ({ key, ...value }));
  });
  const selectedHolding = $derived(holdings.find((holding) => holding.key === holdingKey));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency) => [currency.id, currency])));
  const instrumentLabel = (commodityID: number) => {
    const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) => item.commodity_id === commodityID);
    return instrument?.display_name ?? instrument?.commodity_code ?? `#${commodityID}`;
  };
  const loading = $derived(accountsQuery.isPending || positionsQuery.isPending ||
    instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || positionsQuery.isError ||
    instrumentsQuery.isError || currenciesQuery.isError);
  const canPreview = $derived(!loading && !loadError && !!effectiveOn && !!selectedHolding &&
    !!newUnits.trim() && !!oldUnits.trim());
  const modalGainRows = $derived(preview?.gainImpact
    ? gainImpactRows(preview.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
    : []);

  $effect(() => {
    if (!loading && !didFocus && dateInput) {
      dateInput.focus();
      didFocus = true;
    }
  });

  function discardPreview() {
    preview = null;
    formError = undefined;
  }

  async function runPreview(payload: InvestmentSplitRequest): Promise<void> {
    const result = await previewInvestmentSplit(payload);
    preview = {
      plan: result.plan,
      impacts: result.impact.affected_checkpoints,
      gainImpact: hasGainChanges(result.impact.gain_impact) ? result.impact.gain_impact : null,
      payload
    };
  }

  async function handlePreview(event: SubmitEvent) {
    event.preventDefault();
    if (!canPreview || !selectedHolding) return;
    const ratio = parseSplitRatio(newUnits, oldUnits);
    if (!ratio.ok) {
      formError = new TranslatedFormError(m.investments_split_ratio_error());
      return;
    }
    pending = true;
    formError = undefined;
    try {
      await runPreview({
        effective_on: effectiveOn,
        holding_account_id: selectedHolding.accountID,
        commodity_id: selectedHolding.commodityID,
        ratio_numerator: ratio.numerator,
        ratio_denominator: ratio.denominator,
        source_evidence: sourceReference.trim() ? { reference: sourceReference.trim() } : undefined,
        memo: memo.trim() || undefined
      });
    } catch (error) {
      preview = null;
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function record(override: boolean) {
    if (!preview) return;
    const acknowledgement = gainAcknowledgement(preview.gainImpact);
    await recordInvestmentSplit({
      ...preview.payload,
      ...(override ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    }, csrfToken);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: investmentPositionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: investmentLotsQueryKey }),
      queryClient.invalidateQueries({ queryKey: investmentGainsQueryKey }),
      queryClient.invalidateQueries({ queryKey: forecastQueryKey }),
      queryClient.invalidateQueries({ queryKey: transactionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: accountRegisterQueryKey })
    ]);
    onSaved();
  }

  // The server recomputes everything at commit. A changed gain set re-opens
  // the review with the current figures instead of a dead-end error.
  async function commit(override: boolean) {
    if (!preview) return;
    pending = true;
    formError = undefined;
    try {
      await record(override);
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
  <h2 id="split-title" class="text-base font-semibold text-foreground">{m.investments_split_title()}</h2>
  <p class="text-sm text-muted">{m.investments_split_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_split_load_error()}</p>
  {:else}
    {#if holdings.length === 0}
      <p class="text-sm text-muted" role="status">{m.investments_split_empty()}</p>
    {/if}
    <div>
      <label for="split-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_split_holding()}</label>
      <select id="split-holding" bind:value={holdingKey} required onchange={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_split_select_holding()}</option>
        {#each holdings as holding (holding.key)}
          <option value={holding.key}>
            {accounts.find((account) => account.id === holding.accountID)?.name} · {instrumentLabel(holding.commodityID)}
          </option>
        {/each}
      </select>
    </div>
    <div>
      <label for="split-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_split_effective_date()}</label>
      <input id="split-date" type="date" bind:this={dateInput} bind:value={effectiveOn} required oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <fieldset class="space-y-2">
      <legend class="text-sm font-medium text-foreground">{m.investments_split_ratio()}</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="split-new-units" class="mb-1 block text-xs text-muted">{m.investments_split_ratio_new()}</label>
          <input id="split-new-units" type="text" inputmode="numeric" autocomplete="off" bind:value={newUnits}
            required oninput={discardPreview} aria-describedby="split-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
        <div>
          <label for="split-old-units" class="mb-1 block text-xs text-muted">{m.investments_split_ratio_old()}</label>
          <input id="split-old-units" type="text" inputmode="numeric" autocomplete="off" bind:value={oldUnits}
            required oninput={discardPreview} aria-describedby="split-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
      </div>
      <p id="split-ratio-help" class="text-xs text-muted">{m.investments_split_ratio_help()}</p>
    </fieldset>
    <div>
      <label for="split-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="split-reference" type="text" bind:value={sourceReference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="split-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="split-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="split-preview-title" aria-live="polite">
        <h3 id="split-preview-title" class="text-sm font-semibold text-foreground">{m.investments_split_preview_title()}</h3>
        <ul class="space-y-1 text-sm text-foreground">
          {#each preview.plan.effects as effect (effect.lot_id)}
            <li class="font-mono">{m.investments_split_lot_change({
              lot: String(effect.lot_id),
              before: formatQuantity(effect.quantity_before_value, effect.quantity_before_scale, locale),
              after: formatQuantity(effect.quantity_after_value, effect.quantity_after_scale, locale)
            })}</li>
          {/each}
        </ul>
        <p class="text-sm font-medium text-foreground">{m.investments_split_total_change({
          delta: formatQuantity(preview.plan.quantity_delta_value, preview.plan.quantity_delta_scale, locale)
        })}</p>
        <p class="text-xs text-muted">{m.investments_split_basis_kept()}</p>
        {#if preview.plan.replayed}
          <p class="text-xs text-warning" role="note">{m.investments_split_replayed()}</p>
        {/if}
      </section>
    {/if}
  {/if}
  <APIFormError error={formError} id="split-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={handleRecord} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_split_pending() : m.investments_split_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_split_previewing() : m.investments_split_preview()}
      </button>
    {/if}
  </div>
</form>
