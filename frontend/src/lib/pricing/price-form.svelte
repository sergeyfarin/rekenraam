<script lang="ts">
  import { autofocus } from '#lib/components/autofocus.ts';
  import { tick, untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { APIClientError } from '#lib/api/client.ts';
  import { createManualPrice, correctPrice, withdrawPrice, getPriceImpact, invalidatePriceReads, type PriceImpact, type PriceObservation, type PricingWorkspace, type ManualPriceRequest } from '#lib/api/price-management.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { parsePriceAmount } from './form.ts';
  import PriceDetail from './price-detail.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { workspace, target, mode, initialBase, initialQuote, onSaved, onCancel }: {
    workspace: PricingWorkspace; target: PriceObservation | null; mode: 'create' | 'correct' | 'void';
    initialBase: number; initialQuote: number; onSaved: (base: number, quote: number) => void; onCancel: () => void;
  } = $props();
  const queryClient = useQueryClient();
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  // A dialog owns one working copy; later query refreshes must not overwrite it.
  const initial = untrack(() => ({ base: target?.base_commodity_id ?? initialBase,
    quote: target?.quote_commodity_id ?? initialQuote, date: target?.valuation_date ?? workspace.as_of,
    price: target ? formatEditableAmount(target.price_value, target.price_scale) : '',
    quantity: target ? formatEditableAmount(target.base_quantity_value, target.base_quantity_scale) : '1',
    basis: target?.adjustment_basis ?? 'raw' }));
  let base = $state(initial.base);
  let quote = $state(initial.quote);
  let valuationDate = $state(initial.date);
  let price = $state(initial.price);
  let quantity = $state(initial.quantity);
  // Raw prices value holdings; total-return prices serve only an R13
  // benchmark. A correction keeps its target's basis, so it is shown, not sent.
  let basis = $state(initial.basis);
  let reference = $state('');
  let observedAt = $state('');
  let reason = $state('');
  let pending = $state(false);
  let reviewing = $state(false);
  let impact = $state<PriceImpact | null>(null);
  let error = $state<unknown>();
  let reviewButton = $state<HTMLButtonElement>();
  const locale = $derived(getLocale());
  const title = $derived(mode === 'create' ? m.pricing_add() : mode === 'correct' ? m.pricing_correct() : m.pricing_void());
  const parsedPrice = $derived(parsePriceAmount(price));
  const parsedQuantity = $derived(parsePriceAmount(quantity));
  const baseIsCurrency = $derived(workspace.commodities.find(item => item.id === base)?.kind === 'currency');
  const sentBasis = $derived<NonNullable<ManualPriceRequest['adjustment_basis']>>(mode === 'create' && !baseIsCurrency && basis === 'total_return' ? 'total_return' : 'raw');
  const basisLabels = $derived<Record<string, string>>({ raw: m.pricing_basis_raw(), split_adjusted: m.pricing_basis_split(), dividend_adjusted: m.pricing_basis_dividend(), total_return: m.pricing_basis_total(), not_applicable: m.pricing_basis_na() });
  const canReview = $derived(!!reason.trim() && (mode === 'void' || (base > 0 && quote > 0 && base !== quote && !!valuationDate && !!parsedPrice && !!parsedQuantity)));
  function close() { if (pending) return; onCancel(); }
  function cancelReview() { reviewing = false; impact = null; void tick().then(() => reviewButton?.focus()); }
  async function review() {
    if (!canReview || pending) return;
    pending = true; error = undefined;
    try {
      if (target) impact = await getPriceImpact(target.id);
      reviewing = true;
    } catch (failure) { error = failure; }
    finally { pending = false; }
  }
  async function save() {
    if (!reviewing || pending) return;
    pending = true; error = undefined;
    try {
      const session = await sessionQuery.refetch();
      const csrf = session.data?.csrf_token;
      if (!csrf) throw new APIClientError({ status: 403, code: 'CSRF_INVALID' });
      if (mode === 'void' && target && impact) await withdrawPrice(target.id, reason, impact.token, csrf);
      else {
        if (!parsedPrice || !parsedQuantity) throw new TranslatedFormError(m.pricing_invalid_amount());
        const input: ManualPriceRequest = {
          base_commodity_id: base, quote_commodity_id: quote, valuation_date: valuationDate,
          price_value: parsedPrice.value, price_scale: parsedPrice.scale, base_quantity_value: parsedQuantity.value, base_quantity_scale: parsedQuantity.scale,
          adjustment_basis: sentBasis, reason, reference, ...(observedAt.trim() ? { observed_at: observedAt.trim() } : {})
        };
        if (mode === 'correct' && target && impact) {
          const { base_commodity_id: _base, quote_commodity_id: _quote, valuation_date: _date, adjustment_basis: _basis, ...correction } = input;
          await correctPrice(target.id, { ...correction, expected_impact: impact.token }, csrf);
        } else if (mode === 'create') await createManualPrice(input, csrf);
        else throw new TranslatedFormError(m.pricing_stale_review());
      }
      await invalidatePriceReads(queryClient);
      onSaved(base, quote);
    } catch (failure) {
      error = failure; reviewing = false; impact = null;
      void tick().then(() => reviewButton?.focus());
    } finally { pending = false; }
  }
