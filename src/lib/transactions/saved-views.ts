import type { TransactionFilter } from "$lib/api/transactions";

export type SortBy = "date" | "payee" | "status" | "amount";
export type SortDir = "asc" | "desc";

export type FilterFormState = {
  search: string;
  dateFrom: string;
  dateTo: string;
  statusFilter: string;
  accountFilterId: number | null;
  amountMin: string;
  amountMax: string;
  sortBy: SortBy;
  sortDir: SortDir;
};

export const DEFAULT_FILTER_STATE: FilterFormState = {
  search: "",
  dateFrom: "",
  dateTo: "",
  statusFilter: "",
  accountFilterId: null,
  amountMin: "",
  amountMax: "",
  sortBy: "date",
  sortDir: "desc",
};

const AMOUNT_SCALE_FACTOR = 100;

function parseAmountToMinorOrUndefined(value: string): number | undefined {
  if (!value) return undefined;
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return undefined;
  return Math.round(Math.abs(numeric) * AMOUNT_SCALE_FACTOR);
}

function minorToDisplayString(minor: number | undefined): string {
  if (minor === undefined) return "";
  return String(minor / AMOUNT_SCALE_FACTOR);
}

export function buildFilterFromState(
  state: FilterFormState,
  options: { bookId: number; limit: number; offset: number },
): TransactionFilter {
  return {
    book_id: options.bookId,
    account_id: state.accountFilterId ?? undefined,
    date_from: state.dateFrom || undefined,
    date_to: state.dateTo || undefined,
    search: state.search || undefined,
    status: state.statusFilter || undefined,
    amount_min: parseAmountToMinorOrUndefined(state.amountMin),
    amount_max: parseAmountToMinorOrUndefined(state.amountMax),
    sort_by: state.sortBy,
    sort_dir: state.sortDir,
    limit: options.limit,
    offset: options.offset,
  };
}

export function applyViewToState(filters: TransactionFilter): FilterFormState {
  return {
    search: filters.search ?? "",
    dateFrom: filters.date_from ?? "",
    dateTo: filters.date_to ?? "",
    statusFilter: filters.status ?? "",
    accountFilterId: filters.account_id ?? null,
    amountMin: minorToDisplayString(filters.amount_min),
    amountMax: minorToDisplayString(filters.amount_max),
    sortBy: filters.sort_by ?? "date",
    sortDir: filters.sort_dir ?? "desc",
  };
}
