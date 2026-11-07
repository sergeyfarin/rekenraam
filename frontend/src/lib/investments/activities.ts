import { m } from '#lib/paraglide/messages.js';

/** New transaction's investment activities (#507), in Activity selector order. */
export type InvestmentActivity = 'write-off' | 'buy' | 'sell' | 'short-open' | 'short-cover' | 'payment-in-lieu' | 'dividend' |
  'withholding' | 'reinvested' | 'external-transfer-in' | 'external-transfer-out' | 'internal-transfer' | 'split' |
  'share-exchange' | 'cash-and-stock-merger' | 'spin-off' | 'capital-return';

export type ActivityGroup = 'trades' | 'income' | 'shares' | 'corporate';

export const investmentActivities: { id: InvestmentActivity; label: () => string; titleID?: string; group: ActivityGroup }[] = [
  { id: 'buy', group: 'trades', label: m.investments_activity_buy, titleID: 'buy-title' },
  { id: 'sell', group: 'trades', label: m.investments_activity_sell, titleID: 'sell-title' },
  { id: 'write-off', group: 'shares', label: m.investments_activity_write_off },
  { id: 'short-open', group: 'trades', label: m.investments_activity_short_open, titleID: 'short-open-title' },
  { id: 'short-cover', group: 'trades', label: m.investments_activity_short_cover, titleID: 'short-cover-title' },
  { id: 'payment-in-lieu', group: 'income', label: m.investments_activity_payment_in_lieu, titleID: 'payment-in-lieu-title' },
  { id: 'dividend', group: 'income', label: m.investments_activity_dividend, titleID: 'dividend-title' },
  { id: 'withholding', group: 'income', label: m.investments_activity_withholding, titleID: 'withholding-title' },
  { id: 'reinvested', group: 'income', label: m.investments_activity_reinvested, titleID: 'reinvested-dividend-title' },
  { id: 'external-transfer-in', group: 'shares', label: m.investments_activity_external_transfer_in, titleID: 'external-transfer-in-title' },
  { id: 'internal-transfer', group: 'shares', label: m.investments_activity_internal_transfer, titleID: 'internal-transfer-title' },
  { id: 'external-transfer-out', group: 'shares', label: m.investments_activity_external_transfer_out, titleID: 'external-transfer-out-title' },
  { id: 'split', group: 'corporate', label: m.investments_activity_split, titleID: 'split-title' },
  { id: 'share-exchange', group: 'corporate', label: m.investments_activity_share_exchange, titleID: 'share-exchange-title' },
  { id: 'cash-and-stock-merger', group: 'corporate', label: m.investments_activity_cash_and_stock_merger, titleID: 'merger-title' },
  { id: 'spin-off', group: 'corporate', label: m.investments_activity_spin_off, titleID: 'spin-off-title' },
  { id: 'capital-return', group: 'corporate', label: m.investments_activity_capital_return, titleID: 'capital-return-title' }
];

export function isInvestmentActivity(value: string): value is InvestmentActivity {
  return investmentActivities.some((activity) => activity.id === value);
}

/**
 * Each activity's main date role (design "Shared context and working
 * copies"): trade date for trades and write-offs, payment date for income,
 * effective date for share movements and corporate actions. It is the one
 * date the entry shares; every other date stays in its activity.
 */
export type MainDateRole = 'trade' | 'payment' | 'effective';

export function mainDateRole(activity: InvestmentActivity): MainDateRole {
  switch (activity) {
    case 'buy': case 'sell': case 'short-open': case 'short-cover': case 'write-off':
      return 'trade';
    case 'payment-in-lieu': case 'dividend': case 'withholding': case 'reinvested':
      return 'payment';
    default:
      return 'effective';
  }
}

/**
 * Activities that read the same quantity and unit price with opposite cash
 * direction (#508): Buy ↔ Sell and Short sale ↔ Short cover. Nothing else
 * carries between activities.
 */
export type TradePair = 'long' | 'short';

export function tradePair(activity: InvestmentActivity): TradePair | undefined {
  if (activity === 'buy' || activity === 'sell') return 'long';
  if (activity === 'short-open' || activity === 'short-cover') return 'short';
  return undefined;
}
