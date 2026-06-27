package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type accountResponse struct {
	ID                    int64           `json:"id"`
	BookID                int64           `json:"book_id"`
	IsSystem              bool            `json:"is_system"`
	SystemRole            string          `json:"system_role,omitempty"`
	Status                string          `json:"status"`
	Code                  string          `json:"code,omitempty"`
	Name                  string          `json:"name,omitempty"`
	AccountClass          string          `json:"account_class"`
	AccountKind           string          `json:"account_kind"`
	ParentAccountID       *int64          `json:"parent_account_id,omitempty"`
	InstitutionID         *int64          `json:"institution_id,omitempty"`
	CountryCode           string          `json:"country_code,omitempty"`
	DefaultCommodityID    *int64          `json:"default_commodity_id,omitempty"`
	QuantityScaleOverride *int            `json:"quantity_scale_override,omitempty"`
	AllowsPostings        bool            `json:"allows_postings"`
	HasActivity           bool            `json:"has_activity"`
	NumberLast4           string          `json:"number_last4,omitempty"`
	ExternalRefHint       string          `json:"external_ref_hint,omitempty"`
	CommentMarkdown       string          `json:"comment_markdown"`
	Metadata              json.RawMessage `json:"metadata"`
	OpenedOn              string          `json:"opened_on"`
	ClosedOn              string          `json:"closed_on,omitempty"`
	EffectiveFrom         string          `json:"effective_from"`
	ChangeReason          string          `json:"change_reason"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
	CostBasisMethod       string          `json:"cost_basis_method,omitempty"`
}

type accountsResponse struct {
	Accounts []accountResponse `json:"accounts"`
}

type accountRequest struct {
	Code                  string          `json:"code"`
	Name                  string          `json:"name"`
	AccountClass          string          `json:"account_class"`
	AccountKind           string          `json:"account_kind"`
	ParentAccountID       *int64          `json:"parent_account_id"`
	InstitutionID         *int64          `json:"institution_id"`
	CountryCode           string          `json:"country_code"`
	DefaultCommodityID    *int64          `json:"default_commodity_id"`
	DefaultCommodityCode  string          `json:"default_commodity_code"`
	QuantityScaleOverride *int            `json:"quantity_scale_override"`
	AllowsPostings        *bool           `json:"allows_postings"`
	NumberLast4           string          `json:"number_last4"`
	ExternalRefHint       string          `json:"external_ref_hint"`
	CommentMarkdown       string          `json:"comment_markdown"`
	Metadata              json.RawMessage `json:"metadata"`
	OpenedOn              string          `json:"opened_on"`
	EffectiveFrom         string          `json:"effective_from"`
	ChangeReason          string          `json:"change_reason"`
	CostBasisMethod       string          `json:"cost_basis_method"`
}

type accountLifecycleRequest struct {
	EffectiveFrom string `json:"effective_from"`
	ClosedOn      string `json:"closed_on"`
	ChangeReason  string `json:"change_reason"`
}

type completeSystemAccountsSetupResponse struct {
	Accounts []accountResponse   `json:"accounts"`
	Setup    setupStatusResponse `json:"setup"`
}

func listAccounts(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		includeArchived, err := parseOptionalBool(r.URL.Query().Get("include_archived"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_archived is invalid")
			return
		}
		includeSystem, err := parseOptionalBool(r.URL.Query().Get("include_system"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_system is invalid")
			return
		}

		accounts, err := accountService.ListAccounts(r.Context(), app.ListAccountsInput{
			Status:          r.URL.Query().Get("status"),
			AccountClass:    r.URL.Query().Get("account_class"),
			IncludeArchived: includeArchived,
			IncludeSystem:   includeSystem,
			Query:           r.URL.Query().Get("q"),
		})
		if err != nil {
			writeAccountServiceError(w, r, logger, "list accounts", err)
			return
		}

		writeJSON(w, http.StatusOK, accountsResponse{Accounts: toAccountResponses(accounts)})
	}
}

func readAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}

		account, err := accountService.Account(r.Context(), accountID)
		if err != nil {
			writeAccountServiceError(w, r, logger, "read account", err)
			return
		}

		writeJSON(w, http.StatusOK, toAccountResponse(account))
	}
}

func listAccountVersions(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}

		versions, err := accountService.AccountVersions(r.Context(), accountID)
		if err != nil {
			writeAccountServiceError(w, r, logger, "list account versions", err)
			return
		}

		writeJSON(w, http.StatusOK, accountsResponse{Accounts: toAccountResponses(versions)})
	}
}

func createAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request accountRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		account, err := accountService.CreateAccount(r.Context(), app.CreateAccountInput{
			OwnerUserID:           owner.ID,
			AuthSessionID:         authenticatedSessionID(r),
			RequestID:             RequestIDFromContext(r.Context()),
			OriginType:            "browser_api",
			Operation:             "account.create",
			Code:                  request.Code,
			Name:                  request.Name,
			AccountClass:          request.AccountClass,
			AccountKind:           request.AccountKind,
			ParentAccountID:       request.ParentAccountID,
			InstitutionID:         request.InstitutionID,
			CountryCode:           request.CountryCode,
			DefaultCommodityID:    request.DefaultCommodityID,
			DefaultCommodityCode:  request.DefaultCommodityCode,
			QuantityScaleOverride: request.QuantityScaleOverride,
			AllowsPostings:        request.AllowsPostings,
			NumberLast4:           request.NumberLast4,
			ExternalRefHint:       request.ExternalRefHint,
			CommentMarkdown:       request.CommentMarkdown,
			MetadataJSON:          rawJSONText(request.Metadata),
			OpenedOn:              request.OpenedOn,
			EffectiveFrom:         request.EffectiveFrom,
			ChangeReason:          request.ChangeReason,
			CostBasisMethod:       request.CostBasisMethod,
		})
		if err != nil {
			writeAccountServiceError(w, r, logger, "create account", err)
			return
		}

		writeJSON(w, http.StatusCreated, toAccountResponse(account))
	}))
}

func updateAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}

		var request accountRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		account, err := accountService.UpdateAccount(r.Context(), app.UpdateAccountInput{
			OwnerUserID:           owner.ID,
			AuthSessionID:         authenticatedSessionID(r),
			RequestID:             RequestIDFromContext(r.Context()),
			OriginType:            "browser_api",
			Operation:             "account.update",
			AccountID:             accountID,
			Code:                  request.Code,
			Name:                  request.Name,
			AccountClass:          request.AccountClass,
			AccountKind:           request.AccountKind,
			ParentAccountID:       request.ParentAccountID,
			InstitutionID:         request.InstitutionID,
			CountryCode:           request.CountryCode,
			DefaultCommodityID:    request.DefaultCommodityID,
			DefaultCommodityCode:  request.DefaultCommodityCode,
			QuantityScaleOverride: request.QuantityScaleOverride,
			AllowsPostings:        request.AllowsPostings,
			NumberLast4:           request.NumberLast4,
			ExternalRefHint:       request.ExternalRefHint,
			CommentMarkdown:       request.CommentMarkdown,
			MetadataJSON:          rawJSONText(request.Metadata),
			OpenedOn:              request.OpenedOn,
			EffectiveFrom:         request.EffectiveFrom,
			ChangeReason:          request.ChangeReason,
			CostBasisMethod:       request.CostBasisMethod,
		})
		if err != nil {
			writeAccountServiceError(w, r, logger, "update account", err)
			return
		}

		writeJSON(w, http.StatusOK, toAccountResponse(account))
	}))
}

func closeAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return accountLifecycleMutation(logger, authService, accountService, options, "close account", func(owner app.Owner, accountID int64, request accountLifecycleRequest, r *http.Request) (app.Account, error) {
		return accountService.CloseAccount(r.Context(), app.AccountLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			AccountID:     accountID,
			EffectiveFrom: request.EffectiveFrom,
			ClosedOn:      request.ClosedOn,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func reopenAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return accountLifecycleMutation(logger, authService, accountService, options, "reopen account", func(owner app.Owner, accountID int64, request accountLifecycleRequest, r *http.Request) (app.Account, error) {
		return accountService.ReopenAccount(r.Context(), app.AccountLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			AccountID:     accountID,
			EffectiveFrom: request.EffectiveFrom,
			ClosedOn:      request.ClosedOn,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func archiveAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return accountLifecycleMutation(logger, authService, accountService, options, "archive account", func(owner app.Owner, accountID int64, request accountLifecycleRequest, r *http.Request) (app.Account, error) {
		return accountService.ArchiveAccount(r.Context(), app.AccountLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			AccountID:     accountID,
			EffectiveFrom: request.EffectiveFrom,
			ClosedOn:      request.ClosedOn,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func restoreAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return accountLifecycleMutation(logger, authService, accountService, options, "restore account", func(owner app.Owner, accountID int64, request accountLifecycleRequest, r *http.Request) (app.Account, error) {
		return accountService.RestoreAccount(r.Context(), app.AccountLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			AccountID:     accountID,
			EffectiveFrom: request.EffectiveFrom,
			ClosedOn:      request.ClosedOn,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func deleteAccount(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}

		if err := accountService.DeleteUnusedAccount(r.Context(), app.DeleteAccountInput{
			OwnerUserID: owner.ID,
			AccountID:   accountID,
		}); err != nil {
			writeAccountServiceError(w, r, logger, "delete account", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func accountLifecycleMutation(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions, action string, mutate func(app.Owner, int64, accountLifecycleRequest, *http.Request) (app.Account, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		accountID, ok := readAccountID(w, r)
		if !ok {
			return
		}

		var request accountLifecycleRequest
		if err := decodeOptionalJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		account, err := mutate(owner, accountID, request, r)
		if err != nil {
			writeAccountServiceError(w, r, logger, action, err)
			return
		}

		writeJSON(w, http.StatusOK, toAccountResponse(account))
	}))
}

func completeSystemAccountsSetup(logger *slog.Logger, authService *app.AuthService, accountService *app.AccountService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		result, err := accountService.EnsureSystemAccounts(r.Context(), app.EnsureSystemAccountsInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "setup",
			Operation:     "system_accounts.setup",
		})
		if err != nil {
			writeAccountServiceError(w, r, logger, "complete system accounts setup", err)
			return
		}

		writeJSON(w, http.StatusCreated, completeSystemAccountsSetupResponse{
			Accounts: toAccountResponses(result.Accounts),
			Setup:    toSetupStatusResponse(result.SetupStatus),
		})
	}))
}

func readAccountID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	accountID, err := strconv.ParseInt(r.PathValue("account_id"), 10, 64)
	if err != nil || accountID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "account id is invalid")
		return 0, false
	}

	return accountID, true
}

func writeAccountServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrAccountNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "account not found")
	case errors.Is(err, app.ErrAccountParentInvalid):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "account parent is invalid")
	case errors.Is(err, app.ErrAccountReferenceInvalid):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "account reference is invalid")
	case errors.Is(err, app.ErrAccountCurrencyLocked):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "account currency cannot be changed after postings exist")
	case errors.Is(err, app.ErrAccountStructureLocked):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "account structure cannot be changed after postings exist")
	case errors.Is(err, app.ErrAccountInvalidLifecycle):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "account lifecycle transition is invalid")
	case errors.Is(err, app.ErrAccountDeleteNotAllowed):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "account cannot be deleted while it has postings or references")
	case errors.Is(err, app.ErrSystemAccountProtected):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "system account cannot be changed")
	case errors.Is(err, app.ErrSetupAlreadyComplete):
		writeAPIError(w, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "system accounts setup is already complete")
	default:
		logger.ErrorContext(r.Context(), action, slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func toAccountResponse(account app.Account) accountResponse {
	return accountResponse{
		ID:                    account.ID,
		BookID:                account.BookID,
		IsSystem:              account.IsSystem,
		SystemRole:            account.SystemRole,
		Status:                account.Status,
		Code:                  account.Code,
		Name:                  account.Name,
		AccountClass:          account.AccountClass,
		AccountKind:           account.AccountKind,
		ParentAccountID:       account.ParentAccountID,
		InstitutionID:         account.InstitutionID,
		CountryCode:           account.CountryCode,
		DefaultCommodityID:    account.DefaultCommodityID,
		QuantityScaleOverride: account.QuantityScaleOverride,
		AllowsPostings:        account.AllowsPostings,
		HasActivity:           account.HasActivity,
		NumberLast4:           account.NumberLast4,
		ExternalRefHint:       account.ExternalRefHint,
		CommentMarkdown:       account.CommentMarkdown,
		Metadata:              json.RawMessage(account.MetadataJSON),
		OpenedOn:              account.OpenedOn,
		ClosedOn:              account.ClosedOn,
		EffectiveFrom:         account.EffectiveFrom,
		ChangeReason:          account.ChangeReason,
		CreatedAt:             account.CreatedAt,
		UpdatedAt:             account.UpdatedAt,
		CostBasisMethod:       account.CostBasisMethod,
	}
}

func toAccountResponses(accounts []app.Account) []accountResponse {
	responses := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		responses = append(responses, toAccountResponse(account))
	}

	return responses
}
