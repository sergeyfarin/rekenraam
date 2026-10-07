import type { components } from '#lib/api/schema.js';
import type { TransactionResponse } from '#lib/api/transactions.ts';
import { APIClientError, apiClient, toAPIClientError, toNetworkError } from '#lib/api/client.ts';

export type InvestmentInstrumentResponse = components['schemas']['InvestmentInstrumentResponse'];
export type InvestmentInstrumentsResponse = components['schemas']['InvestmentInstrumentsResponse'];
export type InvestmentPositionResponse = components['schemas']['InvestmentPositionResponse'];
export type InvestmentPositionsResponse = components['schemas']['InvestmentPositionsResponse'];
export type InvestmentLotResponse = components['schemas']['InvestmentLotResponse'];
export type InvestmentLotsResponse = components['schemas']['InvestmentLotsResponse'];
export type CostBasisProfileResponse = components['schemas']['CostBasisProfileResponse'];
export type CostBasisProfilesResponse = components['schemas']['CostBasisProfilesResponse'];
export type DividendDefaultResponse = components['schemas']['DividendDefaultResponse'];
export type DividendDefaultsResponse = components['schemas']['DividendDefaultsResponse'];
export type SellPreviewResponse = components['schemas']['SellPreviewResponse'];
export type InvestmentTradeResponse = components['schemas']['InvestmentTradeResponse'];
export type InvestmentTradeRequest = components['schemas']['InvestmentTradeRequest'];
export type ExternalTransferInRequest = components['schemas']['ExternalTransferInRequest'];
export type ExternalTransferInResponse = components['schemas']['ExternalTransferInResponse'];
export type InternalTransferRequest = components['schemas']['InternalTransferRequest'];
export type InternalTransferResponse = components['schemas']['InternalTransferResponse'];
export type InternalTransferPreviewResponse = components['schemas']['InternalTransferPreviewResponse'];
export type InternalTransferPlan = components['schemas']['InternalTransferPlan'];
export type ExternalTransferOutRequest = components['schemas']['ExternalTransferOutRequest'];
export type ExternalTransferOutResponse = components['schemas']['ExternalTransferOutResponse'];
export type ExternalTransferOutPreviewResponse = components['schemas']['ExternalTransferOutPreviewResponse'];
export type ExternalTransferOutPlan = components['schemas']['ExternalTransferOutPlan'];
export type CapitalReturnRequest = components['schemas']['CapitalReturnRequest'];
export type CapitalReturnResponse = components['schemas']['CapitalReturnResponse'];
export type CapitalReturnPreviewResponse = components['schemas']['CapitalReturnPreviewResponse'];
export type CapitalReturnEffect = components['schemas']['CapitalReturnEffect'];
export type InvestmentTransferOutReplacementRequest = components['schemas']['InvestmentTransferOutReplacementRequest'];
export type InvestmentTransferOutReplacementResponse = components['schemas']['InvestmentTransferOutReplacementResponse'];
export type InvestmentSplitRequest = components['schemas']['InvestmentSplitRequest'];
export type InvestmentSplitPlan = components['schemas']['InvestmentSplitPlan'];
export type InvestmentSplitPreviewResponse = components['schemas']['InvestmentSplitPreviewResponse'];
export type InvestmentSplitResponse = components['schemas']['InvestmentSplitResponse'];
export type InvestmentSplitReplacementRequest = components['schemas']['InvestmentSplitReplacementRequest'];
export type InvestmentSplitReplacementResponse = components['schemas']['InvestmentSplitReplacementResponse'];
export type InvestmentCorrectionSplitTerms = components['schemas']['InvestmentCorrectionSplitTerms'];
export type InvestmentWriteOffRequest = components['schemas']['InvestmentWriteOffRequest'];
export type DividendRequest = components['schemas']['DividendRequest'];
export type ReinvestedDividendRequest = components['schemas']['ReinvestedDividendRequest'];
export type CostBasisMethod = components['schemas']['CostBasisMethod'];
export type InvestmentGainsResponse = components['schemas']['InvestmentGainsResponse'];
export type RealizedGainEntry = components['schemas']['RealizedGainEntry'];
export type UnrealizedGainEntry = components['schemas']['UnrealizedGainEntry'];
export type RealizedGainTotal = components['schemas']['RealizedGainTotal'];

