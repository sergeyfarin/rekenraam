import { APIClientError, toNetworkError } from '$lib/api/client';
import type { components } from '$lib/api/schema';

export type ImportBatch = components['schemas']['ImportBatchResponse'];
export type NormalizedRow = components['schemas']['ImportNormalizedRow'];
export type StagedSplit = components['schemas']['ImportStagedSplit'];
export type ImportResolution = components['schemas']['ImportResolution'];
export type ImportStagedRow = components['schemas']['ImportStagedRowResponse'];
export type ParseWarning = components['schemas']['ParseWarning'];
export type SourceMeta = components['schemas']['ImportSourceMeta'];
export type StartImportResponse = components['schemas']['StartImportResponse'];
export type StartOnlineImportResponse = components['schemas']['StartOnlineImportResponse'];
export type GetImportBatchResponse = components['schemas']['GetImportBatchResponse'];
export type ListImportBatchesResponse = components['schemas']['ListImportBatchesResponse'];
export type CommitImportBatchRequest = components['schemas']['CommitImportBatchRequest'];
export type CommitImportBatchResponse = components['schemas']['CommitImportBatchResponse'];
export type PreviewCommitResponse = components['schemas']['PreviewCommitResponse'];

export interface RowResolutionPatch {
  row_id: number;
  dedupe_status?: components['schemas']['ImportDedupeStatus'];
  resolution?: ImportResolution;
}

const IMPORT_BATCH_ROW_PAGE_LIMIT = 200;

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

export async function startOnlineImport(
  connectionId: number,
  csrfToken: string
): Promise<StartOnlineImportResponse> {
  return apiFetch<StartOnlineImportResponse>('/api/v1/imports', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ source: 'trading212', connection_id: connectionId })
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

export async function getFullImportBatch(
  batchId: number,
  initialPage?: GetImportBatchResponse
): Promise<GetImportBatchResponse> {
  const rows: ImportStagedRow[] = [];
  let batch = initialPage?.batch;
  let cursor = initialPage?.next_cursor ?? undefined;
  let shouldFetch = initialPage === undefined;

  if (initialPage) {
    rows.push(...initialPage.rows);
  }

  while (shouldFetch || cursor) {
    const page = await getImportBatch(batchId, {
      limit: IMPORT_BATCH_ROW_PAGE_LIMIT,
      cursor: shouldFetch ? undefined : cursor
    });

    batch = page.batch;
    rows.push(...page.rows);
    cursor = page.next_cursor ?? undefined;
    shouldFetch = false;
  }

  return {
    batch: batch!,
    rows,
    next_cursor: null
  };
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

// ImportBatchSourceMeta is the shape app.trading212BatchMeta marshals into
// batch.source_meta for online (fetch-driven) batches. It is not part of the
// generated OpenAPI types because source_meta is an opaque JSON string by
// design (see docs/plans/trading212-import-plan.md "Data model").
export interface ImportBatchSourceMeta {
  fetch_status?: 'fetching' | 'ready' | 'failed';
  connection_display_name?: string;
  account_hints?: string[];
  currency_hints?: string[];
  date_from?: string;
  date_to?: string;
  warnings?: ParseWarning[];
  error?: string;
}

export function parseBatchSourceMeta(batch: ImportBatch): ImportBatchSourceMeta {
  try {
    return JSON.parse(batch.source_meta) as ImportBatchSourceMeta;
  } catch {
    return {};
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
