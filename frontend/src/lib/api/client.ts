import createClient from 'openapi-fetch';
import type { components, paths } from '#lib/api/schema.js';

export type APIErrorResponse = components['schemas']['ErrorResponse'];
export type APIErrorCode = components['schemas']['ErrorBody']['code'];
type APIErrorBody = components['schemas']['ErrorBody'];

export type APIReferenceConflict = {
  rule?: NonNullable<APIErrorBody['conflict_rule']>;
  date?: string;
  name?: string;
  builtinKey?: string;
};

function referenceConflict(body: APIErrorBody | undefined): APIReferenceConflict | undefined {
  if (!body?.conflict_rule && !body?.conflict_date) {
    return undefined;
  }
  return {
    rule: body.conflict_rule,
    date: body.conflict_date,
    name: body.conflict_name,
    builtinKey: body.conflict_builtin_key
  };
}

export class APIClientError extends Error {
  status: number;
  code?: APIErrorCode;
  detail?: string;
  requestId?: string;
  /** On a reconciliation guard refusal: the checkpoints the write would invalidate now (#296). */
  checkpointIDs?: number[];
  /** On a dated reference-data refusal: the rule, date and other record to explain (#552). */
  conflict?: APIReferenceConflict;

  constructor({
    status,
    code,
    detail,
    requestId,
    checkpointIDs,
    conflict
  }: {
    status: number;
    code?: APIErrorCode;
    detail?: string;
    requestId?: string;
    checkpointIDs?: number[];
    conflict?: APIReferenceConflict;
  }) {
    super(code ?? 'API_REQUEST_FAILED');

    this.name = 'APIClientError';
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.requestId = requestId;
    this.checkpointIDs = checkpointIDs;
    this.conflict = conflict;
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
    requestId: response?.headers.get('X-Request-ID') ?? undefined,
    checkpointIDs: errorBody?.checkpoint_ids,
    conflict: referenceConflict(errorBody)
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