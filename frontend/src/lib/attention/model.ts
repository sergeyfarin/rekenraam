import type { Attention, AttentionSource, AttentionSourceID } from '#lib/api/attention.ts';
import { m } from '#lib/paraglide/messages.js';

export type AttentionState = {
  data?: Attention;
  isPending: boolean;
  isError: boolean;
  isFetching: boolean;
  refetch: () => unknown;
};

export type AttentionBadge = { count: number; text: string; label: string };

export function attentionSummary(query: AttentionState, locale: string): AttentionBadge {
  const count = query.data?.total_known ?? 0;
  const formatted = new Intl.NumberFormat(locale).format(count);
  const failed = query.isError || query.data?.has_failures;
  const incomplete = query.isPending || !query.data?.complete;
  return {
    count,
    text: failed ? '!' : incomplete ? '…' : formatted,
    label: failed ? m.attention_failed({ count: formatted }) : incomplete
      ? m.attention_checking({ count: formatted }) : m.attention_count({ count: formatted })
  };
}

/** A badge never turns a failed or unknown source into an empty count. */
export function sourceBadge(query: AttentionState, ids: readonly AttentionSourceID[], locale: string): AttentionBadge {
  const sources = ids.map(id => query.data?.sources.find(source => source.id === id));
  const count = sources.reduce((total, source) => total + (source?.count ?? 0), 0);
  const failed = query.isError || sources.some(source => source?.state === 'error' || source?.failed);
  const incomplete = query.isPending || sources.some(source => !source || source.state !== 'ready');
  const formatted = new Intl.NumberFormat(locale).format(count);
  return {
    count,
    text: failed ? '!' : incomplete ? '…' : count > 0 ? formatted : '',
    label: failed ? m.attention_failed({ count: formatted }) : incomplete
      ? m.attention_checking({ count: formatted }) : m.attention_count({ count: formatted })
  };
}

export const sourceDetails: Record<AttentionSourceID, { title: () => string; href: string }> = {
  drafts: { title: m.attention_drafts, href: '/app/planning/recurring' },
  transaction_review: { title: m.attention_transactions, href: '/app/ledger/transactions?needs_review=true' },
  recurring_blocked: { title: m.attention_recurring, href: '/app/planning/recurring' },
  imports: { title: m.attention_imports, href: '/app/ledger/import' },
  suggestions: { title: m.attention_suggestions, href: '/app/investments?tab=suggestions' },
  connections: { title: m.attention_connections, href: '/app/ledger/import?tab=connections' },
  refresh: { title: m.attention_refresh, href: '/app/settings/currencies' },
  backups: { title: m.attention_backups, href: '/app/settings/data' },
  self_check: { title: m.attention_self_check, href: '/app/settings/data' },
  prices: { title: m.attention_prices, href: '/app/investments/prices' },
  rates: { title: m.attention_rates, href: '/app/settings/currencies' }
};

export function sourceStatus(source: AttentionSource, locale: string): string {
  if (source.state === 'error') return m.attention_source_error();
  if (source.state === 'loading') return source.count === null ? m.attention_source_running() : m.attention_checking({ count: new Intl.NumberFormat(locale).format(source.count) });
  if (source.status === 'never') return m.attention_source_never();
  const count = new Intl.NumberFormat(locale).format(source.count ?? 0);
  return source.failed ? m.attention_failed({ count }) : m.attention_count({ count });
}