export type InvestmentEventSuggestionResponse = components['schemas']['InvestmentEventSuggestionResponse'];
export type InvestmentEventSuggestionsResponse = components['schemas']['InvestmentEventSuggestionsResponse'];
export type InvestmentAutomationRuleResponse = components['schemas']['InvestmentAutomationRuleResponse'];
export type InvestmentAutomationRulesResponse = components['schemas']['InvestmentAutomationRulesResponse'];
export type InvestmentAutomationRulesRequest = components['schemas']['InvestmentAutomationRulesRequest'];
export type InvestmentAutomationRuleRequest = components['schemas']['InvestmentAutomationRuleRequest'];
export type ReconciliationImpactResponse = components['schemas']['ReconciliationImpactResponse'];
export type GainImpact = components['schemas']['GainImpact'];
export type GainImpactChange = components['schemas']['GainImpactChange'];
export type InvestmentCorrectionChainResponse = components['schemas']['InvestmentCorrectionChainResponse'];
export type InvestmentSaleReversalRequest = components['schemas']['InvestmentSaleReversalRequest'];
export type InvestmentSaleReversalResponse = components['schemas']['InvestmentSaleReversalResponse'];
export type InvestmentBuyReversalRequest = components['schemas']['InvestmentBuyReversalRequest'];
export type InvestmentBuyReversalResponse = components['schemas']['InvestmentBuyReversalResponse'];
export type InvestmentTradeCorrectionContextResponse = components['schemas']['InvestmentTradeCorrectionContextResponse'];
export type InvestmentBuyReplacementRequest = components['schemas']['InvestmentBuyReplacementRequest'];
export type InvestmentBuyReplacementResponse = components['schemas']['InvestmentBuyReplacementResponse'];
export type InvestmentSaleReplacementRequest = components['schemas']['InvestmentSaleReplacementRequest'];
export type InvestmentWriteOffReplacementRequest = components['schemas']['InvestmentWriteOffReplacementRequest'];
export type InvestmentTransferReplacementRequest = components['schemas']['InvestmentTransferReplacementRequest'];
export type InvestmentTransferInReplacementRequest = components['schemas']['InvestmentTransferInReplacementRequest'];
export type InvestmentTransferInReplacementResponse = components['schemas']['InvestmentTransferInReplacementResponse'];
export type InvestmentTransferReplacementResponse = components['schemas']['InvestmentTransferReplacementResponse'];
export type InvestmentCorrectionTransferTerms = components['schemas']['InvestmentCorrectionTransferTerms'];
export type InvestmentSaleReplacementResponse = components['schemas']['InvestmentSaleReplacementResponse'];
export type InvestmentDividendReplacementRequest = components['schemas']['InvestmentDividendReplacementRequest'];
export type InvestmentDividendReplacementResponse = components['schemas']['InvestmentDividendReplacementResponse'];
export type InvestmentReinvestmentReplacementRequest = components['schemas']['InvestmentReinvestmentReplacementRequest'];
export type InvestmentCorrectionDividendTerms = components['schemas']['InvestmentCorrectionDividendTerms'];
export type InvestmentCorrectionReinvestmentTerms = components['schemas']['InvestmentCorrectionReinvestmentTerms'];

