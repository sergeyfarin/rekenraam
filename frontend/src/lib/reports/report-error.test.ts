import { describe, expect, it } from 'vitest';
import { APIClientError } from '$lib/api/client';
import { isRetryableAPIError, reportErrorCode, reportErrorState } from './report-error';

describe('isRetryableAPIError', () => {
  it('does not offer retry for an overflow, because exact arithmetic repeats', () => {
    expect(isRetryableAPIError('LEDGER_OVERFLOW')).toBe(false);
  });

  it('does not offer retry for a query the server rejected', () => {
    expect(isRetryableAPIError('VALIDATION_FAILED')).toBe(false);
    expect(isRetryableAPIError('NOT_FOUND')).toBe(false);
  });

  it('does not offer retry for an expired session, which the same request cannot renew', () => {
    expect(isRetryableAPIError('UNAUTHENTICATED')).toBe(false);
    expect(isRetryableAPIError('CSRF_INVALID')).toBe(false);
  });

  it('offers retry for a transient server-side failure', () => {
    expect(isRetryableAPIError('RESOURCE_BUSY')).toBe(true);
    expect(isRetryableAPIError('INTERNAL_ERROR')).toBe(true);
    expect(isRetryableAPIError('RATE_LIMITED')).toBe(true);
  });
});

describe('reportErrorCode', () => {
  it('reads the stable code off an API failure', () => {
    expect(reportErrorCode(new APIClientError({ status: 422, code: 'LEDGER_OVERFLOW' }))).toBe('LEDGER_OVERFLOW');
  });

  it('has no code for a transport failure or a non-API error', () => {
    expect(reportErrorCode(new APIClientError({ status: 0 }))).toBeUndefined();
    expect(reportErrorCode(new TypeError('offline'))).toBeUndefined();
  });
});

describe('reportErrorState', () => {
  it('keeps the view copy and offers retry when there is no code to explain', () => {
    expect(reportErrorState(new APIClientError({ status: 0 }), 'fallback')).toEqual({
      copy: 'fallback',
      retryable: true
    });
    expect(reportErrorState(new TypeError('offline'), 'fallback')).toEqual({ copy: 'fallback', retryable: true });
  });
});
