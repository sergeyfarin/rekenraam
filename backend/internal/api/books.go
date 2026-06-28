package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type bookResponse struct {
	ID                         int64  `json:"id"`
	OwnerUserID                int64  `json:"owner_user_id"`
	Code                       string `json:"code"`
	Name                       string `json:"name"`
	DefaultCurrencyCommodityID *int64 `json:"default_currency_commodity_id,omitempty"`
	CreatedAt                  string `json:"created_at"`
	UpdatedAt                  string `json:"updated_at"`
}

type createBookRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type createBookResponse struct {
	Book  bookResponse        `json:"book"`
	Setup setupStatusResponse `json:"setup"`
}

func currentBook(logger *slog.Logger, authService *app.AuthService, bookService *app.BookService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		book, err := bookService.CurrentBook(r.Context())
		if err != nil {
			writeBookServiceError(w, r, logger, "read current book", err)
			return
		}

		writeJSON(w, http.StatusOK, toBookResponse(book))
	}
}

func createBook(logger *slog.Logger, authService *app.AuthService, bookService *app.BookService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request createBookRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		result, err := bookService.CreateBook(r.Context(), app.CreateBookInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "setup",
			Operation:     "book.setup",
			Code:          request.Code,
			Name:          request.Name,
		})
		if err != nil {
			writeBookServiceError(w, r, logger, "create setup book", err)
			return
		}

		writeJSON(w, http.StatusCreated, createBookResponse{
			Book:  toBookResponse(result.Book),
			Setup: toSetupStatusResponse(result.SetupStatus),
		})
	}))
}

func authenticatedMutationOwner(w http.ResponseWriter, r *http.Request) (app.Owner, bool) {
	owner, ok := r.Context().Value(authenticatedOwnerKey{}).(app.Owner)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return app.Owner{}, false
	}

	return owner, true
}

func authenticatedSessionID(r *http.Request) int64 {
	sessionID, _ := r.Context().Value(authenticatedSessionIDKey{}).(int64)
	return sessionID
}

func authenticatedOwner(w http.ResponseWriter, r *http.Request, logger *slog.Logger, authService *app.AuthService) (app.Owner, bool) {
	if authService == nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return app.Owner{}, false
	}

	status, err := authService.Session(r.Context(), readSessionToken(r))
	if err != nil {
		writeAuthServiceError(w, r, logger, "read authenticated owner", err)
		return app.Owner{}, false
	}
	if !status.Authenticated || status.User == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication is required")
		return app.Owner{}, false
	}

	return *status.User, true
}

func writeBookServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrBookNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "book not found")
	case errors.Is(err, app.ErrBookAlreadyExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "book already exists")
	default:
		writeServiceInternalError(w, r, logger, action, err)
	}
}

func toBookResponse(book app.Book) bookResponse {
	return bookResponse{
		ID:                         book.ID,
		OwnerUserID:                book.OwnerUserID,
		Code:                       book.Code,
		Name:                       book.Name,
		DefaultCurrencyCommodityID: book.DefaultCurrencyCommodityID,
		CreatedAt:                  book.CreatedAt,
		UpdatedAt:                  book.UpdatedAt,
	}
}
