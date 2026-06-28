package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

// --- Response types ---

type importConnectionResponse struct {
	ID              int64   `json:"id"`
	BookID          int64   `json:"book_id"`
	SourceKind      string  `json:"source"`
	DisplayName     string  `json:"display_name"`
	KeyHint         string  `json:"key_hint"`
	ConfigJSON      string  `json:"config"`
	LastFetchStatus string  `json:"last_fetch_status"`
	LastFetchedAt   *string `json:"last_fetched_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type listImportConnectionsResponse struct {
	Connections []importConnectionResponse `json:"connections"`
}

type createImportConnectionRequest struct {
	Source      string `json:"source"`
	DisplayName string `json:"display_name"`
	APIKey      string `json:"api_key"`
	ConfigJSON  string `json:"config,omitempty"`
}

type updateImportConnectionRequest struct {
	DisplayName string `json:"display_name"`
	NewAPIKey   string `json:"api_key,omitempty"`
	ConfigJSON  string `json:"config,omitempty"`
}

// --- Handlers ---

func listImportConnections(logger *slog.Logger, authService *app.AuthService, connectionService *app.ImportConnectionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		conns, err := connectionService.ListImportConnections(r.Context())
		if err != nil {
			writeImportConnectionServiceError(w, r, logger, "list import connections", err)
			return
		}
		resp := make([]importConnectionResponse, 0, len(conns))
		for _, c := range conns {
			resp = append(resp, toImportConnectionResponse(c))
		}
		writeJSON(w, http.StatusOK, listImportConnectionsResponse{Connections: resp})
	}
}

func createImportConnection(logger *slog.Logger, authService *app.AuthService, connectionService *app.ImportConnectionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var req createImportConnectionRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}

		conn, err := connectionService.CreateImportConnection(r.Context(), app.CreateImportConnectionInput{
			OwnerUserID: owner.ID,
			SourceKind:  req.Source,
			DisplayName: req.DisplayName,
			APIKey:      req.APIKey,
			ConfigJSON:  req.ConfigJSON,
		})
		if err != nil {
			writeImportConnectionServiceError(w, r, logger, "create import connection", err)
			return
		}

		writeJSON(w, http.StatusCreated, toImportConnectionResponse(conn))
	}))
}

func updateImportConnection(logger *slog.Logger, authService *app.AuthService, connectionService *app.ImportConnectionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		connID, ok := readImportConnectionID(w, r)
		if !ok {
			return
		}

		var req updateImportConnectionRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}

		conn, err := connectionService.UpdateImportConnection(r.Context(), app.UpdateImportConnectionInput{
			OwnerUserID: owner.ID,
			ID:          connID,
			DisplayName: req.DisplayName,
			NewAPIKey:   req.NewAPIKey,
			ConfigJSON:  req.ConfigJSON,
		})
		if err != nil {
			writeImportConnectionServiceError(w, r, logger, "update import connection", err)
			return
		}

		writeJSON(w, http.StatusOK, toImportConnectionResponse(conn))
	}))
}

func deleteImportConnection(logger *slog.Logger, authService *app.AuthService, connectionService *app.ImportConnectionService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		connID, ok := readImportConnectionID(w, r)
		if !ok {
			return
		}

		if err := connectionService.DeleteImportConnection(r.Context(), connID); err != nil {
			writeImportConnectionServiceError(w, r, logger, "delete import connection", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

// --- Error handler ---

func writeImportConnectionServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, op string, err error) {
	switch {
	case errors.Is(err, app.ErrImportConnectionNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "import connection not found")
	case errors.Is(err, app.ErrImportConnectionDuplicate):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "a connection with this name already exists for this source")
	case errors.Is(err, app.ErrProviderUnauthorized):
		writeAPIError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider rejected the API key — check the key and try again")
	case errors.Is(err, app.ErrSecretKeyMissing):
		writeAPIError(w, http.StatusServiceUnavailable, "CONFIG_REQUIRED", "REKENRAAM_SECRET_KEY is not configured on the server")
	default:
		var ve app.ValidationError
		if errors.As(err, &ve) {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", ve.Message)
			return
		}
		writeServiceInternalError(w, r, logger, op, err)
	}
}

// --- ID parsing ---

func readImportConnectionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("connection_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "connection_id is invalid")
		return 0, false
	}
	return id, true
}

// --- Mapping helpers ---

func toImportConnectionResponse(c app.ImportConnection) importConnectionResponse {
	config := c.ConfigJSON
	if config == "" {
		config = "{}"
	}
	return importConnectionResponse{
		ID:              c.ID,
		BookID:          c.BookID,
		SourceKind:      c.SourceKind,
		DisplayName:     c.DisplayName,
		KeyHint:         c.KeyHint,
		ConfigJSON:      config,
		LastFetchStatus: c.LastFetchStatus,
		LastFetchedAt:   c.LastFetchedAt,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}
