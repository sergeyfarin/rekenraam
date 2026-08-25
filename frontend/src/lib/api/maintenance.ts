import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type LedgerExportPreview = components['schemas']['LedgerExportPreviewResponse'];
export type QIFAccountStatus = components['schemas']['QIFAccountStatus'];
export type BackupStatus = components['schemas']['BackupStatusResponse'];
export type BackupPolicy = components['schemas']['BackupPolicy'];
export type BackupPolicyRequest = components['schemas']['BackupPolicyRequest'];
export type BackupRun = components['schemas']['BackupRun'];
export type SelfCheckStatus = components['schemas']['SelfCheckStatusResponse'];
export type SelfCheckRun = components['schemas']['SelfCheckRun'];
export type SelfCheckResult = components['schemas']['SelfCheckResult'];

export const exportPreviewQueryKey = ['api', 'exports', 'preview'] as const;
export const backupStatusQueryKey = ['api', 'maintenance', 'backups'] as const;
export const selfCheckQueryKey = ['api', 'maintenance', 'self-check'] as const;

/** Scope the export screen can ask for. Mirrors the API's shared filter contract. */
export type ExportScope = {
  from?: string;
  to?: string;
  dateBasis?: 'entry' | 'transaction';
  accountIDs?: number[];
  includeDescendants?: boolean;
  commodityIDs?: number[];
  qifDateLayout?: 'mdy' | 'dmy';
};

export function exportPreviewQueryOptions(scope: ExportScope = {}) {
  return {
    queryKey: [...exportPreviewQueryKey, scope] as const,
    queryFn: () => getExportPreview(scope),
    staleTime: 5_000
  };
}

export function backupStatusQueryOptions() {
  return {
    queryKey: backupStatusQueryKey,
    queryFn: getBackupStatus,
    staleTime: 5_000
  };
}

export function selfCheckQueryOptions() {
  return {
    queryKey: selfCheckQueryKey,
    queryFn: getSelfCheckStatus,
    staleTime: 5_000
  };
}

/**
 * The scope as query parameters, in the repeated-id shape the API expects.
 * Written once here so the preview and every download URL cannot describe
 * different exports.
 */
export function exportScopeParams(scope: ExportScope): URLSearchParams {
  const params = new URLSearchParams();
  if (scope.from) params.set('from', scope.from);
  if (scope.to) params.set('to', scope.to);
  if (scope.dateBasis) params.set('date_basis', scope.dateBasis);
  if (scope.includeDescendants) params.set('include_descendants', 'true');
  if (scope.qifDateLayout) params.set('qif_date_layout', scope.qifDateLayout);
  for (const accountID of scope.accountIDs ?? []) params.append('account_id', String(accountID));
  for (const commodityID of scope.commodityIDs ?? []) params.append('commodity_id', String(commodityID));
  return params;
}

export async function getExportPreview(scope: ExportScope): Promise<LedgerExportPreview> {
  const params = exportScopeParams(scope);
  const query = params.toString();
  return request<LedgerExportPreview>(`/api/v1/exports/preview${query ? `?${query}` : ''}`);
}

export async function getBackupStatus(): Promise<BackupStatus> {
  return request<BackupStatus>('/api/v1/maintenance/backups');
}

export async function getSelfCheckStatus(): Promise<SelfCheckStatus> {
  return request<SelfCheckStatus>('/api/v1/maintenance/self-check');
}

export async function saveBackupPolicy(
  policy: BackupPolicyRequest,
  csrfToken: string
): Promise<BackupPolicy> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/maintenance/backups/policy', {
      body: policy,
      headers: { 'X-CSRF-Token': csrfToken }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw asClientError(error);
  }
}

export async function requestBackup(csrfToken: string): Promise<BackupRun> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/maintenance/backups', {
      headers: { 'X-CSRF-Token': csrfToken }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw asClientError(error);
  }
}

export async function retryBackupRun(runID: number, csrfToken: string): Promise<BackupRun> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/maintenance/backups/{run_id}/retry',
      {
        params: { path: { run_id: runID } },
        headers: { 'X-CSRF-Token': csrfToken }
      }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw asClientError(error);
  }
}

export async function runSelfCheck(csrfToken: string): Promise<SelfCheckRun> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/maintenance/self-check', {
      headers: { 'X-CSRF-Token': csrfToken }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    throw asClientError(error);
  }
}

/**
 * Downloads an export through an authenticated request rather than a plain
 * link: the session is a same-origin cookie, and a link cannot carry the error
 * envelope back when the server refuses — which is exactly what a scoped QIF
 * request does when it names an account QIF cannot write.
 */
export async function downloadExport(path: string, scope: ExportScope, extra?: URLSearchParams) {
  const params = exportScopeParams(scope);
  for (const [key, value] of extra ?? []) params.append(key, value);
  const query = params.toString();

  const response = await fetch(`${path}${query ? `?${query}` : ''}`, {
    credentials: 'same-origin',
    headers: { accept: '*/*' }
  });

  if (!response.ok) {
    let code: string | undefined;
    let message: string | undefined;
    try {
      const body = await response.json();
      code = body?.error?.code;
      message = body?.error?.message;
    } catch {
      // A non-JSON failure is still a failure; the status carries it.
    }
    throw new APIClientError({
      status: response.status,
      code: code as never,
      detail: message
    });
  }

  return {
    blob: await response.blob(),
    filename: filenameFrom(response.headers.get('Content-Disposition'))
  };
}

/** The server names the file — including the QIF date layout, which only the filename carries. */
export function filenameFrom(disposition: string | null): string {
  const match = disposition?.match(/filename="([^"]+)"/);
  return match?.[1] ?? 'rekenraam-export';
}

async function request<T>(path: string): Promise<T> {
  try {
    const response = await fetch(path, {
      credentials: 'same-origin',
      headers: { accept: 'application/json' }
    });
    const body = await response.json();
    if (response.ok) return body as T;
    throw toAPIClientError(response, body);
  } catch (error) {
    throw asClientError(error);
  }
}

function asClientError(error: unknown): APIClientError {
  return error instanceof APIClientError ? error : toNetworkError(error);
}
