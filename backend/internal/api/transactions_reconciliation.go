package api

import (
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type reconciliationImpactResponse struct {
	AffectedCheckpoints []checkpointImpactResponse `json:"affected_checkpoints"`
	// GainImpact is present only for commands opted into replay gain
	// disclosure; its absence is not a claim that no gain changes.
	GainImpact *gainImpactResponse `json:"gain_impact,omitempty"`
}

type checkpointImpactResponse struct {
	CheckpointID             int64  `json:"checkpoint_id"`
	AccountID                int64  `json:"account_id"`
	AccountLabel             string `json:"account_label"`
	CommodityID              int64  `json:"commodity_id"`
	CommodityCode            string `json:"commodity_code"`
	StatementDate            string `json:"statement_date"`
	StatementAccountSequence int64  `json:"statement_account_sequence"`
	EntryDate                string `json:"entry_date"`
}

// writeReconciliationImpact writes a preview's checkpoint impact and, for an
// opted-in command, its gain impact. A gain value wider than the coefficient
// contract is an overflow, never a silently omitted disclosure.
func writeReconciliationImpact(w http.ResponseWriter, impact app.ReconciliationImpact) {
	response := toReconciliationImpactResponse(impact)
	gainImpact, err := toGainImpactResponse(impact.GainImpact)
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
		return
	}
	response.GainImpact = gainImpact
	writeJSON(w, http.StatusOK, response)
}

func toReconciliationImpactResponse(impact app.ReconciliationImpact) reconciliationImpactResponse {
	out := make([]checkpointImpactResponse, 0, len(impact.AffectedCheckpoints))
	for _, ref := range impact.AffectedCheckpoints {
		out = append(out, checkpointImpactResponse{
			CheckpointID:             ref.CheckpointID,
			AccountID:                ref.AccountID,
			AccountLabel:             ref.AccountLabel,
			CommodityID:              ref.CommodityID,
			CommodityCode:            ref.CommodityCode,
			StatementDate:            ref.StatementDate,
			StatementAccountSequence: ref.StatementAccountSequence,
			EntryDate:                ref.EntryDate,
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
			writeDecodeError(w, err)
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
		writeReconciliationImpact(w, impact)
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
			writeDecodeError(w, err)
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
		writeReconciliationImpact(w, impact)
	}
}

func postReconciliationImpact(logger *slog.Logger, auth *app.AuthService, service *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		id, ok := readTransactionID(w, r)
		if !ok {
			return
		}
		impact, err := service.ReconciliationImpactForPost(r.Context(), owner.ID, id)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "posting reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}
