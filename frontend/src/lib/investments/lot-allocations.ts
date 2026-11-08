/**
 * Lot quantities an activity's working copy restored for lots the position
 * does not hold on the current date (#521 review). They are never dropped
 * silently: the form names them and refuses to preview until the owner
 * removes them or changes the date back, because filtering them out would
 * quietly move fewer units than the owner entered.
 */
export function unavailableAllocations(
  quantities: Readonly<Record<string, string | undefined>>,
  available: readonly (string | number)[]
): string[] {
  const held = new Set(available.map(String));
  return Object.entries(quantities)
    .filter(([lot, quantity]) => !!quantity?.trim() && !held.has(lot))
    .map(([lot]) => lot);
}

/** The quantities without the named lots. */
export function withoutAllocations<Q extends Record<string, string | undefined>>(quantities: Q, lots: readonly string[]): Q {
  return Object.fromEntries(Object.entries(quantities).filter(([lot]) => !lots.includes(lot))) as Q;
}
