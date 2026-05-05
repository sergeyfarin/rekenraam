import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";
import type { TransactionFilter } from "$lib/api/transactions";

export type TransactionSavedView = {
  id: number;
  user_id: number;
  book_id: number;
  name: string;
  filters: TransactionFilter;
  is_shared: boolean;
  created_at: string;
  updated_at: string;
};

export async function listTransactionViews(bookId: number): Promise<TransactionSavedView[]> {
  return apiGet<TransactionSavedView[]>(`/search/transaction-views?book_id=${bookId}`);
}

export async function createTransactionView(input: {
  book_id: number;
  name: string;
  filters: TransactionFilter;
  is_shared: boolean;
}): Promise<TransactionSavedView> {
  return apiPost<TransactionSavedView, typeof input>("/search/transaction-views", input);
}

export async function updateTransactionView(viewId: number, input: {
  book_id: number;
  name: string;
  filters: TransactionFilter;
  is_shared: boolean;
}): Promise<TransactionSavedView> {
  return apiPut<TransactionSavedView, typeof input>(`/search/transaction-views/${viewId}`, input);
}

export async function deleteTransactionView(viewId: number): Promise<void> {
  await apiDelete<void>(`/search/transaction-views/${viewId}`);
}
