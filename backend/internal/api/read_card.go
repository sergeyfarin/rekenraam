package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type readCardResponse[T any] struct {
	Status  string `json:"status"`
	Summary *T     `json:"summary"`
}

func toReadCard[T any](r *http.Request, logger *slog.Logger, source string, result app.ReadCard[T]) readCardResponse[T] {
	if result.Err != nil || result.Summary == nil {
		// Dependency errors may contain financial data. Record the source and type.
		logger.ErrorContext(r.Context(), "read summary", slog.String("source", source), slog.String("error_type", fmt.Sprintf("%T", result.Err)))
		return readCardResponse[T]{Status: "failed"}
	}
	return readCardResponse[T]{Status: "ok", Summary: result.Summary}
}

func mapReadCard[T, R any](r *http.Request, logger *slog.Logger, source string, result app.ReadCard[T], convert func(T) R) readCardResponse[R] {
	mapped := app.ReadCard[R]{Err: result.Err}
	if result.Err == nil && result.Summary != nil {
		summary := convert(*result.Summary)
		mapped.Summary = &summary
	}
	return toReadCard(r, logger, source, mapped)
}
