import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";
import type { TransactionMutationInput, TransactionSplitInput } from "$lib/api/transactions";

export type TransactionTemplate = {
  id: number;
  user_id: number;
  book_id: number;
  name: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  is_shared: boolean;
  created_at: string;
  updated_at: string;
  splits: (TransactionSplitInput & { id: number; line_order: number })[];
};

export type TransactionTemplateInput = {
  book_id: number;
  name: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  is_shared: boolean;
  splits: TransactionSplitInput[];
};

export async function listTransactionTemplates(bookId: number): Promise<TransactionTemplate[]> {
  return apiGet<TransactionTemplate[]>(`/templates/transactions?book_id=${bookId}`);
}

export async function createTransactionTemplate(input: TransactionTemplateInput): Promise<TransactionTemplate> {
  return apiPost<TransactionTemplate, TransactionTemplateInput>("/templates/transactions", input);
}

export async function updateTransactionTemplate(templateId: number, input: TransactionTemplateInput): Promise<TransactionTemplate> {
  return apiPut<TransactionTemplate, TransactionTemplateInput>(`/templates/transactions/${templateId}`, input);
}

export async function deleteTransactionTemplate(templateId: number): Promise<void> {
  await apiDelete<void>(`/templates/transactions/${templateId}`);
}

export async function postTransactionTemplate(templateId: number, txnDate: string): Promise<TransactionMutationInput> {
  return apiPost<TransactionMutationInput, { txn_date: string }>(`/templates/transactions/${templateId}/post`, { txn_date: txnDate });
}
