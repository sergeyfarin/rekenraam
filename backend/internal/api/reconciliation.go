package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type reconciliationSessionResponse struct {
	ID                   int64                           `json:"id"`
	BookID               int64                           `json:"book_id"`
	AccountID            int64                           `json:"account_id"`
	CommodityID          int64                           `json:"commodity_id"`
	Status               string                          `json:"status"`
	SourceKind           string                          `json:"source_kind"`
	StatementDate        string                          `json:"statement_date"`
	StatementBalance     balanceQuantityResponse         `json:"statement_balance"`
	StartingCheckpointID *int64                          `json:"starting_checkpoint_id,omitempty"`
	StartingBalance      balanceQuantityResponse         `json:"starting_balance"`
	SelectedBalance      balanceQuantityResponse         `json:"selected_balance"`
	Difference           balanceQuantityResponse         `json:"difference"`
	Metadata             json.RawMessage                 `json:"metadata"`
	CreatedAt            string                          `json:"created_at"`
	UpdatedAt            string                          `json:"updated_at"`
	FinishedAt           string                          `json:"finished_at,omitempty"`
	VoidedAt             string                          `json:"voided_at,omitempty"`
	ChangeReason         string                          `json:"change_reason"`
	Candidates           []reconciliationPostingResponse `json:"candidates"`
	SelectedPostings     []reconciliationPostingResponse `json:"selected_postings"`
}

type reconciliationCheckpointResponse struct {
	ID                   int64                   `json:"id"`
	BookID               int64                   `json:"book_id"`
	AccountID            int64                   `json:"account_id"`
	CommodityID          int64                   `json:"commodity_id"`
	SessionID            *int64                  `json:"session_id,omitempty"`
	PreviousCheckpointID *int64                  `json:"previous_checkpoint_id,omitempty"`
	Status               string                  `json:"status"`
	StatementDate        string                  `json:"statement_date"`
	StatementBalance     balanceQuantityResponse `json:"statement_balance"`
	CreatedAt            string                  `json:"created_at"`
	InvalidatedAt        string                  `json:"invalidated_at,omitempty"`
	InvalidationReason   string                  `json:"invalidation_reason,omitempty"`
	VoidedAt             string                  `json:"voided_at,omitempty"`
	VoidReason           string                  `json:"void_reason,omitempty"`
}

type reconciliationPostingResponse struct {
	PostingID            int64             `json:"posting_id"`
	TransactionID        int64             `json:"transaction_id"`
	TransactionVersionID int64             `json:"transaction_version_id"`
	PostingLineID        int64             `json:"posting_line_id"`
	AccountID            int64             `json:"account_id"`
	CommodityID          int64             `json:"commodity_id"`
	EntryDate            string            `json:"entry_date"`
	QuantityValue        exact.Coefficient `json:"quantity_value"`
	QuantityScale        int               `json:"quantity_scale"`
	ReconciliationStatus string            `json:"reconciliation_status"`
	PayeeName            string            `json:"payee_name,omitempty"`
	Description          string            `json:"description,omitempty"`
	Selected             bool              `json:"selected"`
}

type reconciliationCheckpointsResponse struct {
	Checkpoints []reconciliationCheckpointResponse `json:"checkpoints"`
}

type startReconciliationRequest struct {
	CommodityID           int64             `json:"commodity_id"`
	SourceKind            string            `json:"source_kind"`
	StatementDate         string            `json:"statement_date"`
	StatementBalanceValue exact.Coefficient `json:"statement_balance_value"`
	StatementBalanceScale int               `json:"statement_balance_scale"`
	Metadata              json.RawMessage   `json:"metadata"`
	ChangeReason          string            `json:"change_reason"`
}

type reconciliationSelectionRequest struct {
	PostingVersionIDs []int64 `json:"posting_version_ids"`
	ChangeReason      string  `json:"change_reason"`
}

type changePostingReconciliationStatusRequest struct {
	PostingVersionIDs []int64 `json:"posting_version_ids"`
	Status            string  `json:"status"`
	ClearedOn         string  `json:"cleared_on"`
	ChangeReason      string  `json:"change_reason"`
}

