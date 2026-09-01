import { APIClientError } from '$lib/api/client';
import { getAPIClientErrorMessage } from '$lib/api-error-messages';

export type FormErrorState = {
  message: string;
  requestId?: string;
};

/** A message already translated at the UI boundary and safe to show verbatim. */
export class TranslatedFormError extends Error {}

export function getFormErrorState(error: unknown): FormErrorState | null {
  if (error == null) {
    return null;
  }

  if (error instanceof TranslatedFormError) {
    return { message: error.message };
  }

  return {
    message: getAPIClientErrorMessage(error),
    requestId: error instanceof APIClientError ? error.requestId : undefined
  };
}
