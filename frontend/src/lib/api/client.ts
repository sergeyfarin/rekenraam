import createClient from 'openapi-fetch';
import type { components, paths } from '$lib/api/schema';

export type APIErrorResponse = components['schemas']['ErrorResponse'];
export type APIErrorCode = components['schemas']['ErrorBody']['code'];

export class APIClientError extends Error {
  status: number;
  code?: APIErrorCode;
  detail?: string;
  requestId?: string;

  constructor({
    status,
    code,
    detail,
    requestId
  }: {
    status: number;
    code?: APIErrorCode;
    detail?: string;
    requestId?: string;
  }) {
    super(code ?? 'API_REQUEST_FAILED');

    this.name = 'APIClientError';
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.requestId = requestId;
  }
}

export const apiClient = createClient<paths>({
  baseUrl: '',
  credentials: 'same-origin',
  headers: {
    accept: 'application/json'
  }
});

export function toAPIClientError(
  response: Response | undefined,
  error: unknown
): APIClientError {
  const errorBody = isAPIErrorResponse(error) ? error.error : undefined;

  return new APIClientError({
    status: response?.status ?? 0,
    code: errorBody?.code,
    detail: errorBody?.message,
    requestId: response?.headers.get('X-Request-ID') ?? undefined
  });
}

export function toNetworkError(error: unknown): APIClientError {
  const detail = error instanceof Error ? error.message : undefined;

  return new APIClientError({
    status: 0,
    detail
  });
}

function isAPIErrorResponse(value: unknown): value is APIErrorResponse {
  if (typeof value !== 'object' || value === null) {
    return false;
  }

  const candidate = value as Partial<APIErrorResponse>;
  return typeof candidate.error?.message === 'string' && typeof candidate.error?.code === 'string';
}