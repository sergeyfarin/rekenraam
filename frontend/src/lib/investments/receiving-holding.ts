// Where a corporate action's new units arrive (share exchange, spin-off,
// cash-and-stock merger). A holding tied to one instrument cannot hold
// another, so the candidates are the accounts that are untied or tied to the
// new instrument, the source first when it qualifies.
//
// A choice is an account ID, never "the same holding": that would silently
// point at another account once the source changes (#508). A choice that is
// no longer a candidate stays chosen and is reported unavailable, so the form
// names it and refuses to preview instead of selecting another account.

export type ReceivingAccount = { id: number; name?: string; default_commodity_id?: number | null };

export type ReceivingOption = { id: number; name: string; source: boolean };

export type ReceivingHolding = {
  options: ReceivingOption[];
  /** The selected account ID: the owner's choice, else the suggestion, else ''. */
  value: string;
  /** The owner's choice is not among the options. */
  unavailable: boolean;
  /** The request's destination; undefined when the units arrive in the source. */
  destinationID: number | undefined;
};

export function receivingHolding(
  accounts: readonly ReceivingAccount[],
  sourceAccountID: number | undefined,
  newCommodityID: number,
  chosen: string | null
): ReceivingHolding {
  const canHold = (account: ReceivingAccount) =>
    account.default_commodity_id == null || account.default_commodity_id === newCommodityID;
  const source = accounts.find((account) => account.id === sourceAccountID);
  const options: ReceivingOption[] = !source || newCommodityID <= 0 ? [] : [
    ...(canHold(source) ? [{ id: source.id, name: source.name ?? '', source: true }] : []),
    ...accounts.filter((account) => account.id !== source.id && canHold(account))
      .map((account) => ({ id: account.id, name: account.name ?? '', source: false }))
  ];
  const suggested = options.find((option) => option.source)
    ?? options.find((option) => accounts.find((account) => account.id === option.id)?.default_commodity_id === newCommodityID);
  const value = chosen ?? (suggested ? String(suggested.id) : '');
  const unavailable = chosen !== null && chosen !== '' && !options.some((option) => String(option.id) === chosen);
  const id = Number(value);
  return { options, value, unavailable, destinationID: value && id !== sourceAccountID ? id : undefined };
}

