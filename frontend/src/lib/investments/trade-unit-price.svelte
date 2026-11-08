<script lang="ts">
  import { m } from '#lib/paraglide/messages.js';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import type { TradeChargeDraft } from './trade-economics.ts';
  import { pricedTrade } from './trade-price.ts';

  // An optional unit price for a trade (#508). With one, the form's total
  // (and, in exact mode, its gross) is derived from quantity × unit price and
  // this trade's own charges, so Buy ↔ Sell can keep the price and recompute
  // the cash with the chosen side's arithmetic. The total field then shows the
  // derived figure read-only; clearing the price hands it back to the owner.
  let {
    unitPrice = $bindable(),
    cashAmount = $bindable(),
    gross = $bindable(),
    idPrefix,
    side,
    quantity,
    exactMode,
    charges,
    cashCommodityID,
    scale,
    currencyCode
  }: {
    unitPrice: string;
    cashAmount: string;
    gross: string;
    idPrefix: string;
    side: 'buy' | 'sell';
    quantity: string;
    exactMode: boolean;
    charges: TradeChargeDraft[];
    cashCommodityID: number | undefined;
    /** The cash currency's standard scale; undefined until a cash account is chosen. */
    scale: number | undefined;
    currencyCode: string;
  } = $props();

  const priced = $derived(unitPrice.trim() === '' || scale === undefined ? null
    : pricedTrade({ side, quantity, unitPrice, exactMode, charges, cashCommodityID, scale }));

  $effect(() => {
    if (unitPrice.trim() === '') return;
    const next = priced?.ok ? priced.net : '';
    if (cashAmount !== next) cashAmount = next;
    if (exactMode && priced?.ok && gross !== priced.gross) gross = priced.gross;
  });

  const message = $derived.by(() => {
    if (unitPrice.trim() === '') return '';
    if (scale === undefined) return m.investments_trade_unit_price_currency();
    if (!priced || priced.ok) return '';
    switch (priced.reason) {
      case 'precision': return m.investments_trade_unit_price_precision({ exact: priced.exact ?? '', currency: currencyCode });
      case 'charges': return m.investments_trade_unit_price_charges();
      case 'non_positive': return m.investments_trade_unit_price_non_positive();
      default: return '';
    }
  });
</script>

<div>
  <label for="{idPrefix}-unit-price" class="mb-1 block text-sm font-medium text-foreground">
    {m.investments_trade_unit_price()}
    {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
    <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
  </label>
  <input id="{idPrefix}-unit-price" type="text" inputmode="decimal" bind:value={unitPrice}
    placeholder={formatEditableAmount('0', scale ?? 2)} aria-describedby="{idPrefix}-unit-price-hint"
    class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted" />
  <p id="{idPrefix}-unit-price-hint" class="mt-1 text-xs text-muted">{m.investments_trade_unit_price_hint()}</p>
  {#if message}<p role="status" class="mt-1 text-sm text-foreground">{message}</p>{/if}
</div>
