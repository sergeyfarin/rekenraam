import { describe, expect, it } from 'vitest';
import { parseReconciliationWindow, reconciliationAgeView } from './reconciliation-age.ts';

describe('reconciliationAgeView', () => {
  it('says each state in words, flagging the two that need attention', () => {
    expect(reconciliationAgeView({ state: 'reconciled', last_statement_date: '2026-09-30' }, 'en')).toEqual({ text: 'Reconciled to Sep 30, 2026', attention: false });
    expect(reconciliationAgeView({ state: 'out_of_date', last_statement_date: '2026-03-31' }, 'en')).toEqual({ text: 'Out of date: reconciled to Mar 31, 2026', attention: true });
    expect(reconciliationAgeView({ state: 'never_reconciled', last_statement_date: null }, 'en')).toEqual({ text: 'Never reconciled', attention: true });
    expect(reconciliationAgeView({ state: 'nothing_to_reconcile', last_statement_date: null }, 'en')).toEqual({ text: 'Nothing to reconcile', attention: false });
  });

  it('formats the statement date in the active locale', () => {
    expect(reconciliationAgeView({ state: 'reconciled', last_statement_date: '2026-09-30' }, 'nl').text).toContain('30 sep 2026');
  });
});

describe('parseReconciliationWindow', () => {
  it('accepts whole days from 1 to 3650 only', () => {
    expect(parseReconciliationWindow(' 60 ')).toBe(60);
    expect(parseReconciliationWindow('1')).toBe(1);
    expect(parseReconciliationWindow('3650')).toBe(3650);
    for (const input of ['', '0', '3651', '1.5', '-3', '6e1', 'sixty']) expect(parseReconciliationWindow(input)).toBeNull();
  });
});
