package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type reconciliationImpactResponse struct {
	AffectedCheckpoints []checkpointImpactResponse `json:"affected_checkpoints"`
}

type checkpointImpactResponse struct {
	AccountID   int64  `json:"account_id"`
	CommodityID int64  `json:"commodity_id"`
	EntryDate   string `json:"entry_date"`
}

func toReconciliationImpactResponse(impact app.ReconciliationImpact) reconciliationImpactResponse {
	out := make([]checkpointImpactResponse, 0, len(impact.AffectedCheckpoints))
	for _, ref := range impact.AffectedCheckpoints {
		out = append(out, checkpointImpactResponse{
			AccountID:   ref.AccountID,
			CommodityID: ref.CommodityID,
			EntryDate:   ref.EntryDate,
		})
	}
	return reconciliationImpactResponse{AffectedCheckpoints: out}
}

func createReconciliationImpact(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		impact, err := transactionService.ReconciliationImpactForCreate(r.Context(), app.CreateReconciliationImpactInput{
			OwnerUserID: owner.ID,
			Spec:        toTransactionInput(request),
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "reconciliation impact", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationImpactResponse(impact))
	}
}

func updateReconciliationImpact(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readTransactionID(w, r)
		if !ok {
			return
		}
		var request transactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		impact, err := transactionService.ReconciliationImpactForUpdate(r.Context(), app.UpdateReconciliationImpactInput{
			OwnerUserID:   owner.ID,
			TransactionID: transactionID,
			Spec:          toTransactionInput(request),
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "reconciliation impact", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationImpactResponse(impact))
	}
}
