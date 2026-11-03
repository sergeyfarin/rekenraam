import { formatExactMoney, joinCommodityAmount } from '#lib/money/format.ts';
import type { IncomeCoverageItem } from '#lib/api/investments.ts';

export type CoverageAmount = { value: string; scale: number };
export type CoverageCurrency = { id: number; standard_scale: number };

/** Exact coverage money: known figures are never rounded away (#244). */
export function coverageMoney(amount: CoverageAmount, currencyID: number, code: string, currencies: CoverageCurrency[], locale: string): string {
  const standardScale = currencies.find((currency) => currency.id === currencyID)?.standard_scale ?? Math.min(amount.scale, 2);
  return joinCommodityAmount(code, formatExactMoney(amount.value, amount.scale, standardScale, locale));
}

/** Where an owner repairs an item: the import review keeps source evidence and
 * repairs the same payment; a named dividend is corrected from its detail. */
export function coverageRepairHref(item: Pick<IncomeCoverageItem, 'kind' | 'transaction_id' | 'source'>): string {
  if (item.kind === 'imported_cash' && item.source) return `/app/ledger/import/dividends?batch_id=${item.source.batch_id}`;
  return `/app/ledger/transactions?transaction_id=${item.transaction_id}`;
}
