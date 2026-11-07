import { formatEditableAmount, parseDecimalAmount, type ScaledAmount } from '#lib/money/amount.ts';
import type { TradeChargeDraft } from './trade-economics.ts';

/**
 * A trade priced per unit (#508): the owner enters the quantity and the unit
 * price, and the form derives the gross (quantity × unit price) and the cash
 * movement with the trade's own arithmetic. The unit price is an entry aid,
 * never sent or stored: the request still carries the derived total, and,
 * in exact mode, the gross and net the backend checks against each other.
 *
 * The net follows the backend's rule exactly
 * (investment_trade_economics.go): signed gross plus every signed charge in
 * the cash currency that is not paid separately. A buy's gross and charges
 * are money out; a rebate is money back. Nothing is rounded: a product with
 * more decimals than the cash currency has is refused, so the owner enters
 * the total the broker actually settled.
 */
export type PricedTrade =
  | { ok: true; gross: string; net: string }
  | { ok: false; reason: 'invalid' | 'precision' | 'charges' | 'non_positive'; exact?: string };

function magnitude(input: string): ScaledAmount | null {
  const parsed = parseDecimalAmount(input);
  if (!parsed || parsed.value.startsWith('-')) return null;
  return parsed;
}

/** `amount` at `scale` decimals, or null when that would drop a non-zero digit. */
function atScale(amount: ScaledAmount, scale: number): bigint | null {
  const value = BigInt(amount.value);
  if (amount.scale <= scale) return value * 10n ** BigInt(scale - amount.scale);
  const factor = 10n ** BigInt(amount.scale - scale);
  return value % factor === 0n ? value / factor : null;
}

export function pricedTrade(input: {
  side: 'buy' | 'sell';
  quantity: string;
  unitPrice: string;
  /** Exact mode: charges enter the cash movement; otherwise the total is the gross. */
  exactMode: boolean;
  charges: readonly TradeChargeDraft[];
  cashCommodityID: number | undefined;
  /** The cash currency's standard scale. */
  scale: number;
}): PricedTrade {
  const quantity = magnitude(input.quantity);
  const price = magnitude(input.unitPrice);
  if (!quantity || !price || quantity.value === '0' || price.value === '0') return { ok: false, reason: 'invalid' };
  const product: ScaledAmount = {
    value: (BigInt(quantity.value) * BigInt(price.value)).toString(),
    scale: quantity.scale + price.scale
  };
  const gross = atScale(product, input.scale);
  if (gross === null) {
    return { ok: false, reason: 'precision', exact: formatEditableAmount(product.value, product.scale) };
  }
  let net = gross;
  if (input.exactMode) {
    for (const charge of input.charges) {
      const commodityID = Number(charge.commodityID || input.cashCommodityID);
      if (charge.separatePayment || commodityID !== input.cashCommodityID) continue;
      const amount = magnitude(charge.amount);
      const scaled = amount ? atScale(amount, input.scale) : null;
      if (scaled === null) return { ok: false, reason: 'charges' };
      // Buy: fees add to the cash paid, a rebate reduces it. Sell: the reverse.
      const costsOwner = charge.kind !== 'rebate';
      net += (input.side === 'buy') === costsOwner ? scaled : -scaled;
    }
  }
  if (net <= 0n) return { ok: false, reason: 'non_positive' };
  return {
    ok: true,
    gross: formatEditableAmount(gross.toString(), input.scale),
    net: formatEditableAmount(net.toString(), input.scale)
  };
}
