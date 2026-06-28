import { APIClientError, toNetworkError } from '$lib/api/client';
import type { components } from '$lib/api/schema';

export type ImportConnection = components['schemas']['ImportConnectionResponse'];
export type ListImportConnectionsResponse =
  components['schemas']['ListImportConnectionsResponse'];
export type CreateImportConnectionRequest =
  components['schemas']['CreateImportConnectionRequest'];
export type UpdateImportConnectionRequest =
  components['schemas']['UpdateImportConnectionRequest'];

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

export async function listImportConnections(): Promise<ListImportConnectionsResponse> {
  return apiFetch<ListImportConnectionsResponse>('/api/v1/import-connections');
}

export async function createImportConnection(
  request: CreateImportConnectionRequest,
  csrfToken: string
): Promise<ImportConnection> {
  return apiFetch<ImportConnection>('/api/v1/import-connections', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(request)
  });
}

export async function updateImportConnection(
  connectionId: number,
  request: UpdateImportConnectionRequest,
  csrfToken: string
): Promise<ImportConnection> {
  return apiFetch<ImportConnection>(`/api/v1/import-connections/${connectionId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(request)
  });
}

export async function deleteImportConnection(
  connectionId: number,
  csrfToken: string
): Promise<void> {
  await apiFetch<void>(`/api/v1/import-connections/${connectionId}`, {
    method: 'DELETE',
    headers: { 'X-CSRF-Token': csrfToken }
  });
}

// --- Query key helpers ---

export const importConnectionsQueryKey = ['api', 'import-connections'] as const;
