import { describe, expect, it } from 'vitest';
import { inflowPositiveAmount, negateCoefficient } from '#lib/money/amount.ts';
import {
  accountLedgerValue,
  approvalKey,
  approvalStillValid,
  cashAccountProblem,
  directionForTab,
  directionOf,
  entryTabLabel,
  entryTabs,
  investmentMuted,
  shapeForTab,
  startingTab
} from './entry-mode.ts';

const account = (account_class: string, account_kind: string, extra: Record<string, unknown> = {}) => ({
  account_class,
  account_kind,
  status: 'active',
  is_system: false,
  allows_postings: true,
  ...extra
});
const checking = account('asset', 'checking');
const card = account('liability', 'credit_card');
const loan = account('liability', 'loan');
const brokerageCash = account('asset', 'brokerage_cash');
const holding = account('asset', 'security_holding');

describe('entry tabs', () => {
  it('keeps four tabs in one order', () => {
    expect(entryTabs).toEqual(['out', 'in', 'transfer', 'investment']);
  });

  it('labels the cash tabs by account: card words on credit cards only', () => {
    const labels = (a?: { account_kind?: string }) => entryTabs.map((tab) => entryTabLabel(tab, a));
    expect(labels(checking)).toEqual(['withdrawal', 'deposit', 'transfer', 'investment']);
    expect(labels(card)).toEqual(['charge', 'credit', 'transfer', 'investment']);
    expect(labels(loan)).toEqual(['withdrawal', 'deposit', 'transfer', 'investment']);
    expect(labels(account('liability', 'mortgage'))).toEqual(['withdrawal', 'deposit', 'transfer', 'investment']);
    expect(labels(undefined)).toEqual(['withdrawal', 'deposit', 'transfer', 'investment']);
  });

  it('mutes Investment from an ordinary account outside a brokerage', () => {
    expect(investmentMuted(checking)).toBe(true);
    expect(investmentMuted(card)).toBe(true);
    expect(investmentMuted(brokerageCash)).toBe(false);
    expect(investmentMuted(holding)).toBe(false);
    expect(investmentMuted(undefined)).toBe(false);
  });

  it('starts in the caller’s tab, Investment for a holding, money out otherwise', () => {
    expect(startingTab({})).toBe('out');
    expect(startingTab({ account: checking })).toBe('out');
    expect(startingTab({ account: holding })).toBe('investment');
    expect(startingTab({ tab: 'transfer', account: holding })).toBe('transfer');
    expect(startingTab({ tab: 'in' })).toBe('in');
  });

  it('maps tabs to a direction and the cash tabs to their shared shape', () => {
    expect(directionForTab('out')).toBe('out');
    expect(directionForTab('in')).toBe('in');
    expect(directionForTab('transfer')).toBeUndefined();
    // Withdrawal → Transfer → Deposit comes back to the split the owner left.
    expect(shapeForTab('out', 'split')).toBe('split');
    expect(shapeForTab('transfer', 'split')).toBe('transfer');
    expect(shapeForTab('in', 'split')).toBe('split');
    expect(shapeForTab('investment', 'simple')).toBe('investment');
  });
});

describe('mode → ledger sign', () => {
  it('posts money out as a credit and money in as a debit, whatever the class', () => {
    expect(accountLedgerValue('out', '1250')).toBe('-1250');
    expect(accountLedgerValue('in', '1250')).toBe('1250');
    expect(accountLedgerValue('out', '0')).toBe('0');
    expect(accountLedgerValue('in', '000')).toBe('0');
  });

  // Before #506 the owner typed a signed amount: inflow positive on an asset,
  // and on a liability the typed sign was flipped. Each case is what the owner
  // typed then for the same intent; the ledger must not move.
  const before = (accountClass: string, typed: string) => (accountClass === 'liability' ? negateCoefficient(typed) : typed);
  const cases: [string, string, 'out' | 'in', string][] = [
    ['asset (checking) withdrawal', 'asset', 'out', '-4250'],
    ['asset (checking) deposit', 'asset', 'in', '4250'],
    ['credit card charge', 'liability', 'out', '4250'],
    ['credit card credit (refund)', 'liability', 'in', '-4250'],
    ['loan drawdown (withdrawal)', 'liability', 'out', '4250'],
    ['loan repayment (deposit)', 'liability', 'in', '-4250']
  ];
  for (const [name, accountClass, direction, typed] of cases) {
    it(`${name} keeps today’s ledger result`, () => {
      expect(accountLedgerValue(direction, '4250')).toBe(before(accountClass, typed));
    });
  }

  it('reads a stored posting back as the same direction and magnitude', () => {
    for (const value of ['-4250', '4250', '0']) {
      for (const accountClass of ['asset', 'liability']) {
        const { direction, magnitude } = directionOf(value);
        expect(magnitude.startsWith('-')).toBe(false);
        expect(accountLedgerValue(direction, magnitude)).toBe(value);
        // And it agrees with the signed amount the editor used to show.
        const shown = inflowPositiveAmount({ quantity_value: value, quantity_scale: 0, account_class: accountClass });
        expect(before(accountClass, shown)).toBe(value);
      }
    }
    expect(directionOf('-1').direction).toBe('out');
    expect(directionOf('1').direction).toBe('in');
    expect(directionOf('0').direction).toBe('out');
  });
});

describe('account change revalidation', () => {
  it('accepts ordinary cash accounts and explains a holding', () => {
    expect(cashAccountProblem(checking)).toBeUndefined();
    expect(cashAccountProblem(card)).toBeUndefined();
    expect(cashAccountProblem(brokerageCash)).toBeUndefined();
    expect(cashAccountProblem(undefined)).toBeUndefined();
    expect(cashAccountProblem(holding)).toBe('holding');
    expect(cashAccountProblem(account('asset', 'fund_holding'))).toBe('holding');
    expect(cashAccountProblem(account('asset', 'checking', { status: 'archived' }))).toBe('not_ordinary');
  });
});

describe('preview invalidation', () => {
  const request = { transaction_date: '2026-11-05', journal_entries: [{ postings: [{ account_id: 1, quantity_value: '-1250' }] }] };

  it('authorizes exactly the request it reviewed', () => {
    const key = approvalKey(request);
    expect(approvalStillValid(key, structuredClone(request))).toBe(true);
  });

  it('is invalid after a direction, account or tab change', () => {
    const key = approvalKey(request);
    const flipped = structuredClone(request);
    flipped.journal_entries[0].postings[0].quantity_value = '1250';
    const moved = structuredClone(request);
    moved.journal_entries[0].postings[0].account_id = 2;
    const transfer = { ...structuredClone(request), transaction_kind: 'transfer' };
    for (const changed of [flipped, moved, transfer]) expect(approvalStillValid(key, changed)).toBe(false);
  });

  it('is never valid without a review', () => {
    expect(approvalStillValid(undefined, request)).toBe(false);
  });
});
