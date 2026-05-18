import { apiDelete, apiGet, apiPost } from "$lib/api/client";

export type ImportDraft = {
  payee_name: string | null;
  memo: string | null;
  txn_date: string | null;
  amount_minor: number | null;
  reference: string | null;
  import_id: string | null;
  currency_code: string | null;
  account_id: number | null;
  category_id: number | null;
  payee_id: number | null;
};

export type ImportPreviewRow = {
  row_number: number;
  raw_values: string[];
  draft: ImportDraft | null;
  error: string | null;
  matched_tx_id: number | null;
  is_duplicate: boolean;
};

export type ImportPreviewResult = {
  format: string;
  rows: ImportPreviewRow[];
  file_error: string | null;
};

export type ImportRule = {
  id: number;
  book_id: number;
  rule_kind: string;
  match_type: string;
  match_text: string;
  priority: number;
  amount_min_minor: number | null;
  amount_max_minor: number | null;
  date_from: string | null;
  date_to: string | null;
  match_account_id: number | null;
  target_account_id: number | null;
  target_category_id: number | null;
  target_payee_id: number | null;
  created_at: string;
};

export type ImportCommitResult = {
  session: {
    id: number;
    status: string;
    started_at: string;
    committed_at: string | null;
    notes: string | null;
  };
  batch: {
    created_tx_ids: number[];
    matched_tx_ids: number[];
    updated_tx_ids: number[];
    skipped: number;
  };
};

export type ImportPreviewInput = {
  book_id: number;
  account_id: number;
  format: string;
  content?: string | null;
  content_base64?: string | null;
  file_name?: string | null;
  locale?: {
    decimal_separator?: string | null;
    thousands_separator?: string | null;
    date_format?: string | null;
    csv_delimiter?: string | null;
  } | null;
};

export async function previewImport(input: ImportPreviewInput): Promise<ImportPreviewResult> {
  return apiPost<ImportPreviewResult, ImportPreviewInput>("/imports/preview", input);
}

export async function commitImport(input: Record<string, unknown>): Promise<ImportCommitResult> {
  return apiPost<ImportCommitResult, Record<string, unknown>>("/imports/commit", input);
}

export async function listImportRules(bookId: number): Promise<ImportRule[]> {
  return apiGet<ImportRule[]>(`/imports/rules?book_id=${bookId}`);
}

export async function createImportRule(input: Record<string, unknown>): Promise<ImportRule> {
  return apiPost<ImportRule, Record<string, unknown>>("/imports/rules", input);
}

export async function deleteImportRule(ruleId: number): Promise<void> {
  await apiDelete<void>(`/imports/rules/${ruleId}`);
}
