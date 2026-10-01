import { describe, expect, it } from 'vitest';
import type { ImportStagedRow } from '$lib/api/imports';
import { sourceCorrectionKind } from './source-correction';

function row(changes: Partial<ImportStagedRow> = {}): ImportStagedRow {
  return {
    id: 1, batch_id: 1, row_index: 0, dedupe_fingerprint: 'fill',
    normalized: '{}', raw: '{"kind":"trading212_order_fill","side":"SELL"}',
    dedupe_status: 'needs_attention', source_changed: true, source_transaction_id: 12,
    source_buy_operation: false, source_sale_operation: true,
    resolution: '{}', commit_status: 'pending', commit_effects: [], ...changes
  };
}

describe('sourceCorrectionKind', () => {
  it('offers native sale correction for pending and skipped changed fills', () => {
    expect(sourceCorrectionKind(row())).toBe('sale');
    expect(sourceCorrectionKind(row({ commit_status: 'skipped' }))).toBe('sale');
    expect(sourceCorrectionKind(row({ raw: '{"kind":"trading212_order_fill","side":" sell "}' }))).toBe('sale');
  });

  it('retains buy correction and requires the matching native source effect', () => {
    const buy = row({ source_buy_operation: true, source_sale_operation: false, raw: '{"kind":"trading212_order_fill","side":"BUY"}' });
    expect(sourceCorrectionKind(buy)).toBe('buy');
    expect(sourceCorrectionKind(row({ source_sale_operation: false }))).toBeNull();
    expect(sourceCorrectionKind({ ...buy, source_buy_operation: false, source_sale_operation: true })).toBeNull();
  });

  it.each([
    { source_changed: false }, { source_transaction_id: undefined },
    { commit_status: 'committed' as const }, { commit_status: 'failed' as const },
    { raw: '{' }, { raw: 'null' }, { raw: '{"kind":"cash","side":"SELL"}' },
    { raw: '{"kind":"trading212_order_fill","side":1}' }
  ])('keeps ineligible rows under review: %j', (changes) => {
    expect(sourceCorrectionKind(row(changes))).toBeNull();
  });
});
