import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from './client';
import { getTagsByID, tagIDBatches, tagLookupBatchSize, type TagResponse } from './tags';

const tag = (id: number) => ({ id, name: `Tag ${id}`, status: 'archived' }) as TagResponse;

afterEach(() => vi.restoreAllMocks());

describe('tagIDBatches', () => {
  it('splits at the request limit and keeps the order', () => {
    expect(tagIDBatches([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]]);
    expect(tagIDBatches([])).toEqual([]);
  });
});

describe('getTagsByID', () => {
  // #657 review: a record may carry more tags than one request may name; a
  // single over-limit request returned 400 and hid every tag control.
  it('resolves more IDs than one request may name in bounded batches', async () => {
    const ids = Array.from({ length: tagLookupBatchSize + 1 }, (_, index) => index + 1);
    const get = vi.spyOn(apiClient, 'GET').mockImplementation((async (_path: string, init: { params: { query: { id?: number[] } } }) => (
      { data: { tags: (init.params.query.id ?? []).map(tag) }, response: new Response() }
    )) as never);
    const resolved = await getTagsByID(ids);
    expect(get).toHaveBeenCalledTimes(2);
    expect(get).toHaveBeenNthCalledWith(1, '/api/v1/tags', { params: { query: { include_archived: true, id: ids.slice(0, tagLookupBatchSize) } } });
    expect(get).toHaveBeenNthCalledWith(2, '/api/v1/tags', { params: { query: { include_archived: true, id: [tagLookupBatchSize + 1] } } });
    expect(resolved.tags.map((resolvedTag) => resolvedTag.id)).toEqual(ids);
  });
});
