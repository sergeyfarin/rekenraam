import type { components } from '$lib/api/schema';

export type RegisterCorrectionChain = components['schemas']['RegisterCorrectionChain'];
export type RegisterCorrectionMember = components['schemas']['RegisterCorrectionMember'];
export type RegisterCorrectionRole = components['schemas']['RegisterCorrectionRole'];

/**
 * What a register row's correction badge says (T-120). A reversal is always a
 * reversal; the chain's effective transaction is the current correction; any
 * other original or replacement has itself been corrected.
 */
export type CorrectionBadge = 'reversal' | 'current' | 'corrected';

export function correctionBadge(chain: RegisterCorrectionChain, transactionID: number): CorrectionBadge {
  if (chain.role === 'reversal') return 'reversal';
  if (chain.effective_transaction_id === transactionID) return 'current';
  return 'corrected';
}

export function isEffectiveMember(chain: RegisterCorrectionChain, member: RegisterCorrectionMember): boolean {
  return chain.effective_transaction_id === member.transaction_id;
}
