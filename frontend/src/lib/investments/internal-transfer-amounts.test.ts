import { describe, expect, it } from 'vitest';
import type { InvestmentLotResponse } from '$lib/api/investments';
import { parseInternalTransferAllocations } from './internal-transfer-amounts';

const lot = {
  id: 7, status: 'open', remaining_quantity_value: '125', remaining_quantity_scale: 2
} as InvestmentLotResponse;

describe('parseInternalTransferAllocations', () => {
  it('accepts an exact partial lot quantity and preserves its scale', () => {
    expect(parseInternalTransferAllocations([{ lotID: 7, quantity: '1.20' }], [lot], 4)).toEqual({
      ok: true, allocations: [{ lot_id: 7, quantity_value: '120', quantity_scale: 2 }]
    });
  });

  it('rejects a larger quantity even when its coefficient looks smaller', () => {
    expect(parseInternalTransferAllocations([{ lotID: 7, quantity: '1.251' }], [lot], 4)).toEqual({
      ok: false, reason: 'exceeds_available', lotID: 7
    });
  });

  it('requires selected, unique, positive, available lots at instrument precision', () => {
    expect(parseInternalTransferAllocations([], [lot], 4)).toEqual({ ok: false, reason: 'empty' });
    expect(parseInternalTransferAllocations([{ lotID: 7, quantity: '0' }], [lot], 4)).toEqual({ ok: false, reason: 'invalid', lotID: 7 });
    expect(parseInternalTransferAllocations([{ lotID: 7, quantity: '0.00001' }], [lot], 4)).toEqual({ ok: false, reason: 'invalid', lotID: 7 });
    expect(parseInternalTransferAllocations([{ lotID: 7, quantity: '1' }, { lotID: 7, quantity: '0.1' }], [lot], 4)).toEqual({ ok: false, reason: 'invalid', lotID: 7 });
    expect(parseInternalTransferAllocations([{ lotID: 8, quantity: '1' }], [lot], 4)).toEqual({ ok: false, reason: 'invalid', lotID: 8 });
  });
});
