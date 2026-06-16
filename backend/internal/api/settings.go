package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type userPreferencesResponse struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	TimeZone  string `json:"time_zone"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type userPreferencesRequest struct {
	TimeZone     string `json:"time_zone"`
	ChangeReason string `json:"change_reason"`
}

func userPreferences(logger *slog.Logger, authService *app.AuthService, settingsService *app.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}

		preferences, err := settingsService.Preferences(r.Context(), owner.ID)
		if err != nil {
			logger.ErrorContext(r.Context(), "read user preferences", slog.Any("err", err))
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, toUserPreferencesResponse(preferences))
	}
}

func saveUserPreferences(logger *slog.Logger, authService *app.AuthService, settingsService *app.SettingsService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request userPreferencesRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		preferences, err := settingsService.SavePreferences(r.Context(), app.SaveUserPreferencesInput{
			UserID:        owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			TimeZone:      request.TimeZone,
			ChangeReason:  request.ChangeReason,
		})
		if err != nil {
			var validationError app.ValidationError
			switch {
			case errors.As(err, &validationError):
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
			default:
				logger.ErrorContext(r.Context(), "save user preferences", slog.Any("err", err))
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, toUserPreferencesResponse(preferences))
	}))
}

func toUserPreferencesResponse(preferences app.UserPreferences) userPreferencesResponse {
	return userPreferencesResponse{
		ID:        preferences.ID,
		UserID:    preferences.UserID,
		TimeZone:  preferences.TimeZone,
		CreatedAt: preferences.CreatedAt,
		UpdatedAt: preferences.UpdatedAt,
	}
}
