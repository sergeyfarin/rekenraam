import { describe, expect, it } from 'vitest';
import type { ImportResolution, ImportStagedRow } from '$lib/api/imports';
import type { PayeeResponse } from '$lib/api/payees';
import { unknownImportPayeeGroups } from './payee-resolution';

const payee = (id: number, name: string): PayeeResponse => ({ id, name }) as PayeeResponse;

function row(
  id: number,
  payeeHint: string,
  dedupeStatus: ImportStagedRow['dedupe_status'] = 'new'
): ImportStagedRow {
  return {
    id,
    batch_id: 1,
    row_index: id - 1,
    dedupe_fingerprint: `row-${id}`,
    normalized: JSON.stringify({
      date: '2026-08-28',
      amount: '-12.34',
      commodity_hint: 'EUR',
      payee_hint: payeeHint,
      category_hint: '',
      account_hint: '',
      transfer_hint: '',
      memo: '',
      external_ref: '',
      splits: []
    }),
    raw: '{}',
    dedupe_status: dedupeStatus,
    resolution: '{}',
    commit_status: 'pending'
  };
}

describe('unknownImportPayeeGroups', () => {
  const payees = [payee(1, 'Market Hall'), payee(2, 'Corner Bakery')];

  it('groups repeated unknown names using the same normalization as stored payees', () => {
    const groups = unknownImportPayeeGroups(
      [row(1, 'Coffee Place'), row(2, '  coffee   place '), row(3, 'Another Shop')],
      new Map(),
      payees
    );

    expect(groups.map((group) => ({ name: group.name, rowIDs: group.rowIDs }))).toEqual([
      { name: 'Coffee Place', rowIDs: [1, 2] },
      { name: 'Another Shop', rowIDs: [3] }
    ]);
  });

  it('does not ask about exact existing, already resolved, duplicate, excluded, or blank payees', () => {
    const resolutions = new Map<number, ImportResolution>([
      [2, { payee_id: 1, payee_name: 'Market Hall' }],
      [4, { exclude: true }]
    ]);

    const groups = unknownImportPayeeGroups(
      [
        row(1, ' market hall '),
        row(2, 'Unresolved text'),
        row(3, 'Duplicate text', 'duplicate'),
        row(4, 'Excluded locally'),
        row(5, '', 'new'),
        row(6, 'Excluded on server', 'excluded')
      ],
      resolutions,
      payees
    );

    expect(groups).toEqual([]);
  });

  it('offers fuzzy near matches for each unknown name', () => {
    const [group] = unknownImportPayeeGroups([row(1, 'Markt Hall')], new Map(), payees);

    expect(group.suggestions.map((suggestion) => suggestion.id)).toContain(1);
  });
});
