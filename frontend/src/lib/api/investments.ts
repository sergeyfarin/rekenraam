import type { components, paths } from '#lib/api/schema.js';
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
export type CapitalReturnRead = components['schemas']['CapitalReturnRead'];
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
export type ShareExchangeRequest = components['schemas']['ShareExchangeRequest'];
export type ShareExchangePlan = components['schemas']['ShareExchangePlan'];
export type ShareExchangeBasisTotal = components['schemas']['ShareExchangeBasisTotal'];
export type ShareExchangePreviewResponse = components['schemas']['ShareExchangePreviewResponse'];
export type ShareExchangeResponse = components['schemas']['ShareExchangeResponse'];
export type InvestmentCorrectionShareExchangeTerms = components['schemas']['InvestmentCorrectionShareExchangeTerms'];
export type ShareExchangeReplacementRequest = components['schemas']['ShareExchangeReplacementRequest'];
export type ShareExchangeReplacementResponse = components['schemas']['ShareExchangeReplacementResponse'];
export type SpinOffRequest = components['schemas']['SpinOffRequest'];
export type SpinOffPlan = components['schemas']['SpinOffPlan'];
export type SpinOffPreviewResponse = components['schemas']['SpinOffPreviewResponse'];
export type SpinOffResponse = components['schemas']['SpinOffResponse'];
export type InvestmentCorrectionSpinOffTerms = components['schemas']['InvestmentCorrectionSpinOffTerms'];
export type CashStockMergerRequest = components['schemas']['CashStockMergerRequest'];
export type CashStockMergerPlan = components['schemas']['CashStockMergerPlan'];
export type CashStockMergerPreviewResponse = components['schemas']['CashStockMergerPreviewResponse'];
export type CashStockMergerResponse = components['schemas']['CashStockMergerResponse'];
export type InvestmentCorrectionCashStockMergerTerms = components['schemas']['InvestmentCorrectionCashStockMergerTerms'];
export type CashStockMergerReplacementRequest = components['schemas']['CashStockMergerReplacementRequest'];
export type CashStockMergerReplacementResponse = components['schemas']['CashStockMergerReplacementResponse'];
export type SpinOffReplacementRequest = components['schemas']['SpinOffReplacementRequest'];
export type SpinOffReplacementResponse = components['schemas']['SpinOffReplacementResponse'];
export type InvestmentLotOrigin = components['schemas']['InvestmentLotOrigin'];
export type InvestmentWriteOffRequest = components['schemas']['InvestmentWriteOffRequest'];
export type DividendRequest = components['schemas']['DividendRequest'];
export type ReinvestedDividendRequest = components['schemas']['ReinvestedDividendRequest'];
export type CostBasisMethod = components['schemas']['CostBasisMethod'];
export type GainsFXInput = Required<Pick<
  NonNullable<paths['/api/v1/investments/gains']['get']['parameters']['query']>,
  'reporting_currency_id' | 'fx_method' | 'fx_max_age_days'