func startReconciliation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		var request startReconciliationRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		session, err := transactionService.StartReconciliation(r.Context(), app.StartReconciliationInput{
			OwnerUserID:           owner.ID,
			AuthSessionID:         authenticatedSessionID(r),
			RequestID:             RequestIDFromContext(r.Context()),
			OriginType:            "browser_api",
			AccountID:             accountID,
			CommodityID:           request.CommodityID,
			SourceKind:            request.SourceKind,
			StatementDate:         request.StatementDate,
			StatementBalanceValue: request.StatementBalanceValue,
			StatementBalanceScale: request.StatementBalanceScale,
			MetadataJSON:          rawJSONText(request.Metadata),
			ChangeReason:          request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "start reconciliation", err)
			return
		}
		writeJSON(w, http.StatusCreated, toReconciliationSessionResponse(session))
	}))
}

func readReconciliation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		sessionID, ok := readReconciliationID(w, r)
		if !ok {
			return
		}
		session, err := transactionService.ReconciliationSession(r.Context(), sessionID)
		if err != nil {
			writeTransactionServiceError(w, r, logger, "read reconciliation", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationSessionResponse(session))
	}
}

func updateReconciliationSelection(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return reconciliationSelectionMutation(logger, authService, transactionService, options, false)
}

func previewReconciliation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return reconciliationSelectionMutation(logger, authService, transactionService, options, true)
}

func reconciliationSelectionMutation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions, preview bool) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		sessionID, ok := readReconciliationID(w, r)
		if !ok {
			return
		}
		var request reconciliationSelectionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		input := app.ReconciliationSelectionInput{
			OwnerUserID:       owner.ID,
			AuthSessionID:     authenticatedSessionID(r),
			RequestID:         RequestIDFromContext(r.Context()),
			OriginType:        "browser_api",
			SessionID:         sessionID,
			PostingVersionIDs: request.PostingVersionIDs,
			ChangeReason:      request.ChangeReason,
		}
		var (
			session app.ReconciliationSession
			err     error
		)
		if preview {
			session, err = transactionService.PreviewReconciliation(r.Context(), input)
		} else {
			session, err = transactionService.UpdateReconciliationSelection(r.Context(), input)
		}
		if err != nil {
			writeTransactionServiceError(w, r, logger, "update reconciliation selection", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationSessionResponse(session))
	}))
}

func finishReconciliation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		sessionID, ok := readReconciliationID(w, r)
		if !ok {
			return
		}
		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		session, err := transactionService.FinishReconciliation(r.Context(), app.FinishReconciliationInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			SessionID:     sessionID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "finish reconciliation", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationSessionResponse(session))
	}))
}

func voidReconciliation(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		sessionID, ok := readReconciliationID(w, r)
		if !ok {
			return
		}
		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		session, err := transactionService.VoidReconciliationSession(r.Context(), app.VoidReconciliationInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			ID:            sessionID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "void reconciliation", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationSessionResponse(session))
	}))
}

func listReconciliationCheckpoints(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		commodityID, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("commodity_id"), "commodity id")
		if !ok {
			return
		}
		checkpoints, err := transactionService.ReconciliationCheckpoints(r.Context(), app.ListReconciliationCheckpointsInput{
			AccountID:   accountID,
			CommodityID: commodityID,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "list reconciliation checkpoints", err)
			return
		}
		responses := make([]reconciliationCheckpointResponse, 0, len(checkpoints))
		for _, checkpoint := range checkpoints {
			responses = append(responses, toReconciliationCheckpointResponse(checkpoint))
		}
		writeJSON(w, http.StatusOK, reconciliationCheckpointsResponse{Checkpoints: responses})
	}
}

func voidReconciliationCheckpoint(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		checkpointID, ok := readCheckpointID(w, r)
		if !ok {
			return
		}
		var request voidTransactionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		checkpoint, err := transactionService.VoidReconciliationCheckpoint(r.Context(), app.VoidReconciliationInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			ID:            checkpointID,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "void reconciliation checkpoint", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationCheckpointResponse(checkpoint))
	}))
}

