import { apiDeleteWithTauriFallback, apiGet, apiGetWithTauriFallback, apiPostWithTauriFallback, apiPutWithTauriFallback, invokeTauri } from "$lib/api/client";

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

export type TransactionSplitInput = {
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

export type TransactionMutationInput = {
  id?: number;
  book_id: number;
  txn_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  import_id: string | null;
  splits: TransactionSplitInput[];
};

export type AccountRegisterEntry = {
  tx_id: number;
  split_id: number;
  account_id: number;
  txn_date: string;
  posted_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  commodity_id: number;
  category_id: number | null;
  amount_minor: number;
  running_balance_minor: number;
  created_at: string;
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

type ApiRegisterEntry = {
  tx_id: number;
  split_id: number;
  account_id: number;
  occurred_date: string;
  posted_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  commodity_id: number;
  category_id: number | null;
  amount_minor: number;
  running_balance_minor: number;
  created_at: string;
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

function mapRegisterEntry(entry: ApiRegisterEntry): AccountRegisterEntry {
  return {
    tx_id: entry.tx_id,
    split_id: entry.split_id,
    account_id: entry.account_id,
    txn_date: entry.occurred_date,
    posted_date: entry.posted_date,
    payee_id: entry.payee_id,
    memo: entry.memo,
    status: entry.status,
    reference: entry.reference,
    commodity_id: entry.commodity_id,
    category_id: entry.category_id,
    amount_minor: entry.amount_minor,
    running_balance_minor: entry.running_balance_minor,
    created_at: entry.created_at,
  };
}

function mapTransactionToRegisterEntries(transactions: TransactionWithSplits[], accountId: number): AccountRegisterEntry[] {
  const ascendingEntries = [...transactions]
    .sort((left, right) => left.transaction.txn_date.localeCompare(right.transaction.txn_date) || left.transaction.id - right.transaction.id)
    .flatMap((transaction) =>
      transaction.splits
        .filter((split) => split.account_id === accountId)
        .map((split) => ({ transaction, split }))
    );

  let runningBalanceMinor = 0;
  const entries = ascendingEntries.map(({ transaction, split }) => {
    runningBalanceMinor += split.amount_minor;
    return {
      tx_id: transaction.transaction.id,
      split_id: split.id,
      account_id: split.account_id,
      txn_date: transaction.transaction.txn_date,
      posted_date: transaction.transaction.txn_date,
      payee_id: transaction.transaction.payee_id,
      memo: transaction.transaction.memo,
      status: transaction.transaction.status,
      reference: transaction.transaction.reference,
      commodity_id: split.commodity_id,
      category_id: split.category_id,
      amount_minor: split.amount_minor,
      running_balance_minor: runningBalanceMinor,
      created_at: "",
    } satisfies AccountRegisterEntry;
  });

  return entries.reverse();
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

export async function getTransactionById(transactionId: number): Promise<TransactionWithSplits | null> {
  try {
    const response = await apiGet<ApiTransaction>(`/transactions/${transactionId}`);
    return mapTransaction(response);
  } catch (error) {
    if (error instanceof Error && error.message.includes("transaction not found")) {
      return null;
    }
    try {
      return await apiGetWithTauriFallback<TransactionWithSplits>(
        `/transactions/${transactionId}`,
        "get_transaction_with_splits",
        { id: transactionId }
      );
    } catch (fallbackError) {
      if (fallbackError instanceof Error && fallbackError.message.includes("transaction not found")) {
        return null;
      }
      throw fallbackError;
    }
  }
}

export async function listAccountRegister(accountId: number): Promise<AccountRegisterEntry[]> {
  try {
    const response = await apiGet<ApiRegisterEntry[]>(`/accounts/${accountId}/register`);
    return response.map(mapRegisterEntry);
  } catch {
    const transactions = await listTransactions({ account_id: accountId, limit: 10000, offset: 0 });
    return mapTransactionToRegisterEntries(transactions, accountId);
  }
}

export async function createTransaction(input: TransactionMutationInput): Promise<void> {
  await apiPostWithTauriFallback<unknown, TransactionMutationInput>(
    "/transactions",
    input,
    "create_transaction_with_splits",
    { input }
  );
}

export async function updateTransaction(input: TransactionMutationInput): Promise<void> {
  if (input.id === undefined) {
    throw new Error("Transaction id is required for updates");
  }

  await apiPutWithTauriFallback<unknown, TransactionMutationInput>(
    `/transactions/${input.id}`,
    input,
    "update_transaction_with_splits",
    { input }
  );
}

export async function deleteTransaction(transactionId: number): Promise<void> {
  await apiDeleteWithTauriFallback<unknown>(`/transactions/${transactionId}`, "delete_transaction", { id: transactionId });
}

export async function duplicateTransaction(transactionId: number, today: string): Promise<void> {
  await invokeTauri("duplicate_transaction", { id: transactionId, today });
}

export async function bulkVoidTransactions(transactionIds: number[]): Promise<number> {
  return invokeTauri<number>("bulk_void_transactions", { ids: transactionIds });
}

export async function bulkDeleteTransactions(transactionIds: number[]): Promise<number> {
  return invokeTauri<number>("bulk_delete_transactions", { ids: transactionIds });
}

export async function getPayeeDefaults(payeeId: number, accountId?: number): Promise<{ category_id: number | null; memo: string | null }> {
  return invokeTauri<{ category_id: number | null; memo: string | null }>("get_payee_defaults", {
    payeeId,
    accountId,
  });
}