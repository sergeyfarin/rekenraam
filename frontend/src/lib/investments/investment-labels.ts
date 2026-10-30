import type { CostBasisMethod } from '#lib/api/investments.ts';
import { formatQuantity } from '#lib/money/format.ts';
import { m } from '#lib/paraglide/messages.js';

export function formatScaledValue(value: string, scale: number, locale: string): string {
  return formatQuantity(value, scale, locale);
}

export const COST_BASIS_METHODS: readonly CostBasisMethod[] = ['fifo', 'lifo', 'average_cost', 'specific_lot'];

// One localized label per durable cost-basis method code, shared by every
// investment workflow so selects and previews agree in each locale.
export function costBasisMethodLabel(method: CostBasisMethod): string {
  switch (method) {
    case 'fifo':
      return m.transactions_investment_replace_method_fifo();
    case 'lifo':
      return m.transactions_investment_replace_method_lifo();
    case 'average_cost':
      return m.transactions_investment_replace_method_average();
    case 'specific_lot':
      return m.transactions_investment_replace_method_specific();
  }
}