export const investmentPositionsQueryKey = ['api', 'investments', 'positions'] as const;
export const cashInLieuLotsQueryKey = ['api', 'investments', 'cash-in-lieu-lots'] as const;
export const investmentLotsQueryKey = ['api', 'investments', 'lots'] as const;
export const investmentInstrumentsQueryKey = ['api', 'investments', 'instruments'] as const;
export const investmentGainsQueryKey = ['api', 'investments', 'gains'] as const;
export const investmentEventSuggestionsQueryKey = ['api', 'investments', 'event-suggestions'] as const;
export const investmentAutomationRulesQueryKey = ['api', 'investments', 'automation-rules'] as const;
export const investmentCorrectionChainQueryKey = ['api', 'investments', 'correction-chain'] as const;

export function investmentPositionsQueryOptions() {
  return {
    queryKey: investmentPositionsQueryKey,
    queryFn: () => getInvestmentPositions(),
    staleTime: 30_000
  };
}

export function investmentLotsQueryOptions(accountID?: number, commodityID?: number) {
  return {
    queryKey: [...investmentLotsQueryKey, { accountID, commodityID }] as const,
    queryFn: () => getInvestmentLots(accountID, commodityID),
    staleTime: 30_000
  };
}

export function investmentInstrumentsQueryOptions() {
  return {
    queryKey: investmentInstrumentsQueryKey,
    queryFn: () => getInvestmentInstruments(),
    staleTime: 60_000
  };
}

export async function getInvestmentPositions(): Promise<InvestmentPositionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/positions');

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

