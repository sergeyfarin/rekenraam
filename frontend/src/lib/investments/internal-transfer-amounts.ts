import type { InvestmentLotResponse, InternalTransferRequest } from '$lib/api/investments';
import { compareScaledAmounts } from '$lib/money/amount';
import { parseMagnitude } from '$lib/investments/form-amounts';

export type InternalTransferDraft = { lotID: number; quantity: string };

/** Validate selected source-lot amounts without rounding or floating point. */
export function parseInternalTransferAllocations(
  drafts: InternalTransferDraft[],
  availableLots: InvestmentLotResponse[],
  maxScale: number
): { ok: true; allocations: InternalTransferRequest['lot_allocations'] } |
   { ok: false; reason: 'empty' | 'invalid' | 'exceeds_available'; lotID?: number } {
  if (drafts.length === 0) return { ok: false, reason: 'empty' };
  const available = new Map(availableLots.filter((lot) => lot.status === 'open').map((lot) => [lot.id, lot]));
  const seen = new Set<number>();
  const allocations: InternalTransferRequest['lot_allocations'] = [];
  for (const draft of drafts) {
    const lot = available.get(draft.lotID);
    const parsed = parseMagnitude(draft.quantity, { maxScale });
    if (!lot || seen.has(draft.lotID) || !parsed.ok || parsed.field.value === '0') {
      return { ok: false, reason: 'invalid', lotID: draft.lotID };
    }
    if (compareScaledAmounts(parsed.field, {
      value: lot.remaining_quantity_value, scale: lot.remaining_quantity_scale
    }) > 0) {
      return { ok: false, reason: 'exceeds_available', lotID: draft.lotID };
    }
    seen.add(draft.lotID);
    allocations.push({ lot_id: draft.lotID,
      quantity_value: parsed.field.value, quantity_scale: parsed.field.scale });
  }
  return { ok: true, allocations };
}
