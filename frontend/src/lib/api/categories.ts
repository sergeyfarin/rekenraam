import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type CategoryResponse = components['schemas']['CategoryResponse'];
export type CategoriesResponse = components['schemas']['CategoriesResponse'];
export type CreateCategoryRequest = components['schemas']['CreateCategoryRequest'];
export type UpdateCategoryRequest = components['schemas']['UpdateCategoryRequest'];
export type CategoryLifecycleRequest = components['schemas']['CategoryLifecycleRequest'];
export type CompleteCategoriesSetupResponse = components['schemas']['CompleteCategoriesSetupResponse'];

export const categoriesQueryKey = ['api', 'categories'] as const;

export function categoriesQueryOptions(options: { includeArchived?: boolean; categoryType?: 'income' | 'expense' } = {}) {
  return {
    queryKey: [...categoriesQueryKey, options] as const,
    queryFn: () => getCategories(options),
    staleTime: 10_000
  };
}

export async function getCategories(options: { includeArchived?: boolean; categoryType?: 'income' | 'expense' } = {}): Promise<CategoriesResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/categories', {
      params: {
        query: {
          include_archived: options.includeArchived || undefined,
          category_type: options.categoryType
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

export async function getCategory(categoryID: number): Promise<CategoryResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/categories/{category_id}', {
      params: {
        path: {
          category_id: categoryID
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

export async function createCategory(input: CreateCategoryRequest, csrfToken: string): Promise<CategoryResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/categories', {
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

export async function updateCategory(categoryID: number, input: UpdateCategoryRequest, csrfToken: string): Promise<CategoryResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/categories/{category_id}', {
      params: {
        path: {
          category_id: categoryID
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

export async function disableCategory(categoryID: number, csrfToken: string, input: CategoryLifecycleRequest = {}): Promise<CategoryResponse> {
  return categoryLifecycleMutation('/api/v1/categories/{category_id}/disable', categoryID, csrfToken, input);
}

export async function restoreCategory(categoryID: number, csrfToken: string, input: CategoryLifecycleRequest = {}): Promise<CategoryResponse> {
  return categoryLifecycleMutation('/api/v1/categories/{category_id}/restore', categoryID, csrfToken, input);
}

export async function deleteCategory(categoryID: number, csrfToken: string): Promise<void> {
  try {
    const { error, response } = await apiClient.DELETE('/api/v1/categories/{category_id}', {
      params: {
        path: {
          category_id: categoryID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      }
    });

    if (response.ok) {
      return;
    }

    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function completeCategoriesSetup(csrfToken: string): Promise<CompleteCategoriesSetupResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/setup/categories', {
      params: {
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

async function categoryLifecycleMutation(
  path: '/api/v1/categories/{category_id}/disable' | '/api/v1/categories/{category_id}/restore',
  categoryID: number,
  csrfToken: string,
  input: CategoryLifecycleRequest
): Promise<CategoryResponse> {
  try {
    const { data, error, response } = await apiClient.POST(path, {
      params: {
        path: {
          category_id: categoryID
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
