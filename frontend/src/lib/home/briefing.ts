import { parseISO } from 'date-fns';
import type { AttentionSource, AttentionSourceID } from '#lib/api/attention.ts';
import type { Home, HomeHeadline, HomeValuationInput } from '#lib/api/home.ts';
import type { AttentionState } from '#lib/attention/model.ts';
import { formatHomeMoney, hiddenAmount, joinCommodityAmount } from '#lib/money/format.ts';
import { amountPrivacy } from '#lib/money/privacy.svelte.ts';
import { m } from '#lib/paraglide/messages.js';

/**
 * Home's two-line briefing (#476), as text and links. The design's rules live
 * here, in one testable place: which headline shows, what the status line
 * leads with, and what becomes a count.
 */

export type HeadlineID = HomeHeadline['id'];
/** `auto` is the default: net worth, falling back to cash while it is unavailable. */
export type HeadlineChoice = 'auto' | HeadlineID;

export type BriefingHeadline = {
  id: HeadlineID;
  label: string;
  /** Null when the owner chose this headline and it is unavailable. */
  amount: string | null;
  change: { text: string; sign: -1 | 0 | 1; label: string } | null;
};

export type HeadlineOption = { id: HeadlineID; label: string; scope: string };

export type StatusLink = { text: string; href: string; label?: string };
export type BriefingStatus = {
  tone: 'neutral' | 'attention' | 'failed';
  lead: StatusLink;
  more: StatusLink | null;
};

const reviewHref = '/app/review';

function headlineFor(home: Home, id: HeadlineID): HomeHeadline | undefined {
  return home.headlines.find(headline => headline.id === id);
}

function headlineLabel(id: HeadlineID): string {
  return id === 'cash' ? m.briefing_headline_cash() : m.briefing_headline_net_worth();
}

/** The headline to show. Auto never shows an unavailable figure: it falls back
 * to cash, and with no cash figure either only the status line shows. */
export function shownHeadlineID(home: Home, choice: HeadlineChoice): HeadlineID | null {
  if (choice !== 'auto') return headlineFor(home, choice) ? choice : null;
  if (headlineFor(home, 'net_worth')?.value) return 'net_worth';
  if (headlineFor(home, 'cash')?.value) return 'cash';
  return null;
}

export function headlineOptions(home: Home, locale: string): HeadlineOption[] {
  return home.headlines
    .filter(headline => headline.account_count > 0)
    .map(headline => {
      const n = new Intl.NumberFormat(locale).format(headline.account_count);
      const count = headline.account_count;
      return {
        id: headline.id,
        label: headlineLabel(headline.id),
        scope: headline.id === 'cash' ? m.briefing_scope_cash({ count, n }) : m.briefing_scope_all({ count, n })
      };
    });
}

function monthName(date: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { month: 'short' }).format(parseISO(date));
}

function dayName(date: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parseISO(date));
}

export function briefingHeadline(home: Home, id: HeadlineID, locale: string): BriefingHeadline | null {
  const headline = headlineFor(home, id);
  const currency = home.reporting_currency;
  if (!headline) return null;
  const label = headlineLabel(id);
  if (!headline.value || !currency) return { id, label, amount: null, change: null };
  const money = (value: string, scale: number) => joinCommodityAmount(currency.symbol || currency.code, formatHomeMoney(value, scale, currency.scale, locale));
  const amount = value(headline.value.quantity_value, headline.value.quantity_scale, money);
  let change: BriefingHeadline['change'] = null;
  if (headline.change) {
    const raw = headline.change.quantity_value;
    const sign = raw.startsWith('-') ? -1 : /^0+$/.test(raw) ? 0 : 1;
    const magnitude = sign < 0 ? raw.slice(1) : raw;
    const shown = value(magnitude, headline.change.quantity_scale, money);
    const signed = amountPrivacy.hidden ? shown : `${sign < 0 ? '−' : sign > 0 ? '+' : '±'}${shown}`;
    change = {
      text: m.briefing_change({ amount: signed, month: monthName(home.as_of, locale) }),
      // Hidden amounts are neutral: colour would still reveal the direction.
      sign: amountPrivacy.hidden ? 0 : sign,
      label: m.briefing_change_label({ amount: signed, start: dayName(home.start_boundary, locale), end: dayName(home.as_of, locale) })
    };
  }
  return { id, label, amount, change };
}

function value(coefficient: string, scale: number, money: (value: string, scale: number) => string): string {
  return amountPrivacy.hidden ? hiddenAmount : money(coefficient, scale);
}

/** One named input, several of one kind counted by kind, mixed kinds as values. */
function valuationText(inputs: HomeValuationInput[], style: 'needs' | 'missing' | 'stale', locale: string): string {
  const count = inputs.length;
  const n = new Intl.NumberFormat(locale).format(count);
  const kinds = new Set(inputs.map(input => input.kind));
  if (count === 1) {
    const code = inputs[0].code;
    const rate = inputs[0].kind === 'rate';
    if (style === 'needs') return rate ? m.briefing_needs_rate_named({ code }) : m.briefing_needs_price_named({ code });
    if (style === 'missing') return rate ? m.briefing_missing_rate_named({ code }) : m.briefing_missing_price_named({ code });
    return rate ? m.briefing_stale_rate_named({ code }) : m.briefing_stale_price_named({ code });
  }
  const kind = kinds.size > 1 ? 'values' : kinds.has('rate') ? 'rates' : 'prices';
  const messages = {
    needs: { prices: m.briefing_needs_prices, rates: m.briefing_needs_rates, values: m.briefing_needs_values },
    missing: { prices: m.briefing_missing_prices, rates: m.briefing_missing_rates, values: m.briefing_missing_values },
    stale: { prices: m.briefing_stale_prices, rates: m.briefing_stale_rates, values: m.briefing_stale_values }
  } as const;
  return messages[style][kind]({ count, n });
}

