import { parseISO } from 'date-fns';
import type { AccountReconciliation } from '#lib/api/reconciliation.ts';
import { m } from '#lib/paraglide/messages.js';

/**
 * Words for the server's reconciliation judgement (#482). The account list
 * and Home's Reconciliation widget both show it, so the rule (the window, the
 * boundary day, what counts as never reconciled) stays on the server and this
 * module only picks the words. Attention is carried by the words too, never by
 * colour alone.
 */
export type ReconciliationAgeView = { text: string; attention: boolean };

/** Whole days the window accepts; the server validates the same bounds. */
export const reconciliationWindowBounds = { min: 1, max: 3650 } as const;

type Age = Pick<AccountReconciliation['commodities'][number], 'state' | 'last_statement_date'>;

/**
 * An account in several currencies is judged per currency (a statement covers
 * one), so its words name each currency rather than letting a reconciled one
 * stand for the rest (#594 review).
 */
export function reconciliationAgeView(age: AccountReconciliation, locale: string): ReconciliationAgeView {
  if (age.commodities.length > 1) {
    const parts = age.commodities.map(commodity => ({ code: commodity.commodity_code, view: singleAgeView(commodity, locale) }));
    return {
      text: parts.map(part => m.reconciliation_age_commodity({ code: part.code, state: part.view.text })).join(' · '),
      attention: parts.some(part => part.view.attention)
    };
  }
  return singleAgeView(age, locale);
}

function singleAgeView(age: Age, locale: string): ReconciliationAgeView {
  const date = age.last_statement_date
    ? new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parseISO(age.last_statement_date))
    : '';
  switch (age.state) {
    case 'reconciled':
      return { text: m.reconciliation_age_reconciled({ date }), attention: false };
    case 'out_of_date':
      return { text: m.reconciliation_age_out_of_date({ date }), attention: true };
    case 'never_reconciled':
      return { text: m.reconciliation_age_never(), attention: true };
    case 'nothing_to_reconcile':
      return { text: m.reconciliation_age_nothing(), attention: false };
  }
}

/** A whole number of days within the bounds, or null when the input is not one. */
export function parseReconciliationWindow(input: string): number | null {
  const trimmed = input.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const days = Number(trimmed);
  return days >= reconciliationWindowBounds.min && days <= reconciliationWindowBounds.max ? days : null;
}