export async function getInvestmentLots(
  accountID?: number,
  commodityID?: number
): Promise<InvestmentLotsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/lots', {
      params: {
        query: {
          account_id: accountID,
          commodity_id: commodityID
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

export async function getInvestmentInstruments(): Promise<InvestmentInstrumentsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/instruments');

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

export async function searchInvestmentInstruments(q: string): Promise<InvestmentInstrumentsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/search', {
      params: { query: { q } }
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

export async function getDividendDefaults(): Promise<DividendDefaultsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/dividend-defaults');

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

export async function recordBuy(
  input: InvestmentTradeRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/buy', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function externalTransferInReconciliationImpact(
  input: ExternalTransferInRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transfers/external/in/reconciliation-impact',
      { body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordExternalTransferIn(
  input: ExternalTransferInRequest,
  csrfToken: string
): Promise<ExternalTransferInResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transfers/external/in', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Runs the complete transfer writer and rolls back: carried basis plus impact. */
export async function previewInternalTransfer(
  input: InternalTransferRequest
): Promise<InternalTransferPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transfers/internal/preview',
      { body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordInternalTransfer(
  input: InternalTransferRequest,
  csrfToken: string
): Promise<InternalTransferResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transfers/internal', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Runs the complete outbound writer and rolls back: basis carried out plus impact. */
export async function previewExternalTransferOut(
  input: ExternalTransferOutRequest
): Promise<ExternalTransferOutPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transfers/external/out/preview',
      { body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordExternalTransferOut(
  input: ExternalTransferOutRequest,
  csrfToken: string
): Promise<ExternalTransferOutResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transfers/external/out', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewInvestmentSplit(
  input: InvestmentSplitRequest
): Promise<InvestmentSplitPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/splits/preview', { body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordInvestmentSplit(
  input: InvestmentSplitRequest,
  csrfToken: string
): Promise<InvestmentSplitResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/splits', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSell(
  input: InvestmentTradeRequest
): Promise<SellPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/sell/preview', {
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

export async function recordSell(
  input: InvestmentTradeRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/sell', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function getInvestmentCorrectionChain(transactionID: number): Promise<InvestmentCorrectionChainResponse> {
  try {
    const { data, error, response } = await apiClient.GET(
      '/api/v1/investments/transactions/{transaction_id}/correction-chain',
      { params: { path: { transaction_id: transactionID } } }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getInvestmentTradeCorrectionContext(transactionID: number): Promise<InvestmentTradeCorrectionContextResponse> {
  try {
    const { data, error, response } = await apiClient.GET(
      '/api/v1/investments/transactions/{transaction_id}/trade-correction-context',
      { params: { path: { transaction_id: transactionID } } }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewBuyReplacementReconciliation(transactionID: number, input: InvestmentBuyReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-buy/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceManualBuy(transactionID: number, input: InvestmentBuyReplacementRequest, csrfToken: string): Promise<InvestmentBuyReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-buy',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSaleReplacementReconciliation(transactionID: number, input: InvestmentSaleReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-sale/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceManualSale(transactionID: number, input: InvestmentSaleReplacementRequest, csrfToken: string): Promise<InvestmentSaleReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-sale',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSaleReversalReconciliation(
  transactionID: number,
  input: InvestmentSaleReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-sale/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseManualSale(
  transactionID: number,
  input: InvestmentSaleReversalRequest,
  csrfToken: string
): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-sale',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewBuyReversalReconciliation(
  transactionID: number,
  input: InvestmentBuyReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-buy/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseManualBuy(
  transactionID: number,
  input: InvestmentBuyReversalRequest,
  csrfToken: string
): Promise<InvestmentBuyReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-buy',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// A write-off disposes lots at zero proceeds: the whole remaining basis is
// realized as a loss. Separate from recordSell so a mistyped sale amount can
// never become a total loss.
export async function previewWriteOff(
  input: InvestmentWriteOffRequest
): Promise<SellPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/write-off/preview', {
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

export async function recordWriteOff(
  input: InvestmentWriteOffRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/write-off', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function recordDividend(
  input: DividendRequest,
  csrfToken: string
): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/dividend', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export async function recordReinvestedDividend(
  input: ReinvestedDividendRequest,
  csrfToken: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/reinvested-dividend', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

export function investmentGainsQueryOptions(from?: string, to?: string) {
  return {
    queryKey: [...investmentGainsQueryKey, { from, to }] as const,
    queryFn: () => getInvestmentGains(from, to),
    staleTime: 30_000
  };
}

export function investmentEventSuggestionsQueryOptions() {
  return {
    queryKey: investmentEventSuggestionsQueryKey,
    queryFn: () => getInvestmentEventSuggestions(),
    staleTime: 15_000
  };
}

export function investmentAutomationRulesQueryOptions() {
  return {
    queryKey: investmentAutomationRulesQueryKey,
    queryFn: () => getInvestmentAutomationRules(),
    staleTime: 60_000
  };
}

export async function getInvestmentGains(from?: string, to?: string): Promise<InvestmentGainsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/gains', {
      params: { query: { from, to } }
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

export async function getInvestmentEventSuggestions(): Promise<InvestmentEventSuggestionsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/event-suggestions');

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

export async function getInvestmentAutomationRules(): Promise<InvestmentAutomationRulesResponse> {
  try {
    const result = await apiClient.GET('/api/v1/investments/automation-rules');

    if (result.data !== undefined) {
      return result.data;
    }

    throw toAPIClientError(result.response, result.error);
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }

    throw toNetworkError(error);
  }
}

export async function acceptEventSuggestion(
  suggestionId: number,
  csrfToken: string
): Promise<InvestmentEventSuggestionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/event-suggestions/{suggestion_id}/accept',
      {
        params: { path: { suggestion_id: suggestionId }, header: { 'X-CSRF-Token': csrfToken } }
      }
    );

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

export async function ignoreEventSuggestion(
  suggestionId: number,
  csrfToken: string
): Promise<InvestmentEventSuggestionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/event-suggestions/{suggestion_id}/ignore',
      {
        params: { path: { suggestion_id: suggestionId }, header: { 'X-CSRF-Token': csrfToken } }
      }
    );

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

export async function saveAutomationRules(
  input: InvestmentAutomationRulesRequest,
  csrfToken: string
): Promise<InvestmentAutomationRulesResponse> {
  try {
    const { data, error, response } = await apiClient.PUT('/api/v1/investments/automation-rules', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
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

/**
 * Reconciliation-impact previews for the investment write paths.
 *
 * Each one plans the same postings its write route would and reports the active
 * checkpoints that write would invalidate, without persisting anything. The
 * forms call these before submitting so the confirmation can name what is about
 * to be invalidated rather than warning vaguely (T-53). They are previews, not
 * mutations, so they carry no CSRF token — same as previewSell above.
 */
export async function buyReconciliationImpact(
  input: InvestmentTradeRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/buy/reconciliation-impact',
      { body: input }
    );

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

export async function sellReconciliationImpact(
  input: InvestmentTradeRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/sell/reconciliation-impact',
      { body: input }
    );

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

export async function dividendReconciliationImpact(
  input: DividendRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/dividend/reconciliation-impact',
      { body: input }
    );

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

export async function reinvestedDividendReconciliationImpact(
  input: ReinvestedDividendRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/reinvested-dividend/reconciliation-impact',
      { body: input }
    );

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

export async function writeOffReconciliationImpact(
  input: InvestmentWriteOffRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/write-off/reconciliation-impact',
      { body: input }
    );

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

// Split reversal and replacement (T-129). Both previews run the actual
// writer in a rolled-back transaction, so they report the commit's own
// checkpoints and gain changes.
export async function previewSplitReversalReconciliation(
  transactionID: number,
  input: InvestmentSaleReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-split/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseSplit(
  transactionID: number,
  input: InvestmentSaleReversalRequest,
  csrfToken: string
): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-split',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSplitReplacement(
  transactionID: number,
  input: InvestmentSplitReplacementRequest
): Promise<InvestmentSplitPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-split/preview',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceSplit(
  transactionID: number,
  input: InvestmentSplitReplacementRequest,
  csrfToken: string
): Promise<InvestmentSplitReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-split',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Dividend and reinvested-dividend corrections (T-115).

export async function previewDividendReversalReconciliation(transactionID: number, input: InvestmentBuyReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-dividend/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseDividend(transactionID: number, input: InvestmentBuyReversalRequest, csrfToken: string): Promise<InvestmentBuyReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-dividend',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewCapitalReturnReversalReconciliation(transactionID: number, input: InvestmentBuyReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-return-of-capital/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseCapitalReturn(transactionID: number, input: InvestmentBuyReversalRequest, csrfToken: string): Promise<InvestmentBuyReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-return-of-capital',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Runs the complete return-of-capital writer and rolls back: per-lot effects plus impact. */
export async function previewCapitalReturn(input: CapitalReturnRequest): Promise<CapitalReturnPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/return-of-capital/preview', { body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordCapitalReturn(input: CapitalReturnRequest, csrfToken: string): Promise<CapitalReturnResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/return-of-capital', {
      params: { header: { 'X-CSRF-Token': csrfToken } }, body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewDividendReplacementReconciliation(transactionID: number, input: InvestmentDividendReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-dividend/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceDividend(transactionID: number, input: InvestmentDividendReplacementRequest, csrfToken: string): Promise<InvestmentDividendReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-dividend',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewReinvestmentReversalReconciliation(transactionID: number, input: InvestmentBuyReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-reinvested-dividend/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseReinvestedDividend(transactionID: number, input: InvestmentBuyReversalRequest, csrfToken: string): Promise<InvestmentBuyReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-reinvested-dividend',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewReinvestmentReplacementReconciliation(transactionID: number, input: InvestmentReinvestmentReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-reinvested-dividend/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceReinvestedDividend(transactionID: number, input: InvestmentReinvestmentReplacementRequest, csrfToken: string): Promise<InvestmentBuyReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-reinvested-dividend',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Write-off corrections (T-118).

export async function previewWriteOffReversalReconciliation(transactionID: number, input: InvestmentSaleReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-write-off/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseWriteOff(transactionID: number, input: InvestmentSaleReversalRequest, csrfToken: string): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-write-off',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewTransferReversalReconciliation(transactionID: number, input: InvestmentSaleReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-transfer/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseTransfer(transactionID: number, input: InvestmentSaleReversalRequest, csrfToken: string): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-transfer',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewTransferReplacement(transactionID: number, input: InvestmentTransferReplacementRequest): Promise<InternalTransferPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-transfer/preview',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceTransfer(transactionID: number, input: InvestmentTransferReplacementRequest, csrfToken: string): Promise<InvestmentTransferReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-transfer',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewTransferOutReplacement(transactionID: number, input: InvestmentTransferOutReplacementRequest): Promise<ExternalTransferOutPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-transfer-out/preview',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceTransferOut(transactionID: number, input: InvestmentTransferOutReplacementRequest, csrfToken: string): Promise<InvestmentTransferOutReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-transfer-out',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewTransferInReplacement(transactionID: number, input: InvestmentTransferInReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-transfer-in/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceTransferIn(transactionID: number, input: InvestmentTransferInReplacementRequest, csrfToken: string): Promise<InvestmentTransferInReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-transfer-in',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewWriteOffReplacementReconciliation(transactionID: number, input: InvestmentWriteOffReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-write-off/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceWriteOff(transactionID: number, input: InvestmentWriteOffReplacementRequest, csrfToken: string): Promise<InvestmentSaleReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-write-off',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export type CapitalReturnReplacementRequest = components['schemas']['CapitalReturnReplacementRequest'];

export async function previewCapitalReturnReplacement(transactionID: number, input: CapitalReturnReplacementRequest): Promise<CapitalReturnPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transactions/{transaction_id}/replace-return-of-capital/preview', {
      params: { path: { transaction_id: transactionID } }, body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceCapitalReturn(transactionID: number, input: CapitalReturnReplacementRequest, csrfToken: string): Promise<components['schemas']['CapitalReturnReplacementResponse']> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transactions/{transaction_id}/replace-return-of-capital', {
      params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}


export type CashInLieuRequest = components['schemas']['CashInLieuRequest'];
export type CashInLieuPreviewResponse = components['schemas']['CashInLieuPreviewResponse'];
export type CashInLieuReplacementRequest = components['schemas']['CashInLieuReplacementRequest'];

export async function previewCashInLieu(input: CashInLieuRequest): Promise<CashInLieuPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/cash-in-lieu/preview', { body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordCashInLieu(input: CashInLieuRequest, csrfToken: string): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/cash-in-lieu', { params: { header: { 'X-CSRF-Token': csrfToken } }, body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewCashInLieuReplacement(transactionID: number, input: CashInLieuReplacementRequest): Promise<CashInLieuPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transactions/{transaction_id}/replace-cash-in-lieu/preview', { params: { path: { transaction_id: transactionID } }, body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceCashInLieu(transactionID: number, input: CashInLieuReplacementRequest, csrfToken: string): Promise<InvestmentSaleReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transactions/{transaction_id}/replace-cash-in-lieu', { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewCashInLieuReversalReconciliation(transactionID: number, input: InvestmentBuyReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transactions/{transaction_id}/reverse-cash-in-lieu/reconciliation-impact', { params: { path: { transaction_id: transactionID } }, body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseCashInLieu(transactionID: number, input: InvestmentBuyReversalRequest, csrfToken: string): Promise<InvestmentBuyReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transactions/{transaction_id}/reverse-cash-in-lieu', { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getCashInLieuLots(splitTransactionID: number, disposalOn: string, currencyID: number, replacingTransactionID?: number): Promise<components['schemas']['CashInLieuLotsResponse']> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/transactions/{transaction_id}/cash-in-lieu-lots', {
      params: { path: { transaction_id: splitTransactionID }, query: { currency_id: currencyID, disposal_on: disposalOn, replacing_transaction_id: replacingTransactionID } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
