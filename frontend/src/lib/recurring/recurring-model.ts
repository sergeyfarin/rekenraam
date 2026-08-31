import { addDays, parseISO } from 'date-fns';
import { m } from '$lib/paraglide/messages.js';
import type { RecurringTemplate, RecurringTemplatePatch } from '$lib/api/recurring';
import type { TransactionRequest } from '$lib/api/transactions';

export const scheduleKeys = ['frequency', 'interval_count', 'by_weekday', 'day_of_month', 'last_day_of_month', 'month_of_year', 'starts_on', 'ends_on', 'max_occurrences'] as const;

export function templateValues(template: RecurringTemplate | undefined, today: string): TransactionRequest {
  return {
    transaction_date: today,
    transaction_kind: template?.transaction_kind ?? 'ordinary',
    payee_id: template?.payee_id ?? undefined,
    payee_name: template?.payee_name ?? '', description: template?.description ?? '',
    note_markdown: template?.note_markdown ?? '', tag_ids: [...(template?.tag_ids ?? [])],
    journal_entries: template ? [{ entry_date: today, postings: template.postings.map(p => ({ ...p })) }] : []
  };
}

export function templatePatch(schedule: RecurringTemplatePatch, entry: TransactionRequest, current?: RecurringTemplate): RecurringTemplatePatch {
  const entries = entry.journal_entries ?? [];
  if (entries.length !== 1 || (entries[0].postings?.length ?? 0) < 2) throw new Error(m.recurring_incomplete());
  const patch: RecurringTemplatePatch = {
    ...schedule, transaction_kind: entry.transaction_kind === 'transfer' ? 'transfer' : 'ordinary',
    payee_id: entry.payee_id ?? null, payee_name: entry.payee_name ?? '',
    description: entry.description ?? '', note_markdown: entry.note_markdown ?? '', tag_ids: entry.tag_ids ?? [],
    postings: entries[0].postings!.map(p => {
      if (!p.account_id || !p.commodity_id || p.quantity_value === undefined || p.quantity_scale === undefined) throw new Error(m.recurring_incomplete());
      return { line_key: p.line_key, account_id: p.account_id, commodity_id: p.commodity_id, quantity_value: p.quantity_value, quantity_scale: p.quantity_scale, memo: p.memo ?? '' };
    })
  };
  // Sending unchanged schedule fields resets the catch-up watermark. Preserve
  // omission semantics so a name/amount edit never skips downtime catch-up.
  if (current) for (const key of scheduleKeys) if (patch[key] === current[key]) delete patch[key];
  return patch;
}

export function displayDate(date: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parseISO(date));
}
export function weekdayLabel(day: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { weekday: 'long' }).format(addDays(parseISO('2026-08-30'), day));
}
export function scheduleLabel(template: RecurringTemplate, locale: string): string {
  const count = new Intl.NumberFormat(locale).format(template.interval_count);
  const day = template.last_day_of_month ? m.recurring_last_day() : new Intl.NumberFormat(locale).format(template.day_of_month ?? 1);
  switch (template.frequency) {
    case 'daily': return m.recurring_daily_label({ count });
    case 'weekly': return m.recurring_weekly_label({ count, weekday: weekdayLabel(template.by_weekday ?? 0, locale) });
    case 'monthly': return m.recurring_monthly_label({ count, day });
    case 'yearly': return m.recurring_yearly_label({ count, day, month: new Intl.NumberFormat(locale).format(template.month_of_year ?? 1) });
  }
}
