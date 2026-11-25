import { describe, expect, it } from 'vitest';
import { applicableDividendDefault, defaultAccountToApply, defaultStillApplies, heldButUnavailable } from './dividend-default-choice.ts';

describe('dividend default choice (#664 review)', () => {
  const offered = [{ id: 7, status: 'active' }, { id: 8, status: 'active' }];

  it('applies a default offered on the date and not archived today', () => {
    expect(defaultAccountToApply(7, offered, [{ id: 7, status: 'active' }])).toBe('7');
  });

  it('does not apply a default archived today, even where a backdated picker offers it', () => {
    expect(defaultAccountToApply(7, offered, [{ id: 7, status: 'archived' }])).toBe('');
  });

  it('does not apply a default the dated picker does not offer', () => {
    expect(defaultAccountToApply(9, offered, [{ id: 9, status: 'active' }])).toBe('');
  });

  it('does not apply a default missing from today\'s list or absent', () => {
    expect(defaultAccountToApply(8, offered, [])).toBe('');
    expect(defaultAccountToApply(undefined, offered, offered)).toBe('');
  });

  it('flags a held selection the picker no longer offers', () => {
    expect(heldButUnavailable('9', offered)).toBe(true);
    expect(heldButUnavailable('7', offered)).toBe(false);
    expect(heldButUnavailable('', offered)).toBe(false);
  });

  it('a filled default stops applying once it is archived today or leaves today\'s list', () => {
    expect(defaultStillApplies('7', [{ id: 7, status: 'active' }])).toBe(true);
    expect(defaultStillApplies('7', [{ id: 7, status: 'archived' }])).toBe(false);
    expect(defaultStillApplies('7', [])).toBe(false);
  });

  it('resolves the default in effect on the entry date by the server\'s precedence', () => {
    const defaults = [
      { id: 1, commodity_id: null, status: 'active', effective_from: '2026-01-01' },
      { id: 2, commodity_id: 5, status: 'active', effective_from: '2026-01-01', effective_to: '2026-02-28' },
      { id: 3, commodity_id: 5, status: 'active', effective_from: '2026-03-01' },
      { id: 4, commodity_id: 5, status: 'archived', effective_from: '2026-03-05' },
      { id: 5, commodity_id: 6, status: 'active', effective_from: '2026-01-01' }
    ];
    expect(applicableDividendDefault(defaults, 5, '2026-02-15')?.id).toBe(2);
    expect(applicableDividendDefault(defaults, 5, '2026-03-10')?.id).toBe(3);
    expect(applicableDividendDefault(defaults, 7, '2026-03-10')?.id).toBe(1);
    expect(applicableDividendDefault(defaults, 5, '2025-12-31')).toBeUndefined();
    const sameDay = [...defaults, { id: 9, commodity_id: 5, status: 'active', effective_from: '2026-03-01' }];
    expect(applicableDividendDefault(sameDay, 5, '2026-03-02')?.id).toBe(9);
  });
});
