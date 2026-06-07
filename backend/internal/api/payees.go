package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type payeeResponse struct {
	ID                       int64           `json:"id"`
	BookID                   int64           `json:"book_id"`
	Status                   string          `json:"status"`
	Name                     string          `json:"name"`
	DefaultAccountID         *int64          `json:"default_account_id,omitempty"`
	DefaultCategoryAccountID *int64          `json:"default_category_account_id,omitempty"`
	Metadata                 json.RawMessage `json:"metadata"`
	EffectiveFrom            string          `json:"effective_from"`
	ChangeReason             string          `json:"change_reason"`
	CreatedAt                string          `json:"created_at"`
	UpdatedAt                string          `json:"updated_at"`
}

type payeesResponse struct {
	Payees []payeeResponse `json:"payees"`
}

type payeeRequest struct {
	Name                     *string         `json:"name"`
	DefaultAccountID         *int64          `json:"default_account_id"`
	ClearDefaultAccount      bool            `json:"clear_default_account"`
	DefaultCategoryAccountID *int64          `json:"default_category_account_id"`
	ClearDefaultCategory     bool            `json:"clear_default_category"`
	Metadata                 json.RawMessage `json:"metadata"`
	EffectiveFrom            string          `json:"effective_from"`
	ChangeReason             string          `json:"change_reason"`
}

type payeeLifecycleRequest struct {
	EffectiveFrom string `json:"effective_from"`
	ChangeReason  string `json:"change_reason"`
}

func listPayees(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		includeArchived, err := parseOptionalBool(r.URL.Query().Get("include_archived"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_archived is invalid")
			return
		}

		payees, err := payeeService.ListPayees(r.Context(), app.ListPayeesInput{
			Status:          r.URL.Query().Get("status"),
			IncludeArchived: includeArchived,
			Query:           r.URL.Query().Get("q"),
		})
		if err != nil {
			writePayeeServiceError(w, r, logger, "list payees", err)
			return
		}

		writeJSON(w, http.StatusOK, payeesResponse{Payees: toPayeeResponses(payees)})
	}
}

func readPayee(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		payeeID, ok := readPayeeID(w, r)
		if !ok {
			return
		}

		payee, err := payeeService.Payee(r.Context(), payeeID)
		if err != nil {
			writePayeeServiceError(w, r, logger, "read payee", err)
			return
		}

		writeJSON(w, http.StatusOK, toPayeeResponse(payee))
	}
}

func createPayee(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request payeeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		name := ""
		if request.Name != nil {
			name = *request.Name
		}
		payee, err := payeeService.CreatePayee(r.Context(), app.CreatePayeeInput{
			OwnerUserID:              owner.ID,
			AuthSessionID:            authenticatedSessionID(r),
			RequestID:                RequestIDFromContext(r.Context()),
			OriginType:               "browser_api",
			Operation:                "payee.create",
			Name:                     name,
			DefaultAccountID:         request.DefaultAccountID,
			DefaultCategoryAccountID: request.DefaultCategoryAccountID,
			MetadataJSON:             rawJSONText(request.Metadata),
			EffectiveFrom:            request.EffectiveFrom,
			ChangeReason:             request.ChangeReason,
		})
		if err != nil {
			writePayeeServiceError(w, r, logger, "create payee", err)
			return
		}

		writeJSON(w, http.StatusCreated, toPayeeResponse(payee))
	}))
}

func updatePayee(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		payeeID, ok := readPayeeID(w, r)
		if !ok {
			return
		}

		var request payeeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		var metadata *string
		if request.Metadata != nil {
			value := rawJSONText(request.Metadata)
			metadata = &value
		}

		payee, err := payeeService.UpdatePayee(r.Context(), app.UpdatePayeeInput{
			OwnerUserID:              owner.ID,
			AuthSessionID:            authenticatedSessionID(r),
			RequestID:                RequestIDFromContext(r.Context()),
			OriginType:               "browser_api",
			Operation:                "payee.update",
			PayeeID:                  payeeID,
			Name:                     request.Name,
			DefaultAccountID:         request.DefaultAccountID,
			ClearDefaultAccount:      request.ClearDefaultAccount,
			DefaultCategoryAccountID: request.DefaultCategoryAccountID,
			ClearDefaultCategory:     request.ClearDefaultCategory,
			MetadataJSON:             metadata,
			EffectiveFrom:            request.EffectiveFrom,
			ChangeReason:             request.ChangeReason,
		})
		if err != nil {
			writePayeeServiceError(w, r, logger, "update payee", err)
			return
		}

		writeJSON(w, http.StatusOK, toPayeeResponse(payee))
	}))
}

func archivePayee(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService, options HandlerOptions) http.HandlerFunc {
	return payeeLifecycleMutation(logger, authService, payeeService, options, "archive payee", func(owner app.Owner, payeeID int64, r *http.Request, request payeeLifecycleRequest) (app.Payee, error) {
		return payeeService.ArchivePayee(r.Context(), app.PayeeLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			PayeeID:       payeeID,
			EffectiveFrom: request.EffectiveFrom,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func restorePayee(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService, options HandlerOptions) http.HandlerFunc {
	return payeeLifecycleMutation(logger, authService, payeeService, options, "restore payee", func(owner app.Owner, payeeID int64, r *http.Request, request payeeLifecycleRequest) (app.Payee, error) {
		return payeeService.RestorePayee(r.Context(), app.PayeeLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			PayeeID:       payeeID,
			EffectiveFrom: request.EffectiveFrom,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func payeeLifecycleMutation(logger *slog.Logger, authService *app.AuthService, payeeService *app.PayeeService, options HandlerOptions, action string, mutate func(app.Owner, int64, *http.Request, payeeLifecycleRequest) (app.Payee, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		payeeID, ok := readPayeeID(w, r)
		if !ok {
			return
		}

		var request payeeLifecycleRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		payee, err := mutate(owner, payeeID, r, request)
		if err != nil {
			writePayeeServiceError(w, r, logger, action, err)
			return
		}

		writeJSON(w, http.StatusOK, toPayeeResponse(payee))
	}))
}

func readPayeeID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	payeeID, err := strconv.ParseInt(r.PathValue("payee_id"), 10, 64)
	if err != nil || payeeID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "payee id is invalid")
		return 0, false
	}

	return payeeID, true
}

func writePayeeServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrPayeeNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "payee not found")
	case errors.Is(err, app.ErrPayeeExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "payee already exists")
	default:
		logger.ErrorContext(r.Context(), action, slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func toPayeeResponse(payee app.Payee) payeeResponse {
	return payeeResponse{
		ID:                       payee.ID,
		BookID:                   payee.BookID,
		Status:                   payee.Status,
		Name:                     payee.Name,
		DefaultAccountID:         payee.DefaultAccountID,
		DefaultCategoryAccountID: payee.DefaultCategoryAccountID,
		Metadata:                 json.RawMessage(payee.MetadataJSON),
		EffectiveFrom:            payee.EffectiveFrom,
		ChangeReason:             payee.ChangeReason,
		CreatedAt:                payee.CreatedAt,
		UpdatedAt:                payee.UpdatedAt,
	}
}

func toPayeeResponses(payees []app.Payee) []payeeResponse {
	responses := make([]payeeResponse, 0, len(payees))
	for _, payee := range payees {
		responses = append(responses, toPayeeResponse(payee))
	}

	return responses
}
