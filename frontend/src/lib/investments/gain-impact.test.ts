import { describe, expect, it } from 'vitest';
import type { GainImpactChange } from '$lib/api/investments';
import { gainImpactRows, hasGainChanges } from './gain-impact';

function state(gain: string | null, date = '2026-03-01'): GainImpactChange['before'] {
  return {
    account_id: 3, commodity_id: 4, cost_commodity_id: 5, disposal_date: date,
    cost_basis_method: 'fifo', quantity_value: '5', quantity_scale: 0,
    basis_knowledge: gain === null ? 'unknown' : 'known',
    disposed_basis_value: gain === null ? null : '10000', disposed_basis_scale: gain === null ? null : 2,
    proceeds_value: '15000', proceeds_scale: 2,
    realized_gain_value: gain, realized_gain_scale: gain === null ? null : 2
  };
}

function change(kind: GainImpactChange['change_kind'], before: GainImpactChange['before'], after: GainImpactChange['after']): GainImpactChange {
  return { change_kind: kind, root_operation_id: 7, decision_seq: 1, operation_id: 7, decision_id: 11,
    transaction_id: 8, before, after };
}

describe('gainImpactRows', () => {
  it('formats the issue FIFO case exactly: 50.00 to 140.00', () => {
    const [row] = gainImpactRows([change('revised', state('5000'), state('14000'))], () => ({ code: 'EUR', standardScale: 2 }), 'en-US');
    expect(row).toEqual({ key: '7:1', kind: 'revised', date: '2026-03-01', before: '50.00', after: '140.00', currency: 'EUR' });
  });

  it('drops replay scale padding without rounding a revision away', () => {
    const wide = (gain: string) => ({ ...state(gain), realized_gain_scale: 6 });
    const [row] = gainImpactRows([change('revised', wide('50000000'), wide('50004000'))], () => ({ code: 'EUR', standardScale: 2 }), 'en-US');
    expect([row.before, row.after]).toEqual(['50.00', '50.004']);
  });

  it('keeps losses signed and unknown gains null rather than zero', () => {
    const [row] = gainImpactRows([change('revised', state('-2050'), state(null))], () => ({ code: 'EUR', standardScale: 2 }), 'en-US');
    expect(row.before).toBe('-20.50');
    expect(row.after).toBeNull();
  });

  it('shows a removed disposal without an after value and a replacement on its new date', () => {
    const rows = gainImpactRows([
      change('removed', state('8000'), null),
      change('replaced', state('8000'), state('10000', '2026-03-02'))
    ], () => ({ code: 'USD', standardScale: 2 }), 'en-US');
    expect(rows[0].after).toBeNull();
    expect(rows[1].date).toBe('2026-03-02');
  });
});

describe('hasGainChanges', () => {
  it('requires acknowledgement only for a non-empty change set', () => {
    expect(hasGainChanges(undefined)).toBe(false);
    expect(hasGainChanges({ changes: [], acknowledgement: '' })).toBe(false);
    expect(hasGainChanges({ changes: [change('removed', state('1'), null)], acknowledgement: 'x' })).toBe(true);
  });
});
