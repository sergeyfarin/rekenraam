package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type cashInLieuRequest struct {
	SplitTransactionID        int64                            `json:"split_transaction_id"`
	DisposalOn                string                           `json:"disposal_on"`
	PaymentOn                 string                           `json:"payment_on"`
	QuantityValue             exact.Coefficient                `json:"quantity_value"`
	QuantityScale             int                              `json:"quantity_scale"`
	CashAccountID             int64                            `json:"cash_account_id"`
	CurrencyID                int64                            `json:"currency_id"`
	ProceedsValue             moneyCoefficient                 `json:"proceeds_value"`
	ProceedsScale             int                              `json:"proceeds_scale"`
	CostBasisMethod           string                           `json:"cost_basis_method,omitempty"`
	LotAllocations            []investmentLotAllocationRequest `json:"lot_allocations,omitempty"`
	Memo                      string                           `json:"memo"`
	ChangeReason              string                           `json:"change_reason"`
	ReconciliationOverride    bool                             `json:"reconciliation_override"`
	GainImpactAcknowledgement string                           `json:"gain_impact_acknowledgement,omitempty"`
}

type cashInLieuPreviewResponse struct {
	DisposedBasisValue exact.Coefficient               `json:"disposed_basis_value"`
	DisposedBasisScale int                             `json:"disposed_basis_scale"`
	RealizedGainValue  exact.Coefficient               `json:"realized_gain_value"`
	RealizedGainScale  int                             `json:"realized_gain_scale"`
	CostBasisMethod    string                          `json:"cost_basis_method"`
	Allocations        []investmentLotDisposalResponse `json:"allocations"`
	Impact             reconciliationImpactResponse    `json:"impact"`
}

func cashInLieuInput(owner app.Owner, r *http.Request, request cashInLieuRequest) app.CashInLieuInput {
	allocations := make([]app.InvestmentLotAllocationInput, 0, len(request.LotAllocations))
	for _, allocation := range request.LotAllocations {
		allocations = append(allocations, app.InvestmentLotAllocationInput{LotID: allocation.LotID,
			QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	return app.CashInLieuInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		SplitTransactionID: request.SplitTransactionID, DisposalOn: request.DisposalOn, PaymentOn: request.PaymentOn,
		QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		CashAccountID: request.CashAccountID, CurrencyID: request.CurrencyID,
		ProceedsValue: int64(request.ProceedsValue), ProceedsScale: request.ProceedsScale,
		CostBasisMethod: request.CostBasisMethod, LotAllocations: allocations,
		Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func cashInLieu(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request cashInLieuRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.CashInLieu(r.Context(), cashInLieuInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "cash in lieu", err)
			return
		}
		writeJSON(w, http.StatusCreated, toInvestmentTradeResponse(result))
	}))
}

func cashInLieuPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request cashInLieuRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, impact, err := investmentService.PreviewCashInLieu(r.Context(), cashInLieuInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "cash in lieu preview", err)
			return
		}
		response := toReconciliationImpactResponse(impact)
		gainImpact, err := toGainImpactResponse(impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		response.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, cashInLieuPreviewResponse{
			Allocations: toInvestmentLotDisposalResponses(result.Allocations), Impact: response,
			DisposedBasisValue: result.DisposalDecision.DisposedBasisValue, DisposedBasisScale: result.DisposalDecision.DisposedBasisScale, RealizedGainValue: result.RealizedGainValue, RealizedGainScale: result.RealizedGainScale, CostBasisMethod: result.DisposalDecision.CostBasisMethod,
		})
	}
}

type cashInLieuReplacementRequest struct {
	Reason                    string            `json:"reason"`
	Replacement               cashInLieuRequest `json:"replacement"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

func cashInLieuCorrectionInput(owner app.Owner, r *http.Request, id int64, request cashInLieuReplacementRequest) app.ReplaceCashInLieuInput {
	return app.ReplaceCashInLieuInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), TransactionID: id, Reason: request.Reason, Replacement: cashInLieuInput(owner, r, request.Replacement), ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement}
}

func replaceCashInLieu(logger *slog.Logger, auth *app.AuthService, service *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		id, request, ok := correctionRoute[cashInLieuReplacementRequest](w, r)
		if !ok {
			return
		}
		result, err := service.ReplaceCashInLieu(r.Context(), cashInLieuCorrectionInput(owner, r, id, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace cash in lieu", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReplacementResponse{InverseTransaction: toTransactionResponse(result.Inverse), Replacement: toInvestmentTradeResponse(result.Replacement), CorrectedTransactionID: id})
	}))
}

func replaceCashInLieuPreview(logger *slog.Logger, auth *app.AuthService, service *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		id, request, ok := correctionRoute[cashInLieuReplacementRequest](w, r)
		if !ok {
			return
		}
		result, impact, err := service.PreviewCashInLieuReplacement(r.Context(), cashInLieuCorrectionInput(owner, r, id, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview cash in lieu replacement", err)
			return
		}
		response := toReconciliationImpactResponse(impact)
		response.GainImpact, err = toGainImpactResponse(impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact exceeds coefficient range")
			return
		}
		writeJSON(w, http.StatusOK, cashInLieuPreviewResponse{Allocations: toInvestmentLotDisposalResponses(result.Allocations), Impact: response, DisposedBasisValue: result.DisposalDecision.DisposedBasisValue, DisposedBasisScale: result.DisposalDecision.DisposedBasisScale, RealizedGainValue: result.RealizedGainValue, RealizedGainScale: result.RealizedGainScale, CostBasisMethod: result.DisposalDecision.CostBasisMethod})
	}
}

func reverseCashInLieu(logger *slog.Logger, auth *app.AuthService, service *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		id, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		result, err := service.ReverseCashInLieu(r.Context(), app.ReverseInvestmentSaleInput{OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()), TransactionID: id, Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse cash in lieu", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(result), CorrectedTransactionID: id})
	}))
}

func reverseCashInLieuReconciliationImpact(logger *slog.Logger, auth *app.AuthService, service *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		id, request, ok := correctionRoute[investmentSaleReversalRequest](w, r)
		if !ok {
			return
		}
		impact, err := service.ReverseCashInLieuReconciliationImpact(r.Context(), app.ReverseInvestmentSaleInput{OwnerUserID: owner.ID, TransactionID: id, Reason: request.Reason})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview cash in lieu reversal", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

type cashInLieuLotsResponse struct {
	Lots []investmentTradeCorrectionAvailableLotResponse `json:"lots"`
}

func cashInLieuLots(logger *slog.Logger, auth *app.AuthService, service *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		id, ok := readPathInt64(w, r, "transaction_id", "split transaction id")
		if !ok {
			return
		}
		currency, err := strconv.ParseInt(r.URL.Query().Get("currency_id"), 10, 64)
		if err != nil || currency <= 0 {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "currency is required")
			return
		}
		var replacing int64
		if value := r.URL.Query().Get("replacing_transaction_id"); value != "" {
			replacing, err = strconv.ParseInt(value, 10, 64)
			if err != nil || replacing <= 0 {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "replacement id is invalid")
				return
			}
		}
		lots, err := service.CashInLieuAvailableLots(r.Context(), owner.ID, id, currency, r.URL.Query().Get("disposal_on"), replacing)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read cash in lieu dated lots", err)
			return
		}
		out := make([]investmentTradeCorrectionAvailableLotResponse, 0, len(lots))
		for _, lot := range lots {
			out = append(out, investmentTradeCorrectionAvailableLotResponse(lot))
		}
		writeJSON(w, http.StatusOK, cashInLieuLotsResponse{Lots: out})
	}
}
