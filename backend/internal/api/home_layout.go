package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

func readHomeLayout(logger *slog.Logger, auth *app.AuthService, home *app.HomeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, auth)
		if !ok {
			return
		}
		layout, err := home.Layout(r.Context(), owner.ID)
		writeHomeLayout(w, r, logger, layout, err)
	}
}

func saveHomeLayout(logger *slog.Logger, auth *app.AuthService, home *app.HomeService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, auth, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		type layoutDocument struct {
			app.HomeLayout
			ShowReview  *bool `json:"show_review"`
			ExploreOpen *bool `json:"explore_open"`
		}
		// Start with an empty document so that an omitted layout fails validation
		// while an explicit JSON null clears the pointer and requests a reset.
		request := struct {
			ExpectedRevision *int64          `json:"expected_revision"`
			Layout           *layoutDocument `json:"layout"`
		}{Layout: &layoutDocument{}}
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		if request.ExpectedRevision == nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "an expected home layout revision is required")
			return
		}
		var layout *app.HomeLayout
		if document := request.Layout; document != nil {
			if document.ShowReview == nil || document.ExploreOpen == nil {
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid home layout document")
				return
			}
			document.HomeLayout.ShowReview = *document.ShowReview
			document.HomeLayout.ExploreOpen = *document.ExploreOpen
			layout = &document.HomeLayout
		}
		result, err := home.SaveLayout(r.Context(), owner.ID, *request.ExpectedRevision, layout)
		writeHomeLayout(w, r, logger, result, err)
	}))
}

func writeHomeLayout(w http.ResponseWriter, r *http.Request, logger *slog.Logger, result app.HomeLayoutState, err error) {
	if errors.Is(err, app.ErrHomeLayoutConflict) {
		writeAPIError(w, http.StatusConflict, "CONFLICT", "home layout changed in another tab")
		return
	}
	if err != nil {
		writeLedgerServiceError(w, r, logger, "home layout", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}
