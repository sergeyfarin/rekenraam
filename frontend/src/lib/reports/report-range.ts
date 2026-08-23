/**
 * The report date range and bucket, read from and repaired in the URL.
 *
 * This lives beside `report-filters.ts` and follows the same rule: the URL is
 * the source of truth, and anything a module does not own it leaves alone. The
 * range is repaired in place over the existing parameters rather than rebuilt,
 * because a link that arrives with a view, a grouping, and an account selection
 * but a missing `bucket` still asks a specific question — dropping to the
 * default net-worth report would answer a different one.
 */

export type ReportBucket = 'day' | 'week' | 'month' | 'quarter' | 'year';

export type ReportRange = {
  startDate: string;
  endDate: string;
  bucket: ReportBucket;
};

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;
const BUCKETS: ReportBucket[] = ['day', 'week', 'month', 'quarter', 'year'];

function isBucket(value: string | null): value is ReportBucket {
  return value !== null && (BUCKETS as string[]).includes(value);
}

/** The range a report falls back to: this year to date, bucketed by month. */
export function defaultReportRange(today: string): ReportRange {
  return { startDate: `${today.slice(0, 4)}-01-01`, endDate: today, bucket: 'month' };
}

/**
 * Reads the range, or null when any of its three parts is missing or malformed.
 * Null means "repair the URL first", never "show an error": a report is only
 * requested once its range is unambiguous.
 */
export function parseReportRange(params: URLSearchParams): ReportRange | null {
  const startDate = params.get('start_date') ?? '';
  const endDate = params.get('end_date') ?? '';
  const bucket = params.get('bucket');
  if (!ISO_DATE.test(startDate) || !ISO_DATE.test(endDate) || !isBucket(bucket)) {
    return null;
  }
  return { startDate, endDate, bucket };
}

/**
 * Fills in only the parts of the range that are missing or malformed, and
 * returns every other parameter untouched. A valid range returns null, so the
 * caller can skip a redundant navigation.
 */
export function repairReportRange(params: URLSearchParams, today: string): URLSearchParams | null {
  if (parseReportRange(params) !== null) {
    return null;
  }
  const defaults = defaultReportRange(today);
  const next = new URLSearchParams(params);
  if (!ISO_DATE.test(next.get('start_date') ?? '')) {
    next.set('start_date', defaults.startDate);
  }
  if (!ISO_DATE.test(next.get('end_date') ?? '')) {
    next.set('end_date', defaults.endDate);
  }
  if (!isBucket(next.get('bucket'))) {
    next.set('bucket', defaults.bucket);
  }
  return next;
}
