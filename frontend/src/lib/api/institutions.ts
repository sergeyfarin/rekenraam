import type { components } from '$lib/api/schema';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '$lib/api/client';

export type InstitutionResponse = components['schemas']['InstitutionResponse'];
export type InstitutionsResponse = components['schemas']['InstitutionsResponse'];
export type InstitutionRequest = components['schemas']['InstitutionRequest'];

export const institutionsQueryKey = ['api', 'institutions'] as const;

export function institutionsQueryOptions(includeArchived = false) {
  return {
    queryKey: [...institutionsQueryKey, { includeArchived }] as const,
    queryFn: () => getInstitutions(includeArchived),
    staleTime: 10_000
  };
}

export async function getInstitutions(includeArchived = false): Promise<InstitutionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/institutions', {
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

export async function getInstitution(institutionID: number): Promise<InstitutionResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/institutions/{institution_id}', {
      params: {
        path: {
          institution_id: institutionID
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

export async function getInstitutionVersions(institutionID: number): Promise<InstitutionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/institutions/{institution_id}/versions', {
      params: {
        path: {
          institution_id: institutionID
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

export async function createInstitution(input: InstitutionRequest, csrfToken: string): Promise<InstitutionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/institutions', {
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

export async function updateInstitution(
  institutionID: number,
  input: InstitutionRequest,
  csrfToken: string
): Promise<InstitutionResponse> {
  try {
    const { data, error, response } = await apiClient.PATCH('/api/v1/institutions/{institution_id}', {
      params: {
        path: {
          institution_id: institutionID
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

export async function archiveInstitution(institutionID: number, csrfToken: string): Promise<InstitutionResponse> {
  return institutionLifecycleMutation('/api/v1/institutions/{institution_id}/archive', institutionID, csrfToken);
}

export async function restoreInstitution(institutionID: number, csrfToken: string): Promise<InstitutionResponse> {
  return institutionLifecycleMutation('/api/v1/institutions/{institution_id}/restore', institutionID, csrfToken);
}

export async function deleteInstitution(institutionID: number, csrfToken: string): Promise<void> {
  try {
    const { error, response } = await apiClient.DELETE('/api/v1/institutions/{institution_id}', {
      params: {
        path: {
          institution_id: institutionID
        },
        header: {
          'X-CSRF-Token': csrfToken
        }
      }
    });

    if (response?.ok) {
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

async function institutionLifecycleMutation(
  path: '/api/v1/institutions/{institution_id}/archive' | '/api/v1/institutions/{institution_id}/restore',
  institutionID: number,
  csrfToken: string
): Promise<InstitutionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(path, {
      params: {
        path: {
          institution_id: institutionID
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
