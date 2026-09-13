import { describe, expect, it } from 'vitest';

import { isSubledgerManagedAccountKind } from './subledger-accounts';

describe('isSubledgerManagedAccountKind', () => {
  // T-96: the editor used to exclude only 'security_holding', and only in
  // template mode, so an ordinary entry could put shares in a fund holding
  // with no lot behind them.
  it('covers every holding kind the investment commands manage', () => {
    expect(isSubledgerManagedAccountKind('security_holding')).toBe(true);
    expect(isSubledgerManagedAccountKind('fund_holding')).toBe(true);
  });

  it('leaves ordinary posting accounts alone', () => {
    for (const kind of ['checking', 'savings', 'brokerage', 'brokerage_cash', 'credit_card', 'income', 'expense']) {
      expect(isSubledgerManagedAccountKind(kind)).toBe(false);
    }
  });

  // There is no investment command for a crypto account, so excluding it would
  // remove the only way to record a crypto balance rather than redirecting to a
  // better one.
  it('does not exclude crypto wallets, which have no investment command to use instead', () => {
    expect(isSubledgerManagedAccountKind('crypto_wallet')).toBe(false);
  });

  it('treats a missing kind as ordinary', () => {
    expect(isSubledgerManagedAccountKind(undefined)).toBe(false);
    expect(isSubledgerManagedAccountKind(null)).toBe(false);
  });
});
