import { describe, expect, it } from 'vitest';
import { accountEntryKind, isOrdinaryEntryAccount } from './entry-account';

const account = (overrides: Partial<Parameters<typeof isOrdinaryEntryAccount>[0]> = {}) => ({
  account_class: 'asset', account_kind: 'checking', status: 'active', is_system: false, allows_postings: true, ...overrides
});

describe('accountEntryKind (#185)', () => {
  it('opens the generic editor for ordinary asset and liability accounts', () => {
    expect(accountEntryKind(account())).toBe('editor');
    expect(accountEntryKind(account({ account_class: 'liability', account_kind: 'credit_card' }))).toBe('editor');
  });

  it('offers all entry modes for an active brokerage container without allowing it to receive cash postings', () => {
    const broker = account({ account_kind: 'brokerage', allows_postings: false });
    expect(accountEntryKind(broker)).toBe('editor');
    expect(isOrdinaryEntryAccount(broker)).toBe(false);
    expect(accountEntryKind({ ...broker, status: 'archived' })).toBe('none');
    expect(accountEntryKind({ ...broker, is_system: true })).toBe('none');
  });

  it('sends a holding account to investment entry, never the generic editor', () => {
    for (const kind of ['security_holding', 'fund_holding']) {
      expect(isOrdinaryEntryAccount(account({ account_kind: kind }))).toBe(false);
      expect(accountEntryKind(account({ account_kind: kind }))).toBe('investments');
    }
  });

  it('offers nothing for an archived, system, summary or category account', () => {
    expect(accountEntryKind(account({ status: 'archived' }))).toBe('none');
    expect(accountEntryKind(account({ is_system: true }))).toBe('none');
    expect(accountEntryKind(account({ allows_postings: false }))).toBe('none');
    expect(accountEntryKind(account({ account_class: 'expense', account_kind: 'expense' }))).toBe('none');
  });
});
