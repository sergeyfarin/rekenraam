// Reports use a hard-coded scale of 100 (cents) for display, since all the
// report rows come from the API pre-aggregated against the book's default
// currency. If we ever surface mixed-commodity rows, the call sites need to
// switch to formatMinorWithScale from $lib/money.
export function formatCurrencyMinor(minor: number, scale = 100): string {
  const value = minor / scale;
  return value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

export function formatDate(dateStr: string): string {
  if (!dateStr) return "—";
  const date = new Date(dateStr);
  return date.toLocaleDateString();
}

export function formatPeriod(periodStart: string, groupBy: string): string {
  if (!periodStart) return "—";
  const date = new Date(periodStart);
  if (groupBy === "year") return date.getFullYear().toString();
  if (groupBy === "quarter") {
    const q = Math.ceil((date.getMonth() + 1) / 3);
    return `Q${q} ${date.getFullYear()}`;
  }
  if (groupBy === "month") {
    return date.toLocaleDateString(undefined, { year: "numeric", month: "short" });
  }
  return date.toLocaleDateString();
}
