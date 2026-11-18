import { describe, expect, it } from 'vitest';
import {
  markersByRowIndex,
  registerStatementMarkers,
  type AccountRegisterStatementResponse
} from './register-statements.ts';

function statement(checkpointID: number, beforeEntryIndex: number, date = '2026-03-31'): AccountRegisterStatementResponse {
  return {
    checkpoint_id: checkpointID,
    commodity_id: 1,
    commodity_code: 'EUR',
    commodity_symbol: '€',
    statement_date: date,
    statement_account_sequence: 0,
    statement_balance: { commodity_id: 1, quantity_value: '123456', quantity_scale: 2, normal_quantity_value: '123456' },
    before_entry_index: beforeEntryIndex
  };
}

// Entries are opaque here: only their count places statement rows.
const entries = (count: number) => Array.from({ length: count }, () => ({}) as never);

describe('registerStatementMarkers', () => {
  it('offsets each page by the entries of the pages before it', () => {
    const markers = registerStatementMarkers([
      { entries: entries(3), statements: [statement(9, 0), statement(8, 2)] },
      { entries: entries(2), statements: [statement(7, 1)] }
    ]);
    expect(markers.map((marker) => [marker.key, marker.beforeIndex])).toEqual([
      ['statement-9', 0],
      ['statement-8', 2],
      ['statement-7', 4]
    ]);
  });

  it('places a statement that ends a page before the next page’s first entry', () => {
    const markers = registerStatementMarkers([
      { entries: entries(2), statements: [statement(5, 2)] },
      { entries: entries(1), statements: [] }
    ]);
    expect(markers.map((marker) => marker.beforeIndex)).toEqual([2]);
  });

  it('keeps statements of a dormant account with no entries on the page, in order', () => {
    const markers = registerStatementMarkers([
      { entries: [], statements: [statement(3, 0, '2026-03-31'), statement(2, 0, '2026-02-28'), statement(1, 0, '2026-01-31')] }
    ]);
    expect(markers.map((marker) => marker.statement.statement_date)).toEqual(['2026-03-31', '2026-02-28', '2026-01-31']);
    expect(markers.every((marker) => marker.beforeIndex === 0)).toBe(true);
  });

  it('clamps an index outside its page instead of moving the row to another page', () => {
    const markers = registerStatementMarkers([
      { entries: entries(1), statements: [statement(4, 5)] },
      { entries: entries(3), statements: [] }
    ]);
    expect(markers[0]?.beforeIndex).toBe(1);
  });
});

describe('markersByRowIndex', () => {
  it('groups markers by the row they precede and clamps to the row count', () => {
    const grouped = markersByRowIndex([{ beforeIndex: 0 }, { beforeIndex: 0 }, { beforeIndex: 7 }], 3);
    expect(grouped.get(0)).toHaveLength(2);
    expect(grouped.get(3)).toHaveLength(1);
  });
});
