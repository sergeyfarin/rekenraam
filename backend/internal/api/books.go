package api

import (
	"errors"
	"log/slog"
	"net/http"

	"rekenraam/backend/internal/app"
)

type bookResponse struct {
	ID                      int64  `json:"id"`
	OwnerUserID             int64  `json:"owner_user_id"`
	Code                    string `json:"code"`
	Name                    string `json:"name"`
	BaseCurrencyCommodityID *int64 `json:"base_currency_commodity_id,omitempty"`
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
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
			switch {
			case errors.Is(err, app.ErrBookNotFound):
				writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "book not found")
			default:
				logger.ErrorContext(r.Context(), "read current book", slog.Any("err", err))
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, toBookResponse(book))
	}
}

func createBook(logger *slog.Logger, authService *app.AuthService, bookService *app.BookService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}

		var request createBookRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		result, err := bookService.CreateBook(r.Context(), app.CreateBookInput{
			OwnerUserID: owner.ID,
			Code:        request.Code,
			Name:        request.Name,
		})
		if err != nil {
			var validationError app.ValidationError
			switch {
			case errors.As(err, &validationError):
				writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
			case errors.Is(err, app.ErrBookAlreadyExists):
				writeAPIError(w, http.StatusConflict, "CONFLICT", "book already exists")
			case errors.Is(err, app.ErrUnauthenticated):
				writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication is required")
			default:
				logger.ErrorContext(r.Context(), "create setup book", slog.Any("err", err))
				writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusCreated, createBookResponse{
			Book:  toBookResponse(result.Book),
			Setup: toSetupStatusResponse(result.SetupStatus),
		})
	}))
}

func authenticatedOwner(w http.ResponseWriter, r *http.Request, logger *slog.Logger, authService *app.AuthService) (app.Owner, bool) {
	if authService == nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return app.Owner{}, false
	}

	status, err := authService.Session(r.Context(), readSessionToken(r))
	if err != nil {
		logger.ErrorContext(r.Context(), "read authenticated owner", slog.Any("err", err))
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return app.Owner{}, false
	}
	if !status.Authenticated || status.User == nil {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication is required")
		return app.Owner{}, false
	}

	return *status.User, true
}

func toBookResponse(book app.Book) bookResponse {
	return bookResponse{
		ID:                      book.ID,
		OwnerUserID:             book.OwnerUserID,
		Code:                    book.Code,
		Name:                    book.Name,
		BaseCurrencyCommodityID: book.BaseCurrencyCommodityID,
		CreatedAt:               book.CreatedAt,
		UpdatedAt:               book.UpdatedAt,
	}
}
