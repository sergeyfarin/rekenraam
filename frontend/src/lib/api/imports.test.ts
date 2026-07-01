import { describe, it, expect } from 'vitest';
import { parseBatchSourceMeta, type ImportBatch } from './imports';

function batchWithSourceMeta(sourceMeta: string): ImportBatch {
  return {
    id: 1,
    book_id: 1,
    source_kind: 'trading212',
    status: 'previewing',
    original_filename: '',
    source_meta: sourceMeta,
    created_at: '2026-06-30T00:00:00Z'
  } as ImportBatch;
}

describe('parseBatchSourceMeta', () => {
  it('parses fetch_status "failed" — the contract pollFetchStatus relies on to stop polling', () => {
    const meta = parseBatchSourceMeta(
      batchWithSourceMeta('{"fetch_status":"failed","error":"provider rejected the API key"}')
    );
    expect(meta.fetch_status).toBe('failed');
    expect(meta.error).toBe('provider rejected the API key');
  });

  it('parses fetch_status "ready" with hints', () => {
    const meta = parseBatchSourceMeta(
      batchWithSourceMeta(
        '{"fetch_status":"ready","currency_hints":["EUR"],"date_from":"2024-01-01","date_to":"2024-01-02"}'
      )
    );
    expect(meta.fetch_status).toBe('ready');
    expect(meta.currency_hints).toEqual(['EUR']);
    expect(meta.date_from).toBe('2024-01-01');
    expect(meta.date_to).toBe('2024-01-02');
  });

  it('parses fetch_status "fetching"', () => {
    const meta = parseBatchSourceMeta(batchWithSourceMeta('{"fetch_status":"fetching"}'));
    expect(meta.fetch_status).toBe('fetching');
  });

  it('returns an empty object for malformed JSON instead of throwing', () => {
    expect(() => parseBatchSourceMeta(batchWithSourceMeta('{not valid json'))).not.toThrow();
    expect(parseBatchSourceMeta(batchWithSourceMeta('{not valid json'))).toEqual({});
  });

  it('returns an empty object for a file-import batch (no fetch_status key at all)', () => {
    const meta = parseBatchSourceMeta(
      batchWithSourceMeta('{"account_hints":[],"currency_hints":["USD"]}')
    );
    expect(meta.fetch_status).toBeUndefined();
  });
});