/** Prices for anything but a currency; a rate-only problem is fixed on Currencies. */
function valuationHref(inputs: HomeValuationInput[], status: 'missing' | 'stale'): string {
  if (inputs.every(input => input.kind === 'rate')) return '/app/settings/currencies';
  return `/app/investments/prices?status=${status}`;
}

const workSources: readonly AttentionSourceID[] = ['drafts', 'transaction_review', 'recurring_blocked', 'imports', 'suggestions', 'prices', 'rates'];

function workText(source: AttentionSource, locale: string): string {
  const count = source.count ?? 0;
  const n = new Intl.NumberFormat(locale).format(count);
  switch (source.id) {
    case 'drafts': return m.briefing_work_drafts({ count, n });
    case 'transaction_review': return m.briefing_work_transactions({ count, n });
    case 'recurring_blocked': return m.briefing_work_recurring({ count, n });
    case 'imports': return m.briefing_work_imports({ count, n });
    case 'suggestions': return m.briefing_work_suggestions({ count, n });
    case 'rates': return m.briefing_work_rates({ count, n });
    default: return m.briefing_work_prices({ count, n });
  }
}

function failureText(source: AttentionSource, locale: string): string {
  if (source.state === 'error') return m.briefing_failed_unchecked();
  const count = source.count ?? 1;
  const n = new Intl.NumberFormat(locale).format(count);
  switch (source.id) {
    case 'backups': return m.briefing_failed_backup();
    case 'self_check': return m.briefing_failed_self_check();
    case 'connections': return m.briefing_failed_connections({ count, n });
    case 'refresh': return m.briefing_failed_refresh({ count, n });
    case 'imports': return m.briefing_failed_imports({ count, n });
    default: return m.briefing_failed_other();
  }
}

function more(count: number, locale: string): StatusLink | null {
  if (count <= 0) return null;
  const n = new Intl.NumberFormat(locale).format(count);
  return { text: m.briefing_more({ count, n }), href: reviewHref, label: m.briefing_more_label({ count, n }) };
}

function sourceCount(sources: readonly AttentionSource[], ids: readonly AttentionSourceID[]): number {
  return sources.filter(source => ids.includes(source.id)).reduce((total, source) => total + (source.count ?? 0), 0);
}

/**
 * The status line. Failures lead, then a valuation problem (missing outranks
 * out of date), then the work to review; whatever does not lead becomes one
 * "N more" count. "All clear" only after a complete, successful check.
 */
export function briefingStatus(home: Home | undefined, shown: HeadlineID | null, choice: HeadlineChoice, attention: AttentionState, locale: string): BriefingStatus {
  const data = attention.data;
  if (attention.isError && !data) {
    return { tone: 'failed', lead: { text: m.briefing_check_failed(), href: reviewHref }, more: null };
  }
  const sources = data?.sources ?? [];
  const total = data?.total_known ?? 0;

  const failed = sources.find(source => source.state === 'error' || source.failed);
  if (failed) {
    return { tone: 'failed', lead: { text: failureText(failed, locale), href: reviewHref }, more: more(total - (failed.count ?? 0), locale) };
  }

  const netWorth = home?.headlines.find(headline => headline.id === 'net_worth');
  if (home && netWorth && !home.empty_book) {
    const rest = more(total - sourceCount(sources, ['prices', 'rates']), locale);
    if (netWorth.unavailable === 'no_reporting_currency') {
      return { tone: 'attention', lead: { text: m.briefing_needs_currency(), href: '/app/settings/currencies' }, more: more(total, locale) };
    }
    if (netWorth.unavailable === 'unclassified_short') {
      return { tone: 'attention', lead: { text: m.briefing_needs_short_review(), href: '/app/reports/net-worth' }, more: more(total, locale) };
    }
    if (netWorth.missing.length > 0) {
      // A chosen headline that needs no price is never replaced; the line then
      // reports the missing inputs without blaming the headline.
      const style = choice === 'cash' && shown === 'cash' ? 'missing' : 'needs';
      return { tone: 'attention', lead: { text: valuationText(netWorth.missing, style, locale), href: valuationHref(netWorth.missing, 'missing') }, more: rest };
    }
    if (netWorth.stale.length > 0) {
      return { tone: 'attention', lead: { text: valuationText(netWorth.stale, 'stale', locale), href: valuationHref(netWorth.stale, 'stale') }, more: rest };
    }
  }

  const work = sources.filter(source => workSources.includes(source.id) && (source.count ?? 0) > 0);
  if (work.length === 2 && total === sourceCount(work, work.map(source => source.id))) {
    return { tone: 'attention', lead: { text: m.briefing_work_two({ first: workText(work[0], locale), second: workText(work[1], locale) }), href: reviewHref }, more: null };
  }
  if (work.length > 0) {
    return { tone: 'attention', lead: { text: m.briefing_work_one({ item: workText(work[0], locale) }), href: reviewHref }, more: more(total - (work[0].count ?? 0), locale) };
  }
  if (!data || attention.isPending || !data.complete) {
    return { tone: 'neutral', lead: { text: m.briefing_checking(), href: reviewHref }, more: null };
  }
  if (total > 0) {
    return { tone: 'attention', lead: { text: m.briefing_review_count({ count: total, n: new Intl.NumberFormat(locale).format(total) }), href: reviewHref }, more: null };
  }
  return { tone: 'neutral', lead: { text: m.briefing_all_clear(), href: reviewHref }, more: null };
}
