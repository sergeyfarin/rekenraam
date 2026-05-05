import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";

export type MarkdownNote = {
  id: number;
  book_id: number;
  target_type: "account" | "transaction";
  account_id: number | null;
  transaction_id: number | null;
  title: string;
  body_markdown: string;
  pinned: boolean;
  created_at: string;
  updated_at: string;
  created_by_user_id: number | null;
  updated_by_user_id: number | null;
};

export type MarkdownNoteInput = {
  title: string;
  body_markdown: string;
  pinned: boolean;
};

export async function listAccountNotes(accountId: number): Promise<MarkdownNote[]> {
  return apiGet<MarkdownNote[]>(`/notes/accounts/${accountId}`);
}

export async function createAccountNote(accountId: number, input: MarkdownNoteInput): Promise<MarkdownNote> {
  return apiPost<MarkdownNote, MarkdownNoteInput>(`/notes/accounts/${accountId}`, input);
}

export async function listTransactionNotes(transactionId: number): Promise<MarkdownNote[]> {
  return apiGet<MarkdownNote[]>(`/notes/transactions/${transactionId}`);
}

export async function createTransactionNote(transactionId: number, input: MarkdownNoteInput): Promise<MarkdownNote> {
  return apiPost<MarkdownNote, MarkdownNoteInput>(`/notes/transactions/${transactionId}`, input);
}

export async function updateNote(noteId: number, input: MarkdownNoteInput): Promise<MarkdownNote> {
  return apiPut<MarkdownNote, MarkdownNoteInput>(`/notes/${noteId}`, input);
}

export async function deleteNote(noteId: number): Promise<void> {
  await apiDelete<void>(`/notes/${noteId}`);
}
