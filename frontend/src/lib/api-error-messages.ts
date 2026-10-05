import type { APIErrorCode } from '#lib/api/client.ts';
import { APIClientError } from '#lib/api/client.ts';
import { m } from '#lib/paraglide/messages.js';

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
  FORECAST_TOO_LARGE: () => m.api_error_forecast_too_large(),
  FORECAST_BASIS_CHANGED: () => m.api_error_forecast_basis_changed(),
  INVESTMENT_WORKFLOW_REQUIRED: () => m.api_error_investment_workflow_required(),
  INVESTMENT_EVENT_OUT_OF_ORDER: () => m.api_error_investment_event_out_of_order(),
  INVESTMENT_TRANSFER_POOL_REQUIRED: () => m.api_error_investment_transfer_pool_required(),
  INVESTMENT_TRANSFER_POOL_UNAVAILABLE: () => m.api_error_investment_transfer_pool_unavailable(),
  INVESTMENT_SALE_ALREADY_CORRECTED: () => m.api_error_investment_sale_already_corrected(),
  INVESTMENT_IMPORTED_SALE: () => m.api_error_investment_imported_sale(),
  INVESTMENT_SALE_CHANGED: () => m.api_error_investment_sale_changed(),
  INVESTMENT_SALE_DEPENDENCY: () => m.api_error_investment_sale_dependency(),
  INVESTMENT_BUY_ALREADY_CORRECTED: () => m.api_error_investment_buy_already_corrected(),
  INVESTMENT_IMPORTED_BUY: () => m.api_error_investment_imported_buy(),
  INVESTMENT_BUY_CHANGED: () => m.api_error_investment_buy_changed(),
  INVESTMENT_BUY_DEPENDENCY: () => m.api_error_investment_buy_dependency(),
  INVESTMENT_DIVIDEND_ALREADY_CORRECTED: () => m.api_error_investment_dividend_already_corrected(),
  INVESTMENT_IMPORTED_DIVIDEND: () => m.api_error_investment_imported_dividend(),
  INVESTMENT_DIVIDEND_CHANGED: () => m.api_error_investment_dividend_changed(),
  INVESTMENT_WRITE_OFF_ALREADY_CORRECTED: () => m.api_error_investment_write_off_already_corrected(),
  INVESTMENT_WRITE_OFF_CHANGED: () => m.api_error_investment_write_off_changed(),
  INVESTMENT_TRANSFER_ALREADY_CORRECTED: () => m.api_error_investment_transfer_already_corrected(),
  INVESTMENT_TRANSFER_CHANGED: () => m.api_error_investment_transfer_changed(),
  INVESTMENT_TRANSFER_DEPENDENCY: () => m.api_error_investment_transfer_dependency(),
  INVESTMENT_IMPORTED_TRANSFER: () => m.api_error_investment_imported_transfer(),
  INVESTMENT_REINVESTMENT_ALREADY_CORRECTED: () => m.api_error_investment_reinvestment_already_corrected(),
  INVESTMENT_REINVESTMENT_CHANGED: () => m.api_error_investment_reinvestment_changed(),
  INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED: () => m.api_error_investment_gain_impact_acknowledgement_required(),
  INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE: () => m.api_error_investment_gain_impact_acknowledgement_stale(),
  INVESTMENT_SPLIT_NO_HOLDINGS: () => m.api_error_investment_split_no_holdings(),
  INVESTMENT_CAPITAL_RETURN_NO_HOLDINGS: () => m.api_error_investment_capital_return_no_holdings(),
  INVESTMENT_CASH_IN_LIEU_SPLIT_UNAVAILABLE: () => m.api_error_investment_cash_in_lieu_split_unavailable(),
  INVESTMENT_SPLIT_FRACTION_UNREPRESENTABLE: () => m.api_error_investment_split_fraction_unrepresentable(),
  INVESTMENT_SPLIT_CHANGED: () => m.api_error_investment_split_changed(),
  INVESTMENT_SPLIT_DEPENDENCY: () => m.api_error_investment_split_dependency(),
  INVESTMENT_SPLIT_ALREADY_CORRECTED: () => m.api_error_investment_split_already_corrected(),
  INVESTMENT_IMPORTED_SPLIT: () => m.api_error_investment_imported_split(),
  IMPORT_SPLIT_LINK_UNAVAILABLE: () => m.api_error_import_split_link_unavailable(),
  TRANSACTION_DRAFT_NOT_USER_CREATABLE: () => m.api_error_transaction_draft_not_user_creatable(),
  TRANSACTION_VERSION_STALE: () => m.api_error_transaction_version_stale(),
  POSTING_ACCOUNT_VERSION_STALE: () => m.api_error_posting_account_version_stale(),
  RECURRING_TEMPLATE_UNBALANCED: () => m.api_error_recurring_template_unbalanced(),
  RECURRING_SCHEDULE_INVALID: () => m.api_error_recurring_schedule_invalid(),
  RECURRING_OCCURRENCE_ALREADY_MATERIALIZED: () => m.api_error_recurring_occurrence_already_materialized(),
  RECURRING_TEMPLATE_ARCHIVED: () => m.api_error_recurring_template_archived(),
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
