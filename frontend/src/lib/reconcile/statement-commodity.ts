/**
 * Which commodity a new reconciliation is for (#594 review). A statement
 * covers one commodity, and an account can hold several: its default
 * currency and every commodity the server's reconciliation judgement found
 * activity or a statement in. The owner picks among them; Home's link names
 * the one it flagged, so the form opens on it rather than on a default the
 * account may not have.
 */
export type StatementCommodityOption = { id: number; code: string };

export function statementCommodityOptions(
  defaultCommodity: StatementCommodityOption | undefined,
  judged: readonly { commodity_id: number; commodity_code: string }[]
): StatementCommodityOption[] {
  const options: StatementCommodityOption[] = defaultCommodity ? [defaultCommodity] : [];
  for (const commodity of judged) {
    if (!options.some(option => option.id === commodity.commodity_id)) options.push({ id: commodity.commodity_id, code: commodity.commodity_code });
  }
  return options;
}

/** The owner's choice when it is one of the options, else the request, else the default, else the first (weakest) judged commodity. */
export function chooseStatementCommodity(
  options: readonly StatementCommodityOption[],
  chosen: number | undefined,
  requested: number | undefined,
  defaultID: number | null | undefined
): number | undefined {
  for (const candidate of [chosen, requested, defaultID ?? undefined]) {
    if (candidate !== undefined && options.some(option => option.id === candidate)) return candidate;
  }
  return options[0]?.id;
}

/**
 * The statement's commodity for a working copy. Once a commodity is locked
 * (the first one the form settled on, or the owner's pick) a later lookup
 * never switches it: a balance typed for USD must not be sent as EUR. If the
 * locked commodity is no longer offered, there is none until the owner picks
 * again (`lost`).
 */
export function statementCommodityChoice(
  options: readonly StatementCommodityOption[],
  locked: number | undefined,
  requested: number | undefined,
  defaultID: number | null | undefined
): { commodityID: number | undefined; lost: boolean } {
  if (locked !== undefined) {
    return options.some(option => option.id === locked) ? { commodityID: locked, lost: false } : { commodityID: undefined, lost: true };
  }
  return { commodityID: chooseStatementCommodity(options, undefined, requested, defaultID), lost: false };
}
