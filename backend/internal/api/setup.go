package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

const sessionCookieName = "rekenraam_session"

type setupStatusResponse struct {
	Completed   bool                `json:"completed"`
	CurrentStep string              `json:"current_step,omitempty"`
	Steps       []setupStepResponse `json:"steps"`
}

type setupStepResponse struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

type createOwnerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createOwnerResponse struct {
	Owner OwnerResponse       `json:"owner"`
	Setup setupStatusResponse `json:"setup"`
}

type OwnerResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func setupStatus(logger *slog.Logger, setupService *app.SetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := setupService.Status(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "read setup status", slog.Any("err", err))
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, toSetupStatusResponse(status))
	}
}

func createOwner(logger *slog.Logger, setupService *app.SetupService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createOwnerRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		result, err := setupService.CreateOwner(r.Context(), app.CreateOwnerInput{
			Username: request.Username,
			Password: request.Password,
		})
		if err != nil {
			var validationError app.ValidationError
			switch {
			case errors.As(err, &validationError):
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
			case errors.Is(err, app.ErrSetupAlreadyComplete):
				writeAPIError(w, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "setup owner is already complete")
			default:
				logger.ErrorContext(r.Context(), "create setup owner", slog.Any("err", err))
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
			return
		}

		writeSessionCookie(w, r, result.SessionToken)
		writeJSON(w, http.StatusCreated, createOwnerResponse{
			Owner: OwnerResponse{
				ID:       result.Owner.ID,
				Username: result.Owner.Username,
			},
			Setup: toSetupStatusResponse(result.SetupStatus),
		})
	}
}

func toSetupStatusResponse(status app.SetupStatus) setupStatusResponse {
	steps := make([]setupStepResponse, 0, len(status.Steps))
	for _, step := range status.Steps {
		steps = append(steps, setupStepResponse{
			Key:    step.Key,
			Status: step.Status,
		})
	}

	return setupStatusResponse{
		Completed:   status.Completed,
		CurrentStep: status.CurrentStep,
		Steps:       steps,
	}
}

func writeSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})
}

func decodeJSONBody(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return app.ValidationError{Message: "invalid request body"}
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return app.ValidationError{Message: "request body must contain a single JSON object"}
	}

	return nil
}