>>;
export type GainsFXFigure = components['schemas']['GainsFXFigure'];
export type GainsFXComponent = components['schemas']['GainsFXComponent'];
export type InvestmentGainsResponse = components['schemas']['InvestmentGainsResponse'];
export type InvestmentGainsStatementResponse = components['schemas']['InvestmentGainsStatementResponse'];
export type InvestmentReturnsResponse = components['schemas']['InvestmentReturnsResponse'];
export type ReturnFigure = components['schemas']['ReturnFigure'];
export type ReturnsAmount = components['schemas']['ReturnsAmount'];
export type ReturnsSeries = components['schemas']['ReturnsSeries'];
export type ReturnsAllocation = components['schemas']['ReturnsAllocation'];
export type ReturnsAllocationRow = components['schemas']['ReturnsAllocationRow'];
export type ReturnsSeriesPoint = components['schemas']['ReturnsSeriesPoint'];
export type ReturnsAttributionFallback = components['schemas']['ReturnsAttributionFallback'];
export type ReturnsBenchmark = components['schemas']['ReturnsBenchmark'];
export type InstrumentReturns = components['schemas']['InstrumentReturns'];
export type InstrumentReturnsRow = components['schemas']['InstrumentReturnsRow'];
export type BenchmarkBasis = ReturnsBenchmark['basis'];
export type GainsStatementRow = components['schemas']['GainsStatementRow'];
export type GainsStatementValuation = components['schemas']['GainsStatementValuation'];
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
export type InvestmentBasisResolutionRequest = components['schemas']['InvestmentBasisResolutionRequest'];
export type InvestmentBasisResolutionResponse = components['schemas']['InvestmentBasisResolutionResponse'];
export type InvestmentBasisResolutionReplacementResponse = components['schemas']['InvestmentBasisResolutionReplacementResponse'];
export type InvestmentBasisResolutionReversalResponse = components['schemas']['InvestmentBasisResolutionReversalResponse'];
export type InvestmentCorrectionBasisResolutionTerms = components['schemas']['InvestmentCorrectionBasisResolutionTerms'];
export type InvestmentBasisResolutionHistoryEntry = components['schemas']['InvestmentBasisResolutionHistoryEntry'];
export type InvestmentTransferReplacementResponse = components['schemas']['InvestmentTransferReplacementResponse'];
export type InvestmentCorrectionTransferTerms = components['schemas']['InvestmentCorrectionTransferTerms'];
export type InvestmentSaleReplacementResponse = components['schemas']['InvestmentSaleReplacementResponse'];
export type InvestmentDividendReplacementRequest = components['schemas']['InvestmentDividendReplacementRequest'];
export type InvestmentDividendReplacementResponse = components['schemas']['InvestmentDividendReplacementResponse'];
export type PaymentInLieuRequest = components['schemas']['PaymentInLieuRequest'];
export type InvestmentPaymentInLieuReplacementRequest = components['schemas']['InvestmentPaymentInLieuReplacementRequest'];
export type InvestmentCorrectionPaymentInLieuTerms = components['schemas']['InvestmentCorrectionPaymentInLieuTerms'];
export type InvestmentReinvestmentReplacementRequest = components['schemas']['InvestmentReinvestmentReplacementRequest'];
export type InvestmentCorrectionDividendTerms = components['schemas']['InvestmentCorrectionDividendTerms'];
export type InvestmentCorrectionReinvestmentTerms = components['schemas']['InvestmentCorrectionReinvestmentTerms'];

export const investmentPositionsQueryKey = ['api', 'investments', 'positions'] as const;
export const cashInLieuLotsQueryKey = ['api', 'investments', 'cash-in-lieu-lots'] as const;
export const datedHoldingsQueryKey = ['api', 'investments', 'dated-holdings'] as const;
export const investmentLotsQueryKey = ['api', 'investments', 'lots'] as const;
export const investmentInstrumentsQueryKey = ['api', 'investments', 'instruments'] as const;
export const investmentGainsQueryKey = ['api', 'investments', 'gains'] as const;
export const investmentGainsStatementQueryKey = ['api', 'investments', 'gains', 'statement'] as const;
export const investmentReturnsQueryKey = ['api', 'investments', 'returns'] as const;
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

export type DatedHolding = components['schemas']['DatedHolding'];

