import type { components } from '$lib/api/schema';
import { formatQuantity } from '$lib/money/format';

export type InvestmentPositionResponse = components['schemas']['InvestmentPositionResponse'];
export type InvestmentLotResponse = components['schemas']['InvestmentLotResponse'];
export type InvestmentInstrumentResponse = components['schemas']['InvestmentInstrumentResponse'];

export function formatScaledValue(value: string | number, scale: number, locale: string): string {
  return formatQuantity(String(value), scale, locale);
}

export function lotStatusLabel(status: string): string {
  switch (status) {
    case 'open':
      return 'Open';
    case 'closed':
      return 'Closed';
    default:
      return status;
  }
}

export function costBasisMethodLabel(method: string): string {
  switch (method) {
    case 'fifo':
      return 'FIFO';
    case 'lifo':
      return 'LIFO';
    case 'average_cost':
      return 'Average cost';
    case 'specific_lot':
      return 'Specific lot';
    default:
      return method;
  }
}
