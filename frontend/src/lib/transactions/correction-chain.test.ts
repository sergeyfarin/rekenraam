import { describe, expect, it } from 'vitest';
import { correctionBadge, isEffectiveMember, type RegisterCorrectionChain } from './correction-chain';

const member = (transaction_id: number, role: RegisterCorrectionChain['role'], correction_of_transaction_id: number | null) => ({
  transaction_id,
  correction_of_transaction_id,
  role,
  transaction_date: '2026-01-01',
  status: 'posted' as const,
  deleted: false,
  reason: role === 'original' ? '' : 'broker corrected the fill',
  amount: { quantity_value: '-100000', quantity_scale: 2 }
});

const chain = (role: RegisterCorrectionChain['role'], effective: number | null): RegisterCorrectionChain => ({
  root_transaction_id: 1,
  role,
  effective_transaction_id: effective,
  net_effect: { quantity_value: '-120000', quantity_scale: 2 },
  members: [member(1, 'original', null), member(2, 'reversal', 1), member(3, 'replacement', 1)]
});

describe('correctionBadge', () => {
  it('marks the corrected original, the reversal and the current replacement', () => {
    expect(correctionBadge(chain('original', 3), 1)).toBe('corrected');
    expect(correctionBadge(chain('reversal', 3), 2)).toBe('reversal');
    expect(correctionBadge(chain('replacement', 3), 3)).toBe('current');
  });

  it('marks a replacement that was corrected again as corrected', () => {
    expect(correctionBadge(chain('replacement', 5), 3)).toBe('corrected');
  });

  it('never calls anything current once the chain ends in a reversal', () => {
    expect(correctionBadge(chain('original', null), 1)).toBe('corrected');
    expect(chain('original', null).members.some((m) => isEffectiveMember(chain('original', null), m))).toBe(false);
  });
});
