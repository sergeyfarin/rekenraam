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

export async function getTags(includeArchived = false): Promise<TagsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/tags', {
      params: {
        query: {
          include_archived: includeArchived || undefined
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
