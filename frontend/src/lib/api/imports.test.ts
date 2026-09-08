import { afterEach, describe, it, expect, vi } from 'vitest';
import {
  analyzeCSVImport,
  getFullImportBatch,
  parseBatchSourceMeta,
  startImport,
  type GetImportBatchResponse,
  type ImportBatch
} from './imports';

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

function importBatchResponse(rows: { id: number }[], nextCursor: string | null): GetImportBatchResponse {
  return {
    batch: batchWithSourceMeta('{"fetch_status":"ready"}'),
    rows: rows.map((row, index) => ({
      id: row.id,
      batch_id: 1,
      row_index: index,
      dedupe_fingerprint: `fingerprint-${row.id}`,
      normalized: '{}',
      raw: '{}',
      dedupe_status: 'new',
      resolution: '{}',
      commit_status: 'pending'
    })),
    next_cursor: nextCursor
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('startImport', () => {
  it('sends the selected legacy text encoding with the QIF upload', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ batch: {}, rows: [], warnings: [], meta: {} }), {
        status: 201,
        headers: { 'content-type': 'application/json' }
      })
    );

    await startImport(new File(['qif'], 'money.qif'), 'csrf', undefined, 'windows-1251');

    const init = fetchMock.mock.calls[0]?.[1];
    expect(init?.body).toBeInstanceOf(FormData);
    expect((init?.body as FormData).get('text_encoding')).toBe('windows-1251');
  });
});

describe('analyzeCSVImport', () => {
  it('sends the encoding and optional delimiter without creating an import batch', async () => {
    const response = {
      headers: ['Дата', 'Сумма'],
      delimiter: 'semicolon',
      text_encoding: 'windows-1251',
      encoding_source: 'selected',
      encoding_confidence: 100
    };
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify(response), { status: 200, headers: { 'content-type': 'application/json' } })
    );

    await expect(analyzeCSVImport(new File(['csv'], 'money.csv'), 'csrf', 'windows-1251', 'semicolon')).resolves.toEqual(response);

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/imports/analyze', expect.objectContaining({ method: 'POST' }));
    const body = fetchMock.mock.calls[0]?.[1]?.body as FormData;
    expect(body.get('text_encoding')).toBe('windows-1251');
    expect(body.get('delimiter')).toBe('semicolon');
  });
});

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

describe('getFullImportBatch', () => {
  it('follows next_cursor so online preview rows are not silently truncated', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(JSON.stringify(importBatchResponse([{ id: 10 }], '0:10')), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        })
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(importBatchResponse([{ id: 11 }], null)), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        })
      );

    const result = await getFullImportBatch(1);

    expect(result.rows.map((row) => row.id)).toEqual([10, 11]);
    expect(result.next_cursor).toBeNull();
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/v1/imports/1?limit=200',
      expect.objectContaining({ credentials: 'same-origin' })
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/v1/imports/1?limit=200&cursor=0%3A10',
      expect.objectContaining({ credentials: 'same-origin' })
    );
  });

  it('can continue from an already fetched first page', async () => {
    const firstPage = importBatchResponse([{ id: 10 }], '0:10');
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(JSON.stringify(importBatchResponse([{ id: 11 }], null)), {
          status: 200,
          headers: { 'content-type': 'application/json' }
        })
      );

    const result = await getFullImportBatch(1, firstPage);

    expect(result.rows.map((row) => row.id)).toEqual([10, 11]);
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/imports/1?limit=200&cursor=0%3A10',
      expect.objectContaining({ credentials: 'same-origin' })
    );
  });
});
