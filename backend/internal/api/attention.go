package api

import (
	"log/slog"
	"net/http"
	"time"

	"rekenraam/backend/internal/app"
)

type attentionSourceResponse struct {
	DividendReviewCount *int   `json:"dividend_review_count,omitempty"`
	ID                  string `json:"id"`
	Unit                string `json:"unit"`
	State               string `json:"state"`
	Status              string `json:"status"`
	Count               *int   `json:"count"`
	Failed              bool   `json:"failed"`
	ErrorCode           string `json:"error_code,omitempty"`
}
type attentionResponse struct {
	CheckedAt   string                    `json:"checked_at"`
	TotalKnown  int                       `json:"total_known"`
	Complete    bool                      `json:"complete"`
	HasFailures bool                      `json:"has_failures"`
	Sources     []attentionSourceResponse `json:"sources"`
}

func attentionRead(logger *slog.Logger, auth *app.AuthService, service *app.AttentionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, auth); !ok {
			return
		}
		result := service.Read(r.Context())
		response := attentionResponse{CheckedAt: result.CheckedAt.Format(time.RFC3339), TotalKnown: result.TotalKnown, Complete: result.Complete, HasFailures: result.HasFailures, Sources: []attentionSourceResponse{}}
		for _, source := range result.Sources {
			item := attentionSourceResponse{DividendReviewCount: source.DividendReviewCount, ID: source.ID, Unit: source.Unit, State: source.State, Status: source.Status, Count: source.Count, Failed: source.Failed}
			if source.Err != nil {
				item.ErrorCode = "INTERNAL_ERROR"
				logger.ErrorContext(r.Context(), "read attention source", slog.String("source", source.ID), slog.Any("err", source.Err))
			}
			response.Sources = append(response.Sources, item)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, response)
	}
}
