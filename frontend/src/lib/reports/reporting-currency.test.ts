import { describe, it, expect } from 'vitest';
import {
  conversionAddsInformation,
  parseReportingCurrency,
  valuationNotice,
  withReportingCurrency,
  type Valuation
} from './reporting-currency';

const baseValuation: Valuation = {
  commodity_id: 2,
  code: 'EUR',
  scale: 2,
  method: 'observed_on_or_before',
  max_staleness_days: 7,
  complete: true,
  rates_used: [],
  gaps: []
};

describe('parseReportingCurrency', () => {
  it('reads a selection', () => {
    expect(parseReportingCurrency(new URLSearchParams('reporting_currency_id=7'))).toBe(7);
  });

  it('treats a missing or malformed value as no selection', () => {
    // A stale or hand-edited link still renders the report it mostly
    // describes, the same rule the ID filters follow.
    expect(parseReportingCurrency(new URLSearchParams(''))).toBeNull();
    expect(parseReportingCurrency(new URLSearchParams('reporting_currency_id=abc'))).toBeNull();
    expect(parseReportingCurrency(new URLSearchParams('reporting_currency_id=0'))).toBeNull();
    expect(parseReportingCurrency(new URLSearchParams('reporting_currency_id=-3'))).toBeNull();
  });
});

describe('withReportingCurrency', () => {
  it('adds, replaces, and removes without disturbing the rest of the query', () => {
    const params = new URLSearchParams('view=spending&account_id=1&account_id=2');

    expect(withReportingCurrency(params, 5).toString()).toBe(
      'view=spending&account_id=1&account_id=2&reporting_currency_id=5'
    );
    expect(withReportingCurrency(withReportingCurrency(params, 5), 9).get('reporting_currency_id')).toBe('9');
    expect(withReportingCurrency(withReportingCurrency(params, 5), null).has('reporting_currency_id')).toBe(false);
    // The original is untouched: callers pass URL state around.
    expect(params.has('reporting_currency_id')).toBe(false);
  });
});

describe('valuationNotice', () => {
  it('says nothing when there is nothing to say', () => {
    expect(valuationNotice(undefined)).toEqual({ kind: 'none' });
    expect(valuationNotice(baseValuation)).toEqual({ kind: 'none' });
  });

  it('reports a rate that came from another day', () => {
    const notice = valuationNotice({
      ...baseValuation,
      rates_used: [
        { commodity_id: 1, observation_date: '2026-06-01', requested_date: '2026-06-05', stale: true, derived: false },
        { commodity_id: 3, observation_date: '2026-06-04', requested_date: '2026-06-04', stale: false, derived: false }
      ]
    });
    expect(notice).toEqual({ kind: 'stale', dates: ['2026-06-01'] });
  });

  it('reports a missing figure ahead of a stale one', () => {
    // An absent figure is the more serious of the two: a reader looking at a
    // total that silently omits a commodity is reading a wrong number, while a
    // stale rate is a right number from a nearby day.
    const notice = valuationNotice({
      ...baseValuation,
      complete: false,
      rates_used: [{ commodity_id: 1, observation_date: '2026-06-01', requested_date: '2026-06-05', stale: true, derived: false }],
      gaps: [{ commodity_id: 4, reason: 'no_observation_in_window' }]
    });
    expect(notice).toEqual({ kind: 'incomplete', commodityIDs: [4] });
  });
});

describe('conversionAddsInformation', () => {
  it('is nothing to show without a selection', () => {
    expect(conversionAddsInformation([1, 2], null)).toBe(false);
  });

  it('is nothing to show when the report is already in that currency', () => {
    expect(conversionAddsInformation([7], 7)).toBe(false);
  });

  it('shows a report held in another currency', () => {
    expect(conversionAddsInformation([7], 3)).toBe(true);
  });

  it('shows a report spanning several commodities, one of them the reporting one', () => {
    expect(conversionAddsInformation([7, 3], 7)).toBe(true);
  });

  // An empty report has no rows to duplicate, but the notice about a missing
  // rate still belongs on screen — that is often *why* it is empty.
  it('shows an empty report', () => {
    expect(conversionAddsInformation([], 7)).toBe(true);
  });
});
