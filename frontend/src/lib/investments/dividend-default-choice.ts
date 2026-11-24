type AccountLike = { id: number; status?: string };

/**
 * The account a stored dividend default may fill a field with (#664 review).
 * A default is a stored automation target, so like the server it is applied
 * only while its account is not archived today (`today` is the owner-day
 * account list). It must also be an option the dated picker offers, or the
 * field would look blank while holding an ID. Returns '' when it may not.
 * Choosing an account by hand still offers every account eligible on the date.
 */
export function defaultAccountToApply(id: number | null | undefined, offered: readonly AccountLike[], today: readonly AccountLike[]): string {
  if (id === null || id === undefined) return '';
  if (!offered.some((account) => account.id === id)) return '';
  const current = today.find((account) => account.id === id);
  if (current === undefined || current.status === 'archived') return '';
  return String(id);
}

/** A held selection that is not one of the offered options; '' is no selection. */
export function heldButUnavailable(id: string, offered: readonly AccountLike[]): boolean {
  return id !== '' && !offered.some((account) => String(account.id) === id);
}

/**
 * Whether an account a default filled in may still be sent: it must be in
 * the owner-day list and not archived there. A default that stops applying
 * is cleared, not sent as if the owner had chosen it (#664 review).
 */
export function defaultStillApplies(id: string, today: readonly AccountLike[]): boolean {
  const current = today.find((account) => String(account.id) === id);
  return current !== undefined && current.status !== 'archived';
}

type DefaultLike = {
  id: number;
  commodity_id?: number | null;
  status: string;
  effective_from: string;
  effective_to?: string | null;
};

/**
 * The stored dividend default that applies to a commodity on a date, by the
 * server's own precedence (ResolveDividendDefault): active, in effect on the
 * date, the commodity's own before the global one, then the latest effective
 * date, then the newest. A backdated entry thus shows the default the server
 * will check, not today's (#664 review).
 */
export function applicableDividendDefault<T extends DefaultLike>(defaults: readonly T[], commodityID: number, date: string): T | undefined {
  return defaults
    .filter((d) => d.status === 'active' && (d.commodity_id === commodityID || d.commodity_id === undefined || d.commodity_id === null))
    .filter((d) => d.effective_from <= date && (!d.effective_to || d.effective_to >= date))
    .sort((a, b) =>
      Number(a.commodity_id !== commodityID) - Number(b.commodity_id !== commodityID) ||
      b.effective_from.localeCompare(a.effective_from) ||
      b.id - a.id
    )[0];
}
