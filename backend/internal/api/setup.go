package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"rekenraam/backend/internal/app"
)

const (
	sessionCookieName       = "rekenraam_session"
	secureSessionCookieName = "__Host-rekenraam_session"
)

type setupStatusResponse struct {
	Completed        bool                `json:"completed"`
	CurrentStep      string              `json:"current_step,omitempty"`
	InstallState     string              `json:"install_state"`
	ImplementedSteps []string            `json:"implemented_steps"`
	BlockingStep     string              `json:"blocking_step,omitempty"`
	Steps            []setupStepResponse `json:"steps"`
}

type setupStepResponse struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

type createOwnerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TimeZone string `json:"time_zone"`
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

func createOwner(logger *slog.Logger, setupService *app.SetupService, options HandlerOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createOwnerRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		result, err := setupService.CreateOwner(r.Context(), app.CreateOwnerInput{
			Username: request.Username,
			Password: request.Password,
			TimeZone: request.TimeZone,
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

		writeSessionCookie(w, r, options, result.SessionToken)
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
		Completed:        status.Completed,
		CurrentStep:      status.CurrentStep,
		InstallState:     status.InstallState,
		ImplementedSteps: append([]string(nil), status.ImplementedSteps...),
		BlockingStep:     status.BlockingStep,
		Steps:            steps,
	}
}

func writeSessionCookie(w http.ResponseWriter, r *http.Request, options HandlerOptions, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     requestSessionCookieName(r, options),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   requestUsesHTTPS(r, options),
		MaxAge:   int(app.SessionLifetime.Seconds()),
	})
}

func requestSessionCookieName(r *http.Request, options HandlerOptions) string {
	if requestUsesHTTPS(r, options) {
		return secureSessionCookieName
	}

	return sessionCookieName
}

// errRequestTooLarge is returned by the decode helpers when the body exceeds 1 MB.
var errRequestTooLarge = errors.New("request body too large")

func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, errRequestTooLarge) {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body must not exceed 1 MB")
		return
	}
	var validationError app.ValidationError
	if errors.As(err, &validationError) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
		return
	}
	writeAPIError(w, http.StatusBadRequest, "DECODE_ERROR", err.Error())
}

func decodeJSONBody(r *http.Request, destination any) error {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return app.ValidationError{Message: "content type must be application/json"}
	}

	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return app.ValidationError{Message: "invalid request body"}
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return app.ValidationError{Message: "request body must contain a single JSON object"}
	}

	return nil
}

func decodeOptionalJSONBody(r *http.Request, destination any) error {
	if r.Body == nil {
		return nil
	}

	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return app.ValidationError{Message: "invalid request body"}
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return app.ValidationError{Message: "content type must be application/json"}
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return app.ValidationError{Message: "invalid request body"}
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return app.ValidationError{Message: "request body must contain a single JSON object"}
	}

	return nil
}
