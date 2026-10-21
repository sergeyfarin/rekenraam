import type { ReadonlyURLSearchParams } from '$app/state';

/** Invalid shared-link values fall back to the server's owner-local month and first currency. */
export function parseBudgetFilters(params: ReadonlyURLSearchParams) {
  const month = params.get('month') ?? '';
  const currencyID = Number(params.get('currency_id'));
  return {
    month: /^(?!0000)\d{4}-(0[1-9]|1[0-2])$/.test(month) ? month : '',
    currencyID: Number.isSafeInteger(currencyID) && currencyID > 0 ? currencyID : 0
  };
}
