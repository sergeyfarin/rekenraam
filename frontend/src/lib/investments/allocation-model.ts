import { parseDate } from '@internationalized/date';
import type { AllocationPoint, AllocationRevision } from '#lib/api/allocation.ts';
import { parseDecimalAmount } from '#lib/money/amount.ts';

export function parseTargetUnits(input: string, locale: string): number | null {
  const parsed = parseDecimalAmount(input, { maxScale: 4, locale });
  if (!parsed || parsed.value.startsWith('-')) return null;
  const units = BigInt(parsed.value) * 10n ** BigInt(4 - parsed.scale);
  return units <= 1000000n ? Number(units) : null;
}

export function allocationIDs(points: AllocationPoint[], targets: AllocationRevision['targets']): number[] {
  return [...new Set([...points.flatMap(p => p.rows.map(r => r.commodity_id)), ...targets.map(t => t.commodity_id)])]
    .sort((a, b) => a === 0 ? 1 : b === 0 ? -1 : a - b);
}

export function calendarFraction(date: string, from: string, to: string): number {
  const julian = (value: string) => { const date = parseDate(value); return date.calendar.toJulianDay(date); };
  const first = julian(from);
  const last = julian(to);
  return last === first ? 0.5 : (julian(date) - first) / (last - first);
}

type StackPoint = { x: number; lower: number; upper: number };
export function allocationAreas(points: AllocationPoint[], ids: number[]) {
  const from = points[0]?.date;
  const to = points.at(-1)?.date;
  if (!from || !to) return [];
  // Only geometry uses Number. All financial amounts/weights/drift came from Go.
  const areas = ids.map(id => ({ id, segments: [] as StackPoint[][] }));
  const runs = ids.map(() => [] as StackPoint[]);
  const flush = () => { runs.forEach((run, i) => { if (run.length) areas[i].segments.push(run); runs[i] = []; }); };
  for (const point of points) {
    if (point.chart_reason) { flush(); continue; }
    const shares = new Map(point.rows.filter(r => r.side === 'long').map(r => [r.commodity_id, Number(r.weight ?? 0)]));
    let lower = 0;
    const x = calendarFraction(point.date, from, to);
    ids.forEach((id, i) => {
      const upper = lower + (shares.get(id) ?? 0);
      runs[i].push({ x, lower, upper });
      lower = upper;
    });
  }
  flush();
  return areas;
}

export function savedCalendarDay(timestamp: string, timeZone: string): string {
  // Saved changes are UTC system timestamps, not financial calendar dates.
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(timestamp));
  const part = (type: string) => parts.find(p => p.type === type)?.value ?? '';
  return `${part('year')}-${part('month')}-${part('day')}`;
}

export function allocationColor(index: number): string {
  return `var(--color-chart-${index % 5 + 1})`;
}
