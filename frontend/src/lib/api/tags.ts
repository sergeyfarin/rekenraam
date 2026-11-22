import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type TagResponse = components['schemas']['TagResponse'];
export type TagsResponse = components['schemas']['TagsResponse'];
export type TagRequest = components['schemas']['TagRequest'];

export type TagDirectoryResponse = components['schemas']['TagDirectoryResponse'];
export type TagDirectoryRow = TagDirectoryResponse['tags'][number];
export type TagDirectoryStatus = TagDirectoryRow['status'] | 'all';

export const tagDirectoryQueryKey = ['api', 'tag-directory'] as const;
export const tagsQueryKey = ['api', 'tags'] as const;

export function tagsQueryOptions(includeArchived = false) {
  return {
    queryKey: [...tagsQueryKey, { includeArchived }] as const,
    queryFn: () => getTags(includeArchived),
    staleTime: 10_000
  };
}

export function tagDirectoryQueryOptions(status: TagDirectoryStatus, q: string) {
  return {
    queryKey: [...tagDirectoryQueryKey, { status, q }] as const,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }: { pageParam: string | undefined; signal: AbortSignal }) => {
      try {
        const { data, error, response } = await apiClient.GET('/api/v1/tags/directory', {
          params: { query: { status, q: q || undefined, cursor: pageParam } },
          signal
        });
        if (data !== undefined) return data;
        throw toAPIClientError(response, error);
      } catch (error) {
        if (error instanceof APIClientError) throw error;
        throw toNetworkError(error);
      }
    },
    getNextPageParam: (lastPage: TagDirectoryResponse) => lastPage.next_cursor ?? undefined
  };
}

export function tagQueryOptions(tagID?: number) {
  return {
    queryKey: [...tagsQueryKey, 'one', tagID] as const,
    queryFn: () => getTag(tagID!),
    enabled: tagID !== undefined
  };
}

/** The most tag IDs one list request may name (`maxItems` of `id` on `GET /api/v1/tags`). */
export const tagLookupBatchSize = 200;

/** Splits IDs into request-sized batches, keeping their order. */
export function tagIDBatches(ids: readonly number[], size = tagLookupBatchSize): number[][] {
  const batches: number[][] = [];
  for (let start = 0; start < ids.length; start += size) batches.push(ids.slice(start, start + size));
  return batches;
}

/**
 * The named tags, archived ones included, resolved by ID (#657): how a form
 * names the tags a record carries without relying on the capped catalog. A
 * record may carry more tags than one request may name, so the IDs go in
 * bounded batches and the results are merged. `ids` should be sorted so equal
 * sets share a cache entry.
 */
export function tagsByIDQueryOptions(ids: readonly number[]) {
  return {
    queryKey: [...tagsQueryKey, { ids }] as const,
    queryFn: () => getTagsByID(ids),
    staleTime: 10_000
  };
}

export async function getTagsByID(ids: readonly number[]): Promise<TagsResponse> {
  const pages = await Promise.all(tagIDBatches(ids).map((batch) => getTags(true, batch)));
  const byID = new Map<number, TagResponse>();
  for (const page of pages) for (const tag of page.tags) byID.set(tag.id, tag);
  return { tags: [...byID.values()] };
}

export async function getTags(includeArchived = false, ids?: readonly number[]): Promise<TagsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/tags', {
      params: {
        query: {
          include_archived: includeArchived || undefined,
          id: ids?.length ? [...ids] : undefined
        }
      }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function getTag(tagID: number): Promise<TagResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/tags/{tag_id}', {
      params: {
        path: {
          tag_id: tagID
        }
      }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function createTag(input: TagRequest, csrfToken: string): Promise<TagResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/tags', {
      params: {
        header: {
          'X-CSRF-Token': csrfToken
        }
      },
      body: input
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function renameTag(tagID: number, name: string, csrfToken: string): Promise<TagResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/tags/{tag_id}', {
      params: {
        path: {
          tag_id: tagID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      },
      body: { name }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function archiveTag(tagID: number, csrfToken: string): Promise<TagResponse> {
  return tagLifecycleMutation('/api/v1/tags/{tag_id}/archive', tagID, csrfToken);
}

export async function restoreTag(tagID: number, csrfToken: string): Promise<TagResponse> {
  return tagLifecycleMutation('/api/v1/tags/{tag_id}/restore', tagID, csrfToken);
}

async function tagLifecycleMutation(
  path: '/api/v1/tags/{tag_id}/archive' | '/api/v1/tags/{tag_id}/restore',
  tagID: number,
  csrfToken: string
): Promise<TagResponse> {
  try {
    const { data, error, response } = await apiClient.POST(path, {
      params: {
        path: {
          tag_id: tagID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      }
    });

    if (data !== undefined) {
      return data;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}