func changePostingReconciliationStatus(logger *slog.Logger, authService *app.AuthService, transactionService *app.TransactionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}
		var request changePostingReconciliationStatusRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		transactions, err := transactionService.ChangePostingReconciliationStatus(r.Context(), app.ChangePostingReconciliationStatusInput{
			OwnerUserID:       owner.ID,
			AuthSessionID:     authenticatedSessionID(r),
			RequestID:         RequestIDFromContext(r.Context()),
			OriginType:        "browser_api",
			AccountID:         accountID,
			PostingVersionIDs: request.PostingVersionIDs,
			Status:            request.Status,
			ClearedOn:         request.ClearedOn,
			ChangeReason:      request.ChangeReason,
		})
		if err != nil {
			writeTransactionServiceError(w, r, logger, "change posting reconciliation status", err)
			return
		}
		writeJSON(w, http.StatusOK, transactionsResponse{Transactions: toTransactionResponses(transactions)})
	}))
}

func readReconciliationID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("reconciliation_id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "reconciliation id is invalid")
		return 0, false
	}
	return id, true
}

func readCheckpointID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("checkpoint_id"), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "checkpoint id is invalid")
		return 0, false
	}
	return id, true
}

func toReconciliationSessionResponse(session app.ReconciliationSession) reconciliationSessionResponse {
	return reconciliationSessionResponse{
		ID:                   session.ID,
		BookID:               session.BookID,
		AccountID:            session.AccountID,
		CommodityID:          session.CommodityID,
		Status:               session.Status,
		SourceKind:           session.SourceKind,
		StatementDate:        session.StatementDate,
		StatementBalance:     toBalanceQuantityResponse(session.StatementBalance),
		StartingCheckpointID: session.StartingCheckpointID,
		StartingBalance:      toBalanceQuantityResponse(session.StartingBalance),
		SelectedBalance:      toBalanceQuantityResponse(session.SelectedBalance),
		Difference:           toBalanceQuantityResponse(session.Difference),
		Metadata:             json.RawMessage(session.MetadataJSON),
		CreatedAt:            session.CreatedAt,
		UpdatedAt:            session.UpdatedAt,
		FinishedAt:           session.FinishedAt,
		VoidedAt:             session.VoidedAt,
		ChangeReason:         session.ChangeReason,
		Candidates:           toReconciliationPostingResponses(session.Candidates),
		SelectedPostings:     toReconciliationPostingResponses(session.SelectedPostings),
	}
}

func toReconciliationCheckpointResponse(checkpoint app.ReconciliationCheckpoint) reconciliationCheckpointResponse {
	return reconciliationCheckpointResponse{
		ID:                   checkpoint.ID,
		BookID:               checkpoint.BookID,
		AccountID:            checkpoint.AccountID,
		CommodityID:          checkpoint.CommodityID,
		SessionID:            checkpoint.SessionID,
		PreviousCheckpointID: checkpoint.PreviousCheckpointID,
		Status:               checkpoint.Status,
		StatementDate:        checkpoint.StatementDate,
		StatementBalance:     toBalanceQuantityResponse(checkpoint.StatementBalance),
		CreatedAt:            checkpoint.CreatedAt,
		InvalidatedAt:        checkpoint.InvalidatedAt,
		InvalidationReason:   checkpoint.InvalidationReason,
		VoidedAt:             checkpoint.VoidedAt,
		VoidReason:           checkpoint.VoidReason,
	}
}

func toReconciliationPostingResponses(postings []app.ReconciliationPosting) []reconciliationPostingResponse {
	responses := make([]reconciliationPostingResponse, 0, len(postings))
	for _, posting := range postings {
		responses = append(responses, reconciliationPostingResponse{
			PostingID:            posting.PostingID,
			TransactionID:        posting.TransactionID,
			TransactionVersionID: posting.TransactionVersionID,
			PostingLineID:        posting.PostingLineID,
			AccountID:            posting.AccountID,
			CommodityID:          posting.CommodityID,
			EntryDate:            posting.EntryDate,
			QuantityValue:        posting.QuantityValue,
			QuantityScale:        posting.QuantityScale,
			ReconciliationStatus: posting.ReconciliationStatus,
			PayeeName:            posting.PayeeName,
			Description:          posting.Description,
			Selected:             posting.Selected,
		})
	}
	return responses
}
