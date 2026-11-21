package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type searchCatalogItemResponse struct {
	Kind         string  `json:"kind"`
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Code         string  `json:"code"`
	BuiltinKey   string  `json:"builtin_key"`
	StarterKey   *string `json:"starter_key"`
	CurrencyCode *string `json:"currency_code"`
	Status       string  `json:"status"`
	AccountKind  string  `json:"account_kind"`
}
type searchCatalogResponse struct {
	Items      []searchCatalogItemResponse `json:"items"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

func searchCatalog(logger *slog.Logger, auth *app.AuthService, service *app.SearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		limit := 500
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "limit is invalid")
				return
			}
			limit = parsed
		}
		result, err := service.Catalog(r.Context(), r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeAccountServiceError(w, r, logger, "read search catalog", err)
			return
		}
		items := make([]searchCatalogItemResponse, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, searchCatalogItemResponse{Kind: item.Kind, ID: item.ID, Name: item.Name, Code: item.Code, BuiltinKey: item.BuiltinKey,
				StarterKey: optionalNullString(item.StarterKey), CurrencyCode: optionalNullString(item.CurrencyCode), Status: item.Status, AccountKind: item.AccountKind})
		}
		writeJSON(w, http.StatusOK, searchCatalogResponse{Items: items, NextCursor: result.NextCursor})
	}
}
