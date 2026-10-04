import { describe, expect, it } from 'vitest';
import type { ImportStagedRow } from '#lib/api/imports.ts';
import { linkableSplitFill, sourceCorrectionKind, unsupportedSourceFill } from './source-correction';

function row(changes: Partial<ImportStagedRow> = {}): ImportStagedRow {
  return {
    id: 1, batch_id: 1, row_index: 0, dedupe_fingerprint: 'fill',
    normalized: '{}', raw: '{"kind":"trading212_order_fill","fill_type":"TRADE","side":"SELL"}',
    dedupe_status: 'needs_attention', source_changed: true, source_transaction_id: 12,
    source_buy_operation: false, source_sale_operation: true,
    resolution: '{}', commit_status: 'pending', commit_effects: [], ...changes
  };
}

describe('sourceCorrectionKind', () => {
  it('offers native sale correction for pending and skipped changed fills', () => {
    expect(sourceCorrectionKind(row())).toBe('sale');
    expect(sourceCorrectionKind(row({ commit_status: 'skipped' }))).toBe('sale');
    expect(sourceCorrectionKind(row({ raw: '{"kind":"trading212_order_fill","fill_type":"TRADE","side":" sell "}' }))).toBe('sale');
  });

  it('retains buy correction and requires the matching native source effect', () => {
    const buy = row({ source_buy_operation: true, source_sale_operation: false, raw: '{"kind":"trading212_order_fill","fill_type":"TRADE","side":"BUY"}' });
    expect(sourceCorrectionKind(buy)).toBe('buy');
    expect(sourceCorrectionKind(row({ source_sale_operation: false }))).toBeNull();
    expect(sourceCorrectionKind({ ...buy, source_buy_operation: false, source_sale_operation: true })).toBeNull();
  });

  it.each([
    { source_changed: false }, { source_transaction_id: undefined },
    { commit_status: 'committed' as const }, { commit_status: 'failed' as const },
    { raw: '{' }, { raw: 'null' }, { raw: '{"kind":"cash","side":"SELL"}' },
    { raw: '{"kind":"trading212_order_fill","fill_type":"TRADE","side":1}' }
  ])('keeps ineligible rows under review: %j', (changes) => {
    expect(sourceCorrectionKind(row(changes))).toBeNull();
  });
});

describe('unsupportedSourceFill', () => {
  it.each(['STOCK_SPLIT', 'FOP_CORRECTION', 'FUTURE_TYPE', '', undefined])('holds %s under review', (fill_type) => {
    const held = row({ raw: JSON.stringify({ kind: 'trading212_order_fill', side: 'SELL', fill_type }) });
    expect(unsupportedSourceFill(held)).toBe(true);
    expect(sourceCorrectionKind(held)).toBeNull();
  });
  it('accepts a real trade on a cancelled order without treating it as a reversal', () => {
    const trade = row({ raw: JSON.stringify({ kind: 'trading212_order_fill', side: 'SELL', fill_type: 'TRADE', order_status: 'CANCELLED' }) });
    expect(unsupportedSourceFill(trade)).toBe(false);
    expect(sourceCorrectionKind(trade)).toBe('sale');
    expect(unsupportedSourceFill(row({ raw: '{"kind":"cash"}' }))).toBe(false);
  });
});

describe('linkableSplitFill', () => {
  const split = '{"kind":"trading212_order_fill","fill_type":"STOCK_SPLIT","side":"BUY"}';
  it('offers linking for a pending or skipped split fill only', () => {
    expect(linkableSplitFill(row({ raw: split }))).toBe(true);
    expect(linkableSplitFill(row({ raw: split, commit_status: 'skipped' }))).toBe(true);
    expect(linkableSplitFill(row({ raw: split, commit_status: 'committed' }))).toBe(false);
    expect(linkableSplitFill(row({ raw: split, dedupe_status: 'duplicate' }))).toBe(false);
    expect(linkableSplitFill(row())).toBe(false);
    expect(linkableSplitFill(row({ raw: 'not json' }))).toBe(false);
  });
});