</script>

<ModalDialog labelledby="price-form-title" {pending} onCancel={() => (reviewing ? cancelReview() : close())} class="max-w-2xl p-4 sm:p-6">
  <h2 id="price-form-title" class="text-lg font-semibold">{title}</h2>
  <p class="mt-2 text-sm text-muted">{m.pricing_mutation_copy()}</p>
  {#if mode === 'correct'}<p class="mt-2 text-sm text-muted">{m.pricing_correction_copy()}</p>{/if}
  <APIFormError error={error} id="price-form-error" />
  <form class="mt-4 space-y-4" onsubmit={(event) => { event.preventDefault(); void review(); }}>
    <fieldset disabled={pending || reviewing} class="grid min-w-0 gap-3 sm:grid-cols-2 [&>label]:min-w-0">
      {#if mode !== 'void'}
        <label class="grid gap-1 text-sm" for="price-base">{m.pricing_base()}<select id="price-base" bind:value={base} disabled={mode === 'correct'} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3">
          <option value={0}>{m.pricing_choose()}</option>{#each workspace.commodities as commodity}<option value={commodity.id}>{commodity.code}</option>{/each}
        </select></label>
        <label class="grid gap-1 text-sm" for="price-quote">{m.pricing_quote()}<select id="price-quote" bind:value={quote} disabled={mode === 'correct'} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3">
          <option value={0}>{m.pricing_choose()}</option>{#each workspace.commodities.filter(item => item.kind === 'currency') as commodity}<option value={commodity.id}>{commodity.code}</option>{/each}
        </select></label>
        <label class="grid gap-1 text-sm" for="price-date">{m.pricing_date()}<input id="price-date" type="date" required bind:value={valuationDate} disabled={mode === 'correct'} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm" for="price-amount">{m.pricing_price()}<input id="price-amount" inputmode="decimal" required bind:value={price} aria-describedby="price-amount-hint" class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm" for="price-quantity">{m.pricing_base_quantity()}<input id="price-quantity" inputmode="decimal" required bind:value={quantity} aria-describedby="price-amount-hint" class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <p id="price-amount-hint" class="text-sm text-muted">{m.pricing_amount_hint()}</p>
        {#if mode === 'correct'}
          <p class="grid gap-1 text-sm sm:col-span-2"><span class="text-muted">{m.pricing_adjustment_basis()}</span><span>{basisLabels[basis] ?? basis}</span><span class="text-muted">{m.pricing_basis_kept()}</span></p>
        {:else if base > 0 && !baseIsCurrency}
          <label class="grid gap-1 text-sm sm:col-span-2" for="price-basis">{m.pricing_adjustment_basis()}<select id="price-basis" bind:value={basis} aria-describedby="price-basis-hint" class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3">
            <option value="raw">{m.pricing_basis_raw()}</option><option value="total_return">{m.pricing_basis_total()}</option>
          </select></label>
          <p id="price-basis-hint" class="text-sm text-muted sm:col-span-2">{m.pricing_basis_hint()}</p>
        {/if}
        <label class="grid gap-1 text-sm sm:col-span-2" for="price-reference">{m.pricing_reference()}<input id="price-reference" maxlength="500" bind:value={reference} class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3" /></label>
        <label class="grid gap-1 text-sm sm:col-span-2" for="price-observed">{m.pricing_observed_input()}<input id="price-observed" bind:value={observedAt} placeholder="2026-10-13T12:00:00Z" class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3" /></label>
      {/if}
      <label class="grid gap-1 text-sm sm:col-span-2" for="price-reason">{m.pricing_reason()}<textarea id="price-reason" required maxlength="500" bind:value={reason} class="min-h-20 w-full rounded-(--radius-control) border border-border bg-control p-3"></textarea></label>
    </fieldset>
    {#if !reviewing}<div class="flex flex-wrap gap-3">
      <button bind:this={reviewButton} type="submit" disabled={pending || !canReview} class="min-h-11 rounded-(--radius-control) bg-accent px-4 py-2 font-semibold text-accent-foreground disabled:opacity-50">{pending ? m.pricing_loading() : m.pricing_review()}</button>
      <button type="button" onclick={close} disabled={pending} class="min-h-11 rounded-(--radius-control) border border-border px-4 py-2">{m.pricing_cancel()}</button>
    </div>{/if}
  </form>
  {#if reviewing}
    <section aria-label={m.pricing_review_title()} class="mt-5 space-y-4 rounded-(--radius-panel) border border-border p-3">
      <h3 class="font-semibold">{m.pricing_review_title()}</h3>
      <p class="text-sm text-muted">{m.pricing_mutation_copy()}</p>
      {#if mode !== 'void' && parsedPrice && parsedQuantity}
        <p class="wrap-anywhere font-semibold">{m.pricing_amount_pair({ price: formatQuantity(parsedPrice.value, parsedPrice.scale, locale), quote: workspace.commodities.find(item => item.id === quote)?.code ?? '', base: formatQuantity(parsedQuantity.value, parsedQuantity.scale, locale), instrument: workspace.commodities.find(item => item.id === base)?.code ?? '' })}</p>
        <p>{new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parseISO(valuationDate))}</p>
        {#if (mode === 'correct' ? basis : sentBasis) !== 'raw'}<p>{m.pricing_adjustment_basis()}: {basisLabels[mode === 'correct' ? basis : sentBasis] ?? basis}</p>{/if}
        <p class="text-sm text-muted">{m.pricing_owner_assertion()}</p>
      {/if}
      <p class="break-words text-sm">{reason}</p>
      {#if impact}<p class="font-semibold">{m.pricing_retire_list()}</p><ul class="space-y-3">{#each impact.prices as observation}<li class="min-w-0 border-t border-border pt-3"><PriceDetail price={observation} {workspace} /></li>{/each}</ul>{/if}
      <div class="flex flex-wrap gap-3">
        <button {@attach autofocus} type="button" onclick={save} disabled={pending} class="min-h-11 rounded-(--radius-control) bg-accent px-4 py-2 font-semibold text-accent-foreground disabled:opacity-50">{pending ? m.pricing_saving() : m.pricing_confirm()}</button>
        <button type="button" onclick={cancelReview} disabled={pending} class="min-h-11 rounded-(--radius-control) border border-border px-4 py-2">{m.pricing_back()}</button>
      </div>
    </section>
  {/if}
</ModalDialog>
