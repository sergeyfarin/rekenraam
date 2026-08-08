/**
 * URL-addressable filter state for the reports screen.
 *
 * The URL is the source of truth, per docs/plans/reports-plan.md: changing a
 * control updates the URL, and the typed query follows from that. A report a
 * user is looking at must therefore always be reachable by pasting the link —
 * which is also what makes an exported or printed report explainable later.
 *
 * Everything here is pure so it can be unit-tested without a browser; the
 * screen component owns navigation and nothing else.
 */

export const REPORT_VIEWS = ['net-worth', 'spending', 'cashflow'] as const;
export type ReportView = (typeof REPORT_VIEWS)[number];

export const REPORT_BUCKETS = ['day', 'week', 'month', 'quarter', 'year'] as const;
export type ReportBucket = (typeof REPORT_BUCKETS)[number];

export const SPENDING_GROUP_BYS = ['category', 'payee'] as const;
export type SpendingGroupBy = (typeof SPENDING_GROUP_BYS)[number];

export const SPENDING_DIRECTIONS = ['expense', 'income'] as const;
export type SpendingDirection = (typeof SPENDING_DIRECTIONS)[number];

export interface ReportFilters {
  view: ReportView;
  startDate: string;
  endDate: string;
  bucket: ReportBucket;
  groupBy: SpendingGroupBy;
  direction: SpendingDirection;
}

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

function isMember<T extends string>(values: readonly T[], value: string | null): value is T {
  return value !== null && (values as readonly string[]).includes(value);
}

/**
 * Today as an ISO calendar date in the viewer's own zone.
 *
 * `en-CA` is used purely because its date format *is* ISO 8601; this is a
 * formatting trick, not a locale choice, and it avoids `toISOString()`, which
 * would return the UTC day and so show the wrong "today" either side of
 * midnight for most of the world.
 */
export function todayISO(now: Date = new Date()): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).formatToParts(now);
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((candidate) => candidate.type === type)?.value ?? '';
  return `${part('year')}-${part('month')}-${part('day')}`;
}

/** Year-to-date by month — the preset the screen opens on. */
export function defaultFilters(now: Date = new Date()): ReportFilters {
  const endDate = todayISO(now);
  return {
    view: 'net-worth',
    startDate: `${endDate.slice(0, 4)}-01-01`,
    endDate,
    bucket: 'month',
    groupBy: 'category',
    direction: 'expense'
  };
}

/**
 * Read filters out of a URL's query string.
 *
 * Returns `null` when the dates are missing or malformed, which is the screen's
 * signal to redirect to the resolved default preset rather than silently
 * reporting over a range the user never chose. The non-date parameters fall
 * back to their defaults instead: an unrecognised `bucket` is a bad link, not a
 * reason to refuse to show net worth at all.
 */
export function parseReportFilters(search: URLSearchParams, now: Date = new Date()): ReportFilters | null {
  const startDate = search.get('start_date') ?? '';
  const endDate = search.get('end_date') ?? '';
  if (!ISO_DATE.test(startDate) || !ISO_DATE.test(endDate)) {
    return null;
  }
  // An inverted range is rejected here too. The backend also rejects it, but
  // failing at the URL boundary shows the user a filter form they can fix
  // rather than an error panel.
  if (startDate > endDate) {
    return null;
  }

  const defaults = defaultFilters(now);
  const view = search.get('view');
  const bucket = search.get('bucket');
  const groupBy = search.get('group_by');
  const direction = search.get('direction');

  return {
    view: isMember(REPORT_VIEWS, view) ? view : defaults.view,
    startDate,
    endDate,
    bucket: isMember(REPORT_BUCKETS, bucket) ? bucket : defaults.bucket,
    groupBy: isMember(SPENDING_GROUP_BYS, groupBy) ? groupBy : defaults.groupBy,
    direction: isMember(SPENDING_DIRECTIONS, direction) ? direction : defaults.direction
  };
}

/**
 * Render filters back into a query string.
 *
 * Every filter is always written, including ones the active view ignores, so
 * that switching views preserves the settings the user already chose rather
 * than resetting them — the "a category/payee switch preserves compatible
 * filters and changes only group_by" rule in the plan.
 */
export function reportFiltersToSearch(filters: ReportFilters): string {
  return new URLSearchParams({
    view: filters.view,
    start_date: filters.startDate,
    end_date: filters.endDate,
    bucket: filters.bucket,
    group_by: filters.groupBy,
    direction: filters.direction
  }).toString();
}

export function reportFiltersToHref(filters: ReportFilters): string {
  return `/app/reports?${reportFiltersToSearch(filters)}`;
}
