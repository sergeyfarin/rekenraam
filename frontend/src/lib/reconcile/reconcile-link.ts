/**
 * Links into Reconcile name the account and, when they come from one
 * statement, its commodity: an account can be reconciled in more than its
 * default currency, and its statement's checkpoints are listed per commodity
 * (#595 review).
 */
export function reconcileHref(accountID: number, commodityID?: number): string {
  const params = new URLSearchParams({ account: String(accountID) });
  if (commodityID !== undefined) params.set('commodity', String(commodityID));
  return `/app/ledger/reconcile?${params.toString()}`;
}

export interface ReconcileRequest {
  accountID?: number;
  commodityID?: number;
}

function positiveID(value: string | null): number | undefined {
  if (value === null || !/^[1-9][0-9]{0,15}$/.test(value)) return undefined;
  return Number(value);
}

/** Reads `?account=` and `?commodity=`; a commodity without an account is ignored. */
export function parseReconcileRequest(params: Pick<URLSearchParams, 'get'>): ReconcileRequest {
  const accountID = positiveID(params.get('account'));
  if (accountID === undefined) return {};
  return { accountID, commodityID: positiveID(params.get('commodity')) };
}
