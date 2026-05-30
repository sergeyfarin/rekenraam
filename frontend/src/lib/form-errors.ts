import { APIClientError } from '$lib/api/client';
import { getAPIClientErrorMessage } from '$lib/api-error-messages';

export type FormErrorState = {
  message: string;
  requestId?: string;
};

export function getFormErrorState(error: unknown): FormErrorState | null {
  if (error == null) {
    return null;
  }

  return {
    message: getAPIClientErrorMessage(error),
    requestId: error instanceof APIClientError ? error.requestId : undefined
  };
}