import { apiGet, apiGetWithTauriFallback } from "$lib/api/client";

export type TransactionFilter = {
  book_id?: number;
  account_id?: number;
  payee_id?: number;
  date_from?: string;
  date_to?: string;
  search?: string;
  status?: string;
  amount_min?: number;
  amount_max?: number;
  sort_by?: "date" | "payee" | "status" | "amount";
  sort_dir?: "asc" | "desc";
  limit?: number;
  offset?: number;
};

export type TransactionRecord = {
  id: number;
  txn_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
};

export type SplitRecord = {
  id: number;
  tx_id: number;
  account_id: number;
  commodity_id: number;
  amount_minor: number;
  category_id: number | null;
  tag_id: number | null;
  person_id: number | null;
  project_id: number | null;
  share_bps: number | null;
  memo: string | null;
};

export type TransactionWithSplits = {
  transaction: TransactionRecord;
  splits: SplitRecord[];
};

type ApiSplit = {
  id: number;
  tx_id: number;
  account_id: number;
  commodity_id: number;
  amount_minor: number;
  category_id: number | null;
  tag_id: number | null;
  person_id: number | null;
  project_id: number | null;
  share_bps: number | null;
  memo: string | null;
};

type ApiTransaction = {
  id: number;
  book_id: number;
  occurred_date: string;
  posted_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  created_at: string;
  splits: ApiSplit[];
};

function toQueryString(filter: TransactionFilter): string {
  const params = new URLSearchParams();

  if (filter.book_id !== undefined) params.set("book_id", String(filter.book_id));
  if (filter.account_id !== undefined) params.set("account_id", String(filter.account_id));
  if (filter.payee_id !== undefined) params.set("payee_id", String(filter.payee_id));
  if (filter.date_from) params.set("occurred_from", filter.date_from);
  if (filter.date_to) params.set("occurred_to", filter.date_to);
  if (filter.search) params.set("search", filter.search);
  if (filter.status) params.set("status", filter.status);
  if (filter.amount_min !== undefined) params.set("amount_min", String(filter.amount_min));
  if (filter.amount_max !== undefined) params.set("amount_max", String(filter.amount_max));
  if (filter.sort_by) params.set("sort_by", filter.sort_by);
  if (filter.sort_dir) params.set("sort_dir", filter.sort_dir);
  if (filter.limit !== undefined) params.set("limit", String(filter.limit));
  if (filter.offset !== undefined) params.set("offset", String(filter.offset));

  const query = params.toString();
  return query ? `?${query}` : "";
}

function mapTransaction(transaction: ApiTransaction): TransactionWithSplits {
  return {
    transaction: {
      id: transaction.id,
      txn_date: transaction.occurred_date,
      payee_id: transaction.payee_id,
      memo: transaction.memo,
      status: transaction.status,
      reference: transaction.reference,
    },
    splits: transaction.splits.map((split) => ({
      id: split.id,
      tx_id: split.tx_id,
      account_id: split.account_id,
      commodity_id: split.commodity_id,
      amount_minor: split.amount_minor,
      category_id: split.category_id,
      tag_id: split.tag_id,
      person_id: split.person_id,
      project_id: split.project_id,
      share_bps: split.share_bps,
      memo: split.memo,
    })),
  };
}

export async function listTransactions(filter: TransactionFilter): Promise<TransactionWithSplits[]> {
  const path = `/transactions${toQueryString(filter)}`;

  try {
    const response = await apiGet<ApiTransaction[]>(path);
    return response.map(mapTransaction);
  } catch {
    return apiGetWithTauriFallback<TransactionWithSplits[]>(path, "list_transactions", { filter });
  }
}