import createClient from 'openapi-fetch';
import type { components, paths } from '$lib/api/schema';

export type APIErrorResponse = components['schemas']['ErrorResponse'];
export type APIErrorCode = components['schemas']['ErrorBody']['code'];

export class APIClientError extends Error {
  status: number;
  code?: APIErrorCode;
  requestId?: string;

  constructor({
    status,
    message,
    code,
    requestId
  }: {
    status: number;
    message: string;
    code?: APIErrorCode;
    requestId?: string;
  }) {
    super(message);

    this.name = 'APIClientError';
    this.status = status;
    this.code = code;
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
  error: unknown,
  fallbackMessage: string
): APIClientError {
  const errorBody = isAPIErrorResponse(error) ? error.error : undefined;

  return new APIClientError({
    status: response?.status ?? 0,
    message: errorBody?.message ?? fallbackMessage,
    code: errorBody?.code,
    requestId: response?.headers.get('X-Request-ID') ?? undefined
  });
}

function isAPIErrorResponse(value: unknown): value is APIErrorResponse {
  if (typeof value !== 'object' || value === null) {
    return false;
  }

  const candidate = value as Partial<APIErrorResponse>;
  return typeof candidate.error?.message === 'string' && typeof candidate.error?.code === 'string';
}