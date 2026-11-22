import type { components } from '#lib/api/schema.js';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type TagResponse = components['schemas']['TagResponse'];
export type TagsResponse = components['schemas']['TagsResponse'];
export type TagRequest = components['schemas']['TagRequest'];

export const tagsQueryKey = ['api', 'tags'] as const;

export function tagsQueryOptions(includeArchived = false) {
  return {
    queryKey: [...tagsQueryKey, { includeArchived }] as const,
    queryFn: () => getTags(includeArchived),
    staleTime: 10_000
  };
}

/**
 * The named tags, archived ones included, in one bounded read (#657): how a
 * form resolves the tags a record carries without relying on the capped
 * catalog. `ids` should be sorted so equal sets share a cache entry.
 */
export function tagsByIDQueryOptions(ids: readonly number[]) {
  return {
    queryKey: [...tagsQueryKey, { ids }] as const,
    queryFn: () => getTags(true, ids),
    staleTime: 10_000
  };
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

export async function updateTag(tagID: number, input: TagRequest, csrfToken: string): Promise<TagResponse> {
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