// Holdings and lots as an entry dated asOf finds them, including holdings a
// later sale closed (#166). One request per selector.
export async function getDatedHoldings(asOf: string): Promise<DatedHolding[]> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/dated-holdings', {
      params: { query: { as_of: asOf } }
    });
    if (data !== undefined) return data.holdings;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export function datedHoldingsQueryOptions(asOf: string) {
  return {
    queryKey: [...datedHoldingsQueryKey, asOf] as const,
    queryFn: () => getDatedHoldings(asOf),
    enabled: /^\d{4}-\d{2}-\d{2}$/.test(asOf),
    // Keep the previous date's list while the next loads, so editing the
    // date never swaps the form for a loading state.
    placeholderData: (previous: DatedHolding[] | undefined) => previous,
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
  csrfToken: string, requestKey: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/buy', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<ExternalTransferInResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transfers/external/in', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<InternalTransferResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transfers/internal', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<ExternalTransferOutResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/transfers/external/out', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<InvestmentSplitResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/splits', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Share exchange (#177/#178): the whole holding of one instrument becomes
// another at an exact ratio, carrying each lot's basis and original date.
export async function previewShareExchange(input: ShareExchangeRequest): Promise<ShareExchangePreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/share-exchanges/preview', { body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSpinOff(input: SpinOffRequest): Promise<SpinOffPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/spin-offs/preview', { body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordSpinOff(input: SpinOffRequest, csrfToken: string, requestKey: string): Promise<SpinOffResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/spin-offs', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordShareExchange(input: ShareExchangeRequest, csrfToken: string, requestKey: string): Promise<ShareExchangeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/share-exchanges', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/sell', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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

// Named short sale and cover (#173). An opening receives like a sale and opens
// a short lot; a cover pays like a buy and consumes short lots only.
export async function recordShortSale(
  input: InvestmentTradeRequest,
  csrfToken: string, requestKey: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/short-sale', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function shortSaleReconciliationImpact(
  input: InvestmentTradeRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/short-sale/reconciliation-impact',
      { body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordShortCover(
  input: InvestmentTradeRequest,
  csrfToken: string, requestKey: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/short-cover', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewShortCover(
  input: InvestmentTradeRequest
): Promise<SellPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/short-cover/preview', {
      body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function shortCoverReconciliationImpact(
  input: InvestmentTradeRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/short-cover/reconciliation-impact',
      { body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Native short-sale and cover correction (#175).
export async function previewShortSaleReversal(transactionID: number, input: InvestmentSaleReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-short-sale/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseShortSale(transactionID: number, input: InvestmentSaleReversalRequest, csrfToken: string): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-short-sale',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewShortCoverReversal(transactionID: number, input: InvestmentSaleReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-short-cover/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseShortCover(transactionID: number, input: InvestmentSaleReversalRequest, csrfToken: string): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-short-cover',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewShortSaleReplacement(transactionID: number, input: InvestmentBuyReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-short-sale/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceShortSale(transactionID: number, input: InvestmentBuyReplacementRequest, csrfToken: string): Promise<InvestmentBuyReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-short-sale',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewShortCoverReplacement(transactionID: number, input: InvestmentSaleReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-short-cover/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceShortCover(transactionID: number, input: InvestmentSaleReplacementRequest, csrfToken: string): Promise<InvestmentSaleReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-short-cover',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
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
  csrfToken: string, requestKey: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/write-off', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/dividend', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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
  csrfToken: string, requestKey: string
): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/reinvested-dividend', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } },
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

export type AlternativeBasisMethod = NonNullable<
  NonNullable<paths['/api/v1/investments/gains']['get']['parameters']['query']>['alternative_method']
>;

export function investmentGainsQueryOptions(from?: string, to?: string, valuationAsOf?: string, alternativeMethod?: AlternativeBasisMethod, fx?: GainsFXInput, jurisdictionNote?: string) {
  return {
    queryKey: [...investmentGainsQueryKey, { from, to, valuationAsOf, alternativeMethod, fx, jurisdictionNote }] as const,
    queryFn: () => getInvestmentGains(from, to, valuationAsOf, alternativeMethod, fx, jurisdictionNote),
    staleTime: 30_000
  };
}

export function investmentGainsStatementQueryOptions(from: string, to: string, fx?: GainsFXInput) {
  return {
    queryKey: [...investmentGainsStatementQueryKey, { from, to, fx }] as const,
    queryFn: () => getInvestmentGainsStatement(from, to, fx),
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

// valuationAsOf replays unrealized positions to that date (R18 operational_v1,
// #230); omitted values today's positions.
export async function getInvestmentGains(
  from?: string,
  to?: string,
  valuationAsOf?: string,
  alternativeMethod?: AlternativeBasisMethod,
  fx?: GainsFXInput,
  jurisdictionNote?: string
): Promise<InvestmentGainsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/gains', {
      params: { query: { from, to, valuation_as_of: valuationAsOf, alternative_method: alternativeMethod, jurisdiction_note: jurisdictionNote, ...fx } }
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

// The R18 period statement (#231): opening and closing unrealized, realized
// and income per currency for from..to, composed in one backend read.
/** What a returns read asks for (R13 returns_v1, #119). Empty accountIDs
 * selects the server's default investment accounts. */
export type InvestmentReturnsInput = {
  from: string;
  to: string;
  reportingCurrencyID: number;
  accountIDs: number[];
  /** Null reads no benchmark (slice 5). */
  benchmarkCommodityID: number | null;
  benchmarkBasis: BenchmarkBasis;
  /** Adds per-instrument returns from the same snapshot (slice 9). */
  instruments: boolean;
};

// Always stale: returns read every posted ledger entry and price, and the
// writers that change them (transactions, imports, reconciliation, prices,
// investment commands) have no single invalidation hook. Each visit refetches
// while the screen keeps showing, and labelling, the cached figures.
export function investmentReturnsQueryOptions(input: InvestmentReturnsInput) {
  return {
    queryKey: [...investmentReturnsQueryKey, input] as const,
    queryFn: () => getInvestmentReturns(input),
    staleTime: 0
  };
}

export async function getInvestmentReturns(input: InvestmentReturnsInput): Promise<InvestmentReturnsResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/returns', {
      params: {
        query: {
          from: input.from,
          to: input.to,
          reporting_currency_id: input.reportingCurrencyID,
          account_id: input.accountIDs.length > 0 ? input.accountIDs : undefined,
          benchmark_commodity_id: input.benchmarkCommodityID ?? undefined,
          benchmark_basis: input.benchmarkCommodityID !== null ? input.benchmarkBasis : undefined,
          instruments: input.instruments ? true : undefined
        }
      }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getInvestmentGainsStatement(
  from: string,
  to: string,
  fx?: GainsFXInput
): Promise<InvestmentGainsStatementResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/gains/statement', {
      params: { query: { from, to, ...fx } }
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

export type EventSuggestionCompletion = components['schemas']['EventSuggestionCompletion'];
export type InvestmentEventLinkCandidate = components['schemas']['InvestmentEventLinkCandidate'];
export type DividendEventFetchAccepted = components['schemas']['DividendEventFetchAccepted'];

export async function acceptEventSuggestion(
  suggestionId: number,
  csrfToken: string,
  completion?: EventSuggestionCompletion
): Promise<InvestmentEventSuggestionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/event-suggestions/{suggestion_id}/accept',
      {
        params: { path: { suggestion_id: suggestionId }, header: { 'X-CSRF-Token': csrfToken } },
        ...(completion ? { body: { completion } } : {})
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

export async function linkEventSuggestion(
  suggestionId: number,
  transactionId: number,
  csrfToken: string
): Promise<InvestmentEventSuggestionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/event-suggestions/{suggestion_id}/link',
      {
        params: { path: { suggestion_id: suggestionId }, header: { 'X-CSRF-Token': csrfToken } },
        body: { transaction_id: transactionId }
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

export async function queueProviderEventFetch(csrfToken: string, family: 'dividend' | 'split'): Promise<DividendEventFetchAccepted> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/event-suggestions/fetch', {
      params: { header: { 'X-CSRF-Token': csrfToken } },
      body: family === 'split' ? { event_family: family } : {}
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
): Promise<components['schemas']['InvestmentSplitReversalResponse']> {
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

// Share exchange reversal and replacement (#179). Both previews run the
// actual writer in a rolled-back transaction.
export async function previewShareExchangeReversalReconciliation(
  transactionID: number,
  input: InvestmentSaleReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-share-exchange/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseShareExchange(
  transactionID: number,
  input: InvestmentSaleReversalRequest,
  csrfToken: string
): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-share-exchange',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewShareExchangeReplacement(
  transactionID: number,
  input: ShareExchangeReplacementRequest
): Promise<ShareExchangePreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-share-exchange/preview',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceShareExchange(
  transactionID: number,
  input: ShareExchangeReplacementRequest,
  csrfToken: string
): Promise<ShareExchangeReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-share-exchange',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Spin-off reversal and replacement (#183). Both previews run the actual
// writer in a rolled-back transaction.
export async function previewSpinOffReversalReconciliation(
  transactionID: number,
  input: InvestmentSaleReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-spin-off/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseSpinOff(
  transactionID: number,
  input: InvestmentSaleReversalRequest,
  csrfToken: string
): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-spin-off',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSpinOffReplacement(
  transactionID: number,
  input: SpinOffReplacementRequest
): Promise<SpinOffPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-spin-off/preview',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceSpinOff(
  transactionID: number,
  input: SpinOffReplacementRequest,
  csrfToken: string
): Promise<SpinOffReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-spin-off',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}


// Cash-and-stock merger (#181) and its reversal and replacement (#184). Every
// preview runs the actual writer in a rolled-back transaction.
export async function previewCashStockMerger(input: CashStockMergerRequest): Promise<CashStockMergerPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/cash-and-stock-mergers/preview',
      { body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function recordCashStockMerger(input: CashStockMergerRequest, csrfToken: string, requestKey: string): Promise<CashStockMergerResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/cash-and-stock-mergers',
      { params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewCashStockMergerReversalReconciliation(
  transactionID: number,
  input: InvestmentSaleReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-cash-and-stock-merger/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseCashStockMerger(
  transactionID: number,
  input: InvestmentSaleReversalRequest,
  csrfToken: string
): Promise<InvestmentSaleReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-cash-and-stock-merger',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewCashStockMergerReplacement(
  transactionID: number,
  input: CashStockMergerReplacementRequest
): Promise<CashStockMergerPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-cash-and-stock-merger/preview',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceCashStockMerger(
  transactionID: number,
  input: CashStockMergerReplacementRequest,
  csrfToken: string
): Promise<CashStockMergerReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-cash-and-stock-merger',
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

export async function recordCapitalReturn(input: CapitalReturnRequest, csrfToken: string, requestKey: string): Promise<CapitalReturnResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/return-of-capital', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } }, body: input
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

export async function recordCashInLieu(input: CashInLieuRequest, csrfToken: string, requestKey: string): Promise<InvestmentTradeResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/cash-in-lieu', { params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } }, body: input });
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

/** Preview a sourced resolution of an unknown inbound basis (T-145) through its actual writer. */
export async function previewTransferBasisResolution(transactionID: number, input: InvestmentBasisResolutionRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/resolve-basis/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Record a sourced resolution of an unknown inbound basis (T-145). */
export async function resolveTransferBasis(transactionID: number, input: InvestmentBasisResolutionRequest, csrfToken: string): Promise<InvestmentBasisResolutionResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/resolve-basis',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Preview replacing a transfer's effective sourced basis resolution (#168) through its actual writer. */
export async function previewBasisResolutionReplacement(transactionID: number, input: InvestmentBasisResolutionRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-basis-resolution/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Replace a transfer's effective sourced basis resolution with a corrected sourced basis (#168). */
export async function replaceBasisResolution(transactionID: number, input: InvestmentBasisResolutionRequest, csrfToken: string): Promise<InvestmentBasisResolutionReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-basis-resolution',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Preview withdrawing a transfer's effective sourced basis resolution (#168). */
export async function previewBasisResolutionReversal(transactionID: number, input: InvestmentSaleReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-basis-resolution/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** Withdraw a transfer's effective sourced basis resolution; its basis reads unknown again (#168). */
export async function reverseBasisResolution(transactionID: number, input: InvestmentSaleReversalRequest, csrfToken: string): Promise<InvestmentBasisResolutionReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-basis-resolution',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSplitOperationReversal(
  operationID: number,
  input: InvestmentSaleReversalRequest
): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/operations/{operation_id}/reverse-split/reconciliation-impact',
      { params: { path: { operation_id: operationID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reverseSplitOperation(
  operationID: number,
  input: InvestmentSaleReversalRequest,
  csrfToken: string
): Promise<components['schemas']['InvestmentSplitReversalResponse']> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/operations/{operation_id}/reverse-split',
      { params: { path: { operation_id: operationID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewSplitOperationReplacement(
  operationID: number,
  input: InvestmentSplitReplacementRequest
): Promise<InvestmentSplitPreviewResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/operations/{operation_id}/replace-split/preview',
      { params: { path: { operation_id: operationID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replaceSplitOperation(
  operationID: number,
  input: InvestmentSplitReplacementRequest,
  csrfToken: string
): Promise<InvestmentSplitReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/operations/{operation_id}/replace-split',
      { params: { path: { operation_id: operationID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getCashInLieuLotsForOperation(splitOperationID: number, disposalOn: string, currencyID: number, replacingTransactionID?: number): Promise<components['schemas']['CashInLieuLotsResponse']> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/operations/{operation_id}/cash-in-lieu-lots', {
      params: { path: { operation_id: splitOperationID }, query: { currency_id: currencyID, disposal_on: disposalOn, replacing_transaction_id: replacingTransactionID } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}


export const investmentSplitsQueryKey = ['investments', 'splits'] as const;
export type InvestmentSplitActivity = components['schemas']['InvestmentSplitActivity'];
export async function getSplitActivity(cursor?: string, operationID?: number): Promise<components['schemas']['InvestmentSplitActivityResponse']> {
 try {
  const { data, error, response } = await apiClient.GET('/api/v1/investments/splits', { params: { query: { cursor, operation_id: operationID } } });
  if (data !== undefined) return data;
  throw toAPIClientError(response,error);
 } catch(error) { if(error instanceof APIClientError)throw error;throw toNetworkError(error); }
}

export type GainsRunRequest = components['schemas']['GainsRunRequest'];
export type GainsRunResponse = components['schemas']['GainsRunResponse'];
export type GainsRunListResponse = components['schemas']['GainsRunListResponse'];
export const gainsRunsQueryKey = ['investments', 'gains-runs'] as const;
export const gainsRunQueryKey = (id: number) => ['investments', 'gains-run', id] as const;

export async function saveGainsRun(body: GainsRunRequest, csrfToken: string): Promise<GainsRunResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/gains/runs', {
      params: { header: { 'X-CSRF-Token': csrfToken } }, body
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getGainsRun(id: number): Promise<GainsRunResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/gains/runs/{id}', { params: { path: { id } } });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getGainsRuns(cursor?: string): Promise<GainsRunListResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/gains/runs', { params: { query: { cursor } } });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Saved returns runs (R13 slice 6, #119): immutable returns_v1 documents.
// A saved output never changes, so it is cached indefinitely by id.
export type ReturnsRunRequest = components['schemas']['ReturnsRunRequest'];
export type ReturnsRunResponse = components['schemas']['ReturnsRunResponse'];
export type ReturnsRunListResponse = components['schemas']['ReturnsRunListResponse'];
export const returnsRunsQueryKey = ['investments', 'returns-runs'] as const;
export const returnsRunQueryKey = (id: number) => ['investments', 'returns-run', id] as const;

export async function saveReturnsRun(body: ReturnsRunRequest, csrfToken: string): Promise<ReturnsRunResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/returns/runs', {
      params: { header: { 'X-CSRF-Token': csrfToken } }, body
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getReturnsRun(id: number): Promise<ReturnsRunResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/returns/runs/{id}', { params: { path: { id } } });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function getReturnsRuns(cursor?: string): Promise<ReturnsRunListResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/returns/runs', { params: { query: { cursor } } });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

// Income attribution coverage (#244): one composed read per view, never a
// fetch per instrument or payment.
export type IncomeCoverageResponse = components['schemas']['IncomeCoverageResponse'];
export type IncomeCoverageInstrument = components['schemas']['IncomeCoverageInstrument'];
export type IncomeCoverageTotal = components['schemas']['IncomeCoverageTotal'];
export type IncomeCoverageItem = components['schemas']['IncomeCoverageItem'];
export const incomeCoverageQueryKey = ['api', 'investments', 'income-coverage'] as const;

export async function getIncomeCoverage(params: { commodityID?: number; cursor?: string; limit?: number } = {}): Promise<IncomeCoverageResponse> {
  try {
    const { data, error, response } = await apiClient.GET('/api/v1/investments/income-coverage', {
      params: { query: { commodity_id: params.commodityID, cursor: params.cursor, limit: params.limit } }
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

/** An instrument's income beside the book's unassigned scope, without items. */
export function instrumentIncomeCoverageQueryOptions(commodityID: number) {
  return {
    queryKey: [...incomeCoverageQueryKey, { commodityID }] as const,
    queryFn: () => getIncomeCoverage({ commodityID, limit: 0 }),
    staleTime: 30_000
  };
}

// A short holder's payment in lieu of a dividend and its corrections (#255).

export async function recordPaymentInLieu(input: PaymentInLieuRequest, csrfToken: string, requestKey: string): Promise<TransactionResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/payment-in-lieu', {
      params: { header: { 'X-CSRF-Token': csrfToken, 'Idempotency-Key': requestKey } }, body: input
    });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function paymentInLieuReconciliationImpact(input: PaymentInLieuRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST('/api/v1/investments/payment-in-lieu/reconciliation-impact', { body: input });
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewPaymentInLieuReversalReconciliation(transactionID: number, input: InvestmentBuyReversalRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-payment-in-lieu/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function reversePaymentInLieu(transactionID: number, input: InvestmentBuyReversalRequest, csrfToken: string): Promise<InvestmentBuyReversalResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/reverse-payment-in-lieu',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function previewPaymentInLieuReplacementReconciliation(transactionID: number, input: InvestmentPaymentInLieuReplacementRequest): Promise<ReconciliationImpactResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-payment-in-lieu/reconciliation-impact',
      { params: { path: { transaction_id: transactionID } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}

export async function replacePaymentInLieu(transactionID: number, input: InvestmentPaymentInLieuReplacementRequest, csrfToken: string): Promise<InvestmentDividendReplacementResponse> {
  try {
    const { data, error, response } = await apiClient.POST(
      '/api/v1/investments/transactions/{transaction_id}/replace-payment-in-lieu',
      { params: { path: { transaction_id: transactionID }, header: { 'X-CSRF-Token': csrfToken } }, body: input }
    );
    if (data !== undefined) return data;
    throw toAPIClientError(response, error);
  } catch (error) {
    if (error instanceof APIClientError) throw error;
    throw toNetworkError(error);
  }
}
