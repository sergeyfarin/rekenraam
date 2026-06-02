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
	LogoURL         string          `json:"logo_url,omitempty"`
	LogoSmallURL    string          `json:"logo_small_url,omitempty"`
	BackdropURL     string          `json:"backdrop_url,omitempty"`
	Address         json.RawMessage `json:"address"`
	CommentMarkdown string          `json:"comment_markdown"`
	Metadata        json.RawMessage `json:"metadata"`
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
	LogoURL         string          `json:"logo_url"`
	LogoSmallURL    string          `json:"logo_small_url"`
	BackdropURL     string          `json:"backdrop_url"`
	Address         json.RawMessage `json:"address"`
	CommentMarkdown string          `json:"comment_markdown"`
	Metadata        json.RawMessage `json:"metadata"`
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
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		institution, err := institutionService.CreateInstitution(r.Context(), app.CreateInstitutionInput{
			OwnerUserID:     owner.ID,
			Name:            request.Name,
			Kind:            request.Kind,
			CountryCode:     request.CountryCode,
			Website:         request.Website,
			LogoURL:         request.LogoURL,
			LogoSmallURL:    request.LogoSmallURL,
			BackdropURL:     request.BackdropURL,
			AddressJSON:     rawJSONText(request.Address),
			CommentMarkdown: request.CommentMarkdown,
			MetadataJSON:    rawJSONText(request.Metadata),
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
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		institution, err := institutionService.UpdateInstitution(r.Context(), app.UpdateInstitutionInput{
			OwnerUserID:     owner.ID,
			InstitutionID:   institutionID,
			Name:            request.Name,
			Kind:            request.Kind,
			CountryCode:     request.CountryCode,
			Website:         request.Website,
			LogoURL:         request.LogoURL,
			LogoSmallURL:    request.LogoSmallURL,
			BackdropURL:     request.BackdropURL,
			AddressJSON:     rawJSONText(request.Address),
			CommentMarkdown: request.CommentMarkdown,
			MetadataJSON:    rawJSONText(request.Metadata),
		})
		if err != nil {
			writeInstitutionServiceError(w, r, logger, "update institution", err)
			return
		}

		writeJSON(w, http.StatusOK, toInstitutionResponse(institution))
	}))
}

func archiveInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return institutionLifecycleMutation(logger, authService, institutionService, options, "archive institution", func(ctxOwner app.Owner, institutionID int64, r *http.Request) (app.Institution, error) {
		return institutionService.ArchiveInstitution(r.Context(), app.InstitutionLifecycleInput{
			OwnerUserID:   ctxOwner.ID,
			InstitutionID: institutionID,
		})
	})
}

func restoreInstitution(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions) http.HandlerFunc {
	return institutionLifecycleMutation(logger, authService, institutionService, options, "restore institution", func(ctxOwner app.Owner, institutionID int64, r *http.Request) (app.Institution, error) {
		return institutionService.RestoreInstitution(r.Context(), app.InstitutionLifecycleInput{
			OwnerUserID:   ctxOwner.ID,
			InstitutionID: institutionID,
		})
	})
}

func institutionLifecycleMutation(logger *slog.Logger, authService *app.AuthService, institutionService *app.InstitutionService, options HandlerOptions, action string, mutate func(app.Owner, int64, *http.Request) (app.Institution, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		institutionID, ok := readInstitutionID(w, r)
		if !ok {
			return
		}

		institution, err := mutate(owner, institutionID, r)
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
		LogoURL:         institution.LogoURL,
		LogoSmallURL:    institution.LogoSmallURL,
		BackdropURL:     institution.BackdropURL,
		Address:         json.RawMessage(institution.AddressJSON),
		CommentMarkdown: institution.CommentMarkdown,
		Metadata:        json.RawMessage(institution.MetadataJSON),
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
