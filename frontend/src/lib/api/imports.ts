import { APIClientError, toNetworkError } from '$lib/api/client';

// --- Types matching backend API responses ---

export interface ImportBatch {
  id: number;
  book_id: number;
  source_kind: string;
  profile_id?: number;
  status: 'previewing' | 'committed' | 'partially_committed' | 'discarded' | 'failed';
  original_filename: string;
  source_meta: string;
  created_at: string;
}

export interface NormalizedRow {
  date: string;
  amount: string;
  commodity_hint: string;
  payee_hint: string;
  category_hint: string;
  account_hint: string;
  transfer_hint: string;
  memo: string;
  external_ref: string;
  splits: StagedSplit[];
}

export interface StagedSplit {
  category_hint: string;
  amount: string;
  memo: string;
}

export interface ImportResolution {
  account_id?: number;
  commodity_id?: number;
  payee_id?: number;
  payee_name?: string;
  category_id?: number;
  exclude?: boolean;
}

export interface ImportStagedRow {
  id: number;
  batch_id: number;
  row_index: number;
  dedupe_fingerprint: string;
  normalized: string; // JSON string
  raw: string; // JSON string
  dedupe_status: 'new' | 'duplicate' | 'needs_attention' | 'excluded';
  resolution: string; // JSON string
  commit_status: 'pending' | 'committed' | 'skipped' | 'failed';
  committed_transaction_id?: number;
  commit_error?: string;
}

export interface ParseWarning {
  row_index: number;
  message: string;
}

export interface SourceMeta {
  account_hints: string[];
  currency_hints: string[];
  date_from?: string;
  date_to?: string;
}

export interface StartImportResponse {
  batch: ImportBatch;
  rows: ImportStagedRow[];
  warnings: ParseWarning[];
  meta: SourceMeta;
}

export interface GetImportBatchResponse {
  batch: ImportBatch;
  rows: ImportStagedRow[];
  next_cursor?: string;
}

export interface ListImportBatchesResponse {
  batches: ImportBatch[];
  next_cursor?: string;
}

export interface RowResolutionPatch {
  row_id: number;
  dedupe_status?: string;
  resolution?: ImportResolution;
}

export interface CommitImportBatchRequest {
  reconciliation_override?: boolean;
}

export interface CommitImportBatchResponse {
  batch_id: number;
  status: string;
  total_rows: number;
  committed_count: number;
  skipped_count: number;
  failed_count: number;
}

export interface PreviewCommitResponse {
  includable_count: number;
  duplicate_count: number;
  reconciliation_issues: Array<{ row_index: number; checkpoint_ids: number[] }>;
}

// --- API helpers ---

async function apiFetch<T>(url: string, init: RequestInit = {}): Promise<T> {
  try {
    const resp = await fetch(url, {
      credentials: 'same-origin',
      headers: { accept: 'application/json', ...init.headers },
      ...init
    });

    const body = await resp.json().catch(() => null);

    if (!resp.ok) {
      const code = body?.error?.code;
      const message = body?.error?.message;
      throw new APIClientError({
        status: resp.status,
        code,
        detail: message,
        requestId: resp.headers.get('X-Request-ID') ?? undefined
      });
    }

    return body as T;
  } catch (err) {
    if (err instanceof APIClientError) throw err;
    throw toNetworkError(err);
  }
}

// --- Public API functions ---

export async function startImport(file: File, csrfToken: string): Promise<StartImportResponse> {
  const form = new FormData();
  form.append('file', file, file.name);

  return apiFetch<StartImportResponse>('/api/v1/imports', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: form
  });
}

export async function getImportBatch(
  batchId: number,
  options: { limit?: number; cursor?: string } = {}
): Promise<GetImportBatchResponse> {
  const params = new URLSearchParams();
  if (options.limit) params.set('limit', String(options.limit));
  if (options.cursor) params.set('cursor', options.cursor);
  const qs = params.size > 0 ? '?' + params.toString() : '';
  return apiFetch<GetImportBatchResponse>(`/api/v1/imports/${batchId}${qs}`);
}

export async function listImportBatches(
  options: { limit?: number; cursor?: string } = {}
): Promise<ListImportBatchesResponse> {
  const params = new URLSearchParams();
  if (options.limit) params.set('limit', String(options.limit));
  if (options.cursor) params.set('cursor', options.cursor);
  const qs = params.size > 0 ? '?' + params.toString() : '';
  return apiFetch<ListImportBatchesResponse>(`/api/v1/imports${qs}`);
}

export async function patchImportBatch(
  batchId: number,
  patches: RowResolutionPatch[],
  csrfToken: string
): Promise<void> {
  await apiFetch<void>(`/api/v1/imports/${batchId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({
      row_resolutions: patches.map((p) => ({
        row_id: p.row_id,
        dedupe_status: p.dedupe_status,
        resolution: JSON.stringify(p.resolution ?? {})
      }))
    })
  });
}

export async function previewCommitImportBatch(batchId: number): Promise<PreviewCommitResponse> {
  return apiFetch<PreviewCommitResponse>(`/api/v1/imports/${batchId}/preview-commit`, {
    method: 'POST'
  });
}

export async function commitImportBatch(
  batchId: number,
  options: CommitImportBatchRequest,
  csrfToken: string
): Promise<CommitImportBatchResponse> {
  return apiFetch<CommitImportBatchResponse>(`/api/v1/imports/${batchId}/commit`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(options)
  });
}

export async function discardImportBatch(batchId: number, csrfToken: string): Promise<void> {
  await apiFetch<void>(`/api/v1/imports/${batchId}/discard`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken }
  });
}

// --- Parse helpers ---

export function parseNormalized(row: ImportStagedRow): NormalizedRow {
  try {
    return JSON.parse(row.normalized) as NormalizedRow;
  } catch {
    return {
      date: '',
      amount: '',
      commodity_hint: '',
      payee_hint: '',
      category_hint: '',
      account_hint: '',
      transfer_hint: '',
      memo: '',
      external_ref: '',
      splits: []
    };
  }
}

export function parseResolution(row: ImportStagedRow): ImportResolution {
  try {
    return JSON.parse(row.resolution) as ImportResolution;
  } catch {
    return {};
  }
}

export const importBatchesQueryKey = ['api', 'imports'] as const;
export const importBatchQueryKey = (batchId: number) => ['api', 'imports', batchId] as const;
