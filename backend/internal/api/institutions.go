package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type institutionResponse struct {
	ID              int64           `json:"id"`
	BookID          int64           `json:"book_id"`
	Status          string          `json:"status"`
	Name            string          `json:"name"`
	Kind            string          `json:"kind"`
	CountryCode     string          `json:"country_code,omitempty"`
	Website         string          `json:"website,omitempty"`
	Address         json.RawMessage `json:"address"`
	CommentMarkdown string          `json:"comment_markdown"`
	Metadata        json.RawMessage `json:"metadata"`
	EffectiveFrom   string          `json:"effective_from"`
	ChangeReason    string          `json:"change_reason"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

type institutionsResponse struct {
	Institutions []institutionResponse `json:"institutions"`
}

type institutionRequest struct {
	Name            string          `json:"name"`
	Kind            string          `json:"kind"`
	CountryCode     string          `json:"country_code"`
	Website         string          `json:"website"`
	Address         json.RawMessage `json:"address"`
	CommentMarkdown string          `json:"comment_markdown"`
	Metadata        json.RawMessage `json:"metadata"`
	EffectiveFrom   string          `json:"effective_from"`
	ChangeReason    string          `json:"change_reason"`
}

type institutionLifecycleRequest struct {
	EffectiveFrom string `json:"effective_from"`
	ChangeReason  string `json:"change_reason"`
}

func listInstitutions(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		includeArchived, err := parseOptionalBool(r.URL.Query().Get("include_archived"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_archived is invalid")
			return
		}

		institutions, err := institutionService.ListInstitutions(r.Context(), app.ListInstitutionsInput{
			Status:          r.URL.Query().Get("status"),
			IncludeArchived: includeArchived,
			Query:           r.URL.Query().Get("q"),
		})
		if err != nil {
			writeInstitutionServiceError(w, r, logger, "list institutions", err)
			return
		}

		writeJSON(w, http.StatusOK, institutionsResponse{Institutions: toInstitutionResponses(institutions)})
	}
}

func readInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		institutionID, ok := readInstitutionID(w, r)
		if !ok {
			return
		}

		institution, err := institutionService.Institution(r.Context(), institutionID)
		if err != nil {
			writeInstitutionServiceError(w, r, logger, "read institution", err)
			return
		}

		writeJSON(w, http.StatusOK, toInstitutionResponse(institution))
	}
}

func listInstitutionVersions(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		institutionID, ok := readInstitutionID(w, r)
		if !ok {
			return
		}

		versions, err := institutionService.InstitutionVersions(r.Context(), institutionID)
		if err != nil {
			writeInstitutionServiceError(w, r, logger, "list institution versions", err)
			return
		}

		writeJSON(w, http.StatusOK, institutionsResponse{Institutions: toInstitutionResponses(versions)})
	}
}

func createInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request institutionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		institution, err := institutionService.CreateInstitution(r.Context(), app.CreateInstitutionInput{
			OwnerUserID:     owner.ID,
			AuthSessionID:   authenticatedSessionID(r),
			RequestID:       RequestIDFromContext(r.Context()),
			OriginType:      "browser_api",
			Operation:       "institution.create",
			Name:            request.Name,
			Kind:            request.Kind,
			CountryCode:     request.CountryCode,
			Website:         request.Website,
			AddressJSON:     rawJSONText(request.Address),
			CommentMarkdown: request.CommentMarkdown,
			MetadataJSON:    rawJSONText(request.Metadata),
			EffectiveFrom:   request.EffectiveFrom,
			ChangeReason:    request.ChangeReason,
		})
		if err != nil {
			writeInstitutionServiceError(w, r, logger, "create institution", err)
			return
		}

		writeJSON(w, http.StatusCreated, toInstitutionResponse(institution))
	}))
}

func updateInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		institutionID, ok := readInstitutionID(w, r)
		if !ok {
			return
		}

		var request institutionRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		institution, err := institutionService.UpdateInstitution(r.Context(), app.UpdateInstitutionInput{
			OwnerUserID:     owner.ID,
			AuthSessionID:   authenticatedSessionID(r),
			RequestID:       RequestIDFromContext(r.Context()),
			OriginType:      "browser_api",
			Operation:       "institution.update",
			InstitutionID:   institutionID,
			Name:            request.Name,
			Kind:            request.Kind,
			CountryCode:     request.CountryCode,
			Website:         request.Website,
			AddressJSON:     rawJSONText(request.Address),
			CommentMarkdown: request.CommentMarkdown,
			MetadataJSON:    rawJSONText(request.Metadata),
			EffectiveFrom:   request.EffectiveFrom,
			ChangeReason:    request.ChangeReason,
		})
		if err != nil {
			writeInstitutionServiceError(w, r, logger, "update institution", err)
			return
		}

		writeJSON(w, http.StatusOK, toInstitutionResponse(institution))
	}))
}

func archiveInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return institutionLifecycleMutation(logger, authService, institutionService, options, "archive institution", func(ctxOwner app.Owner, institutionID int64, request institutionLifecycleRequest, r *http.Request) (app.Institution, error) {
		return institutionService.ArchiveInstitution(r.Context(), app.InstitutionLifecycleInput{
			OwnerUserID:   ctxOwner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			InstitutionID: institutionID,
			EffectiveFrom: request.EffectiveFrom,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func restoreInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return institutionLifecycleMutation(logger, authService, institutionService, options, "restore institution", func(ctxOwner app.Owner, institutionID int64, request institutionLifecycleRequest, r *http.Request) (app.Institution, error) {
		return institutionService.RestoreInstitution(r.Context(), app.InstitutionLifecycleInput{
			OwnerUserID:   ctxOwner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			InstitutionID: institutionID,
			EffectiveFrom: request.EffectiveFrom,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func deleteInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		institutionID, ok := readInstitutionID(w, r)
		if !ok {
			return
		}

		if err := institutionService.DeleteUnusedInstitution(r.Context(), app.DeleteInstitutionInput{
			OwnerUserID:   owner.ID,
			InstitutionID: institutionID,
		}); err != nil {
			writeInstitutionServiceError(w, r, logger, "delete institution", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func institutionLifecycleMutation(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions, action string, mutate func(app.Owner, int64, institutionLifecycleRequest, *http.Request) (app.Institution, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		institutionID, ok := readInstitutionID(w, r)
		if !ok {
			return
		}

		var request institutionLifecycleRequest
		if err := decodeOptionalJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		institution, err := mutate(owner, institutionID, request, r)
		if err != nil {
			writeInstitutionServiceError(w, r, logger, action, err)
			return
		}

		writeJSON(w, http.StatusOK, toInstitutionResponse(institution))
	}))
}

func readInstitutionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	institutionID, err := strconv.ParseInt(r.PathValue("institution_id"), 10, 64)
	if err != nil || institutionID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "institution id is invalid")
		return 0, false
	}

	return institutionID, true
}

func writeInstitutionServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrInstitutionNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "institution not found")
	case errors.Is(err, app.ErrInstitutionDeleteNotAllowed):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "institution cannot be deleted while accounts reference it")
	default:
		logger.ErrorContext(r.Context(), action, slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func parseOptionalBool(value string) (bool, error) {
	if value == "" {
		return false, nil
	}

	return strconv.ParseBool(value)
}

func rawJSONText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	return string(raw)
}

func toInstitutionResponse(institution app.Institution) institutionResponse {
	return institutionResponse{
		ID:              institution.ID,
		BookID:          institution.BookID,
		Status:          institution.Status,
		Name:            institution.Name,
		Kind:            institution.Kind,
		CountryCode:     institution.CountryCode,
		Website:         institution.Website,
		Address:         json.RawMessage(institution.AddressJSON),
		CommentMarkdown: institution.CommentMarkdown,
		Metadata:        json.RawMessage(institution.MetadataJSON),
		EffectiveFrom:   institution.EffectiveFrom,
		ChangeReason:    institution.ChangeReason,
		CreatedAt:       institution.CreatedAt,
		UpdatedAt:       institution.UpdatedAt,
	}
}

func toInstitutionResponses(institutions []app.Institution) []institutionResponse {
	responses := make([]institutionResponse, 0, len(institutions))
	for _, institution := range institutions {
		responses = append(responses, toInstitutionResponse(institution))
	}

	return responses
}
