import type { components } from '$lib/api/schema';

/**
 * The reporting currency, read from and written to the URL like every other
 * report control.
 *
 * It is deliberately not part of `ReportFilterState`: a filter narrows what a
 * report is about, and this changes nothing about that. Every exact
 * per-commodity figure is present whether or not a reporting currency is
 * chosen — the conversion is additive, and so is this parameter.
 */

export const REPORTING_CURRENCY_PARAM = 'reporting_currency_id';

/**
 * Reads the reporting currency from a URL, or null when none is selected.
 *
 * A malformed value reads as "none" rather than as an error: a stale or
 * hand-edited link should still render the report it mostly describes, which
 * is the same rule the ID filters follow.
 */
export function parseReportingCurrency(params: URLSearchParams): number | null {
  const raw = params.get(REPORTING_CURRENCY_PARAM);
  if (raw === null) return null;
  const value = Number(raw);
  return Number.isInteger(value) && value > 0 ? value : null;
}

/** Writes the selection into a copy of the URL parameters. */
export function withReportingCurrency(
  params: URLSearchParams,
  commodityID: number | null
): URLSearchParams {
  const next = new URLSearchParams(params);
  if (commodityID === null) {
    next.delete(REPORTING_CURRENCY_PARAM);
  } else {
    next.set(REPORTING_CURRENCY_PARAM, String(commodityID));
  }
  return next;
}

/**
 * The valuation block a converted response carries.
 *
 * Taken from the generated schema rather than restated, so a field that changes
 * shape upstream breaks the type check here instead of reading as `undefined`.
 */
export type Valuation = components['schemas']['Valuation'];

/**
 * What a screen should tell the reader about a set of converted figures.
 *
 * Three states, because they call for three different things: nothing to say,
 * a rate that came from another day (the figure is there, with a caveat), and
 * a commodity with no usable rate at all (a figure is missing, and the reader
 * needs to know which one rather than wondering why a total looks small).
 */
export type ValuationNotice =
  | { kind: 'none' }
  | { kind: 'stale'; dates: string[] }
  | { kind: 'incomplete'; commodityIDs: number[] };

export function valuationNotice(valuation: Valuation | undefined): ValuationNotice {
  if (!valuation) return { kind: 'none' };

  if (!valuation.complete) {
    return {
      kind: 'incomplete',
      commodityIDs: valuation.gaps.map((gap) => gap.commodity_id)
    };
  }

  const stale = valuation.rates_used.filter((use) => use.stale).map((use) => use.observation_date);
  if (stale.length > 0) {
    return { kind: 'stale', dates: [...new Set(stale)].sort() };
  }

  return { kind: 'none' };
}

/**
 * Whether converting adds anything to a report that already holds these
 * commodities.
 *
 * A book whose whole report is already in the reporting currency converts every
 * figure to itself at 1, and the restatement would be a second copy of each row
 * saying the same thing. The selection stays in the URL either way — a range
 * that later includes a second currency starts showing the conversion again
 * without the reader having to re-pick it.
 */
export function conversionAddsInformation(
  commodityIDs: number[],
  reportingCurrencyID: number | null
): boolean {
  if (reportingCurrencyID === null) return false;
  const distinct = new Set(commodityIDs);
  return !(distinct.size === 1 && distinct.has(reportingCurrencyID));
}
