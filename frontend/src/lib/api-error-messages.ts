import type { APIErrorCode } from '$lib/api/client';
import { APIClientError } from '$lib/api/client';
import { m } from '$lib/paraglide/messages.js';

const apiErrorMessageByCode: Record<APIErrorCode, () => string> = {
  VALIDATION_FAILED: () => m.api_error_validation_failed(),
  UNAUTHENTICATED: () => m.api_error_unauthenticated(),
  FORBIDDEN: () => m.api_error_forbidden(),
  NOT_FOUND: () => m.api_error_not_found(),
  CONFLICT: () => m.api_error_conflict(),
  CSRF_INVALID: () => m.api_error_csrf_invalid(),
  EXPORT_SCOPE_UNSUPPORTED: () => m.api_error_export_scope_unsupported(),
  RATE_LIMITED: () => m.api_error_rate_limited(),
  RESOURCE_BUSY: () => m.api_error_resource_busy(),
  LEDGER_OVERFLOW: () => m.api_error_ledger_overflow(),
  SETUP_REQUIRED: () => m.api_error_setup_required(),
  SETUP_ALREADY_COMPLETE: () => m.api_error_setup_already_complete(),
  CONFIG_REQUIRED: () => m.api_error_config_required(),
  PROVIDER_ERROR: () => m.api_error_provider_error(),
  QIF_ACCOUNT_UNSUPPORTED: () => m.api_error_qif_account_unsupported(),
  INTERNAL_ERROR: () => m.api_error_internal_error()
};

export function getAPIErrorMessage(code: APIErrorCode): string {
  return apiErrorMessageByCode[code]?.() ?? m.api_error_generic();
}

export function getAPIClientErrorMessage(error: unknown): string {
  if (error instanceof APIClientError) {
    if (error.code !== undefined) {
      return getAPIErrorMessage(error.code);
    }

    if (error.status === 0) {
      return m.api_error_network();
    }
  }

  return m.api_error_generic();
}
