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
  rebindSplitAccount,
  shapeForTab,
  splitDirection,
  splitLegCommodities as splitLegCommoditiesOf,
  splitLineProblem,
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

describe('split entry under the cash tabs', () => {
  const leg = (accountID: number, value: string | null, scale = 2) => ({
    accountID: String(accountID),
    amount: value === null ? null : { value, scale }
  });

  it('reads the direction from the net of the entry account legs', () => {
    expect(splitDirection([leg(1, '-1240'), leg(9, '1240')], 1)).toBe('out');
    expect(splitDirection([leg(1, '1240'), leg(9, '-1240')], 1)).toBe('in');
    // Two legs on the account, at different scales: the net decides.
    expect(splitDirection([leg(1, '-1000'), leg(1, '5', 0), leg(9, '-4000')], 1)).toBe('out');
    expect(splitDirection([leg(1, '-1000'), leg(1, '20', 0), leg(9, '-1000')], 1)).toBe('in');
  });

  it('contradicts no tab when it cannot tell', () => {
    expect(splitDirection([leg(9, '-1240'), leg(8, '1240')], 1)).toBeUndefined();
    expect(splitDirection([leg(1, null), leg(9, '1240')], 1)).toBeUndefined();
    expect(splitDirection([leg(1, '-500'), leg(1, '500')], 1)).toBeUndefined();
    expect(splitDirection([leg(1, '-500')], undefined)).toBeUndefined();
  });

  it('moves the entry account legs to the new account in its currency', () => {
    const legs = [
      { accountID: '1', commodityID: '10', amountStr: '-12.40' },
      { accountID: '9', commodityID: '10', amountStr: '12.40' }
    ];
    const currency: Record<string, string> = { '1': '10', '2': '20', '9': '' };
    const moved = rebindSplitAccount(legs, 1, 2, (id) => currency[id] ?? '');
    expect(moved).toEqual([
      { accountID: '2', commodityID: '20', amountStr: '-12.40' },
      { accountID: '9', commodityID: '10', amountStr: '12.40' }
    ]);
  });

  it('swaps the two accounts when the split already posts to the new one', () => {
    const legs = [
      { accountID: '1', commodityID: '10', amountStr: '-100' },
      { accountID: '2', commodityID: '10', amountStr: '100' }
    ];
    const moved = rebindSplitAccount(legs, 1, 2, () => '10');
    expect(moved.map((l) => [l.accountID, l.amountStr])).toEqual([['2', '-100'], ['1', '100']]);
  });
});

describe('split line currencies (#511)', () => {
  // 1 and 2 are EUR accounts, 3 a USD account, 4 a receivable with no
  // currency of its own; 10 is a category with none.
  const own: Record<string, string> = { '1': 'eur', '2': 'eur', '3': 'usd', '4': '', '10': '' };
  const ownOf = (id: string) => own[id] ?? '';
  const isAccountLine = (id: string) => id !== '10';
  const splitLegCommodities = (legs: Array<{ accountID: string; commodityID: string }>, of: typeof ownOf) => splitLegCommoditiesOf(legs, of, isAccountLine);
  const leg = (accountID: string, commodityID = '') => ({ accountID, commodityID });

  it('gives a currency-less line the currency its account lines share', () => {
    expect(splitLegCommodities([leg('1', 'eur'), leg('10')], ownOf)).toEqual(['eur', 'eur']);
    expect(splitLegCommodities([leg('1', 'eur'), leg('2', 'eur'), leg('10')], ownOf)).toEqual(['eur', 'eur', 'eur']);
  });

  it('follows an account line chosen after the category line', () => {
    expect(splitLegCommodities([leg('10'), leg('1')], ownOf)).toEqual(['eur', 'eur']);
  });

  it('leaves it unset when the account lines use two currencies, or there are none', () => {
    expect(splitLegCommodities([leg('1', 'eur'), leg('3', 'usd'), leg('10')], ownOf)).toEqual(['eur', 'usd', '']);
    expect(splitLegCommodities([leg('10'), leg('')], ownOf)).toEqual(['', '']);
  });

  it('keeps a currency the line carries, the owner’s choice included', () => {
    expect(splitLegCommodities([leg('1', 'eur'), leg('10', 'usd')], ownOf)).toEqual(['eur', 'usd']);
  });

  // #512 review: a currency chosen on an account line counts like a default.
  it('counts a currency chosen on a currency-less account line', () => {
    expect(splitLegCommodities([leg('4', 'usd'), leg('10')], ownOf)).toEqual(['usd', 'usd']);
    expect(splitLegCommodities([leg('1', 'eur'), leg('4', 'usd'), leg('10')], ownOf)).toEqual(['eur', 'usd', '']);
  });

  it('lets an account line without a currency yet follow, without counting it', () => {
    expect(splitLegCommodities([leg('1', 'eur'), leg('4'), leg('10')], ownOf)).toEqual(['eur', 'eur', 'eur']);
  });

  it('never takes the shared currency from a category line', () => {
    expect(splitLegCommodities([leg('10', 'usd'), leg('10')], ownOf)).toEqual(['usd', '']);
  });
});

describe('split line problems (#511)', () => {
  const line = (accountID: string, commodityID: string, amountStr: string) => ({ accountID, commodityID, amountStr });

  it('ignores blank lines', () => {
    expect(splitLineProblem([line('1', 'eur', '-12.40'), line('10', 'eur', '12.40'), line('', '', ' ')])).toBeNull();
  });

  it('names the first filled line the save would otherwise drop', () => {
    expect(splitLineProblem([line('1', 'eur', '-12.40'), line('10', '', '12.40')])).toEqual({ line: 2, missing: 'currency' });
    expect(splitLineProblem([line('1', 'eur', '-12.40'), line('', '', '12.40')])).toEqual({ line: 2, missing: 'account' });
    expect(splitLineProblem([line('1', 'eur', ''), line('10', 'eur', '12.40')])).toEqual({ line: 1, missing: 'amount' });
    expect(splitLineProblem([line('1', 'eur', 'abc'), line('10', '', '12.40')])).toEqual({ line: 1, missing: 'number' });
  });
});
