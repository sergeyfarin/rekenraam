package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type categoryResponse struct {
	ID               int64           `json:"id"`
	BookID           int64           `json:"book_id"`
	Status           string          `json:"status"`
	Code             string          `json:"code,omitempty"`
	Name             string          `json:"name,omitempty"`
	CategoryType     string          `json:"category_type"`
	ParentCategoryID *int64          `json:"parent_category_id,omitempty"`
	AllowsPostings   bool            `json:"allows_postings"`
	Icon             string          `json:"icon,omitempty"`
	IsBuiltin        bool            `json:"is_builtin"`
	IsStarter        bool            `json:"is_starter"`
	BuiltinKey       string          `json:"builtin_key,omitempty"`
	Metadata         json.RawMessage `json:"metadata"`
	OpenedOn         string          `json:"opened_on"`
	ClosedOn         string          `json:"closed_on,omitempty"`
	EffectiveFrom    string          `json:"effective_from"`
	ChangeReason     string          `json:"change_reason"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

type categoriesResponse struct {
	Categories []categoryResponse `json:"categories"`
}

type completeCategoriesSetupResponse struct {
	Categories []categoryResponse  `json:"categories"`
	Setup      setupStatusResponse `json:"setup"`
}

type createCategoryRequest struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	CategoryType     string `json:"category_type"`
	ParentCategoryID *int64 `json:"parent_category_id"`
	AllowsPostings   *bool  `json:"allows_postings"`
	Icon             string `json:"icon"`
	OpenedOn         string `json:"opened_on"`
	EffectiveFrom    string `json:"effective_from"`
	ChangeReason     string `json:"change_reason"`
}

type updateCategoryRequest struct {
	Code             *string `json:"code"`
	Name             *string `json:"name"`
	CategoryType     *string `json:"category_type"`
	ParentCategoryID *int64  `json:"parent_category_id"`
	ClearParent      bool    `json:"clear_parent"`
	AllowsPostings   *bool   `json:"allows_postings"`
	Icon             *string `json:"icon"`
	OpenedOn         string  `json:"opened_on"`
	EffectiveFrom    string  `json:"effective_from"`
	ChangeReason     string  `json:"change_reason"`
}

type categoryLifecycleRequest struct {
	EffectiveFrom string `json:"effective_from"`
	ChangeReason  string `json:"change_reason"`
}

func listCategories(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		includeArchived, err := parseOptionalBool(r.URL.Query().Get("include_archived"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_archived is invalid")
			return
		}

		categories, err := categoryService.ListCategories(r.Context(), app.ListCategoriesInput{
			Status:          r.URL.Query().Get("status"),
			CategoryType:    r.URL.Query().Get("category_type"),
			IncludeArchived: includeArchived,
			Query:           r.URL.Query().Get("q"),
		})
		if err != nil {
			writeCategoryServiceError(w, r, logger, "list categories", err)
			return
		}

		writeJSON(w, http.StatusOK, categoriesResponse{Categories: toCategoryResponses(categories)})
	}
}

func readCategory(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		categoryID, ok := readCategoryID(w, r)
		if !ok {
			return
		}

		category, err := categoryService.Category(r.Context(), categoryID)
		if err != nil {
			writeCategoryServiceError(w, r, logger, "read category", err)
			return
		}

		writeJSON(w, http.StatusOK, toCategoryResponse(category))
	}
}

func createCategory(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request createCategoryRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		category, err := categoryService.CreateCategory(r.Context(), app.CreateCategoryInput{
			OwnerUserID:      owner.ID,
			AuthSessionID:    authenticatedSessionID(r),
			RequestID:        RequestIDFromContext(r.Context()),
			OriginType:       "browser_api",
			Operation:        "category.create",
			Code:             request.Code,
			Name:             request.Name,
			CategoryType:     request.CategoryType,
			ParentCategoryID: request.ParentCategoryID,
			AllowsPostings:   request.AllowsPostings,
			Icon:             request.Icon,
			OpenedOn:         request.OpenedOn,
			EffectiveFrom:    request.EffectiveFrom,
			ChangeReason:     request.ChangeReason,
		})
		if err != nil {
			writeCategoryServiceError(w, r, logger, "create category", err)
			return
		}

		writeJSON(w, http.StatusCreated, toCategoryResponse(category))
	}))
}

func updateCategory(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		categoryID, ok := readCategoryID(w, r)
		if !ok {
			return
		}

		var request updateCategoryRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		category, err := categoryService.UpdateCategory(r.Context(), app.UpdateCategoryInput{
			OwnerUserID:      owner.ID,
			AuthSessionID:    authenticatedSessionID(r),
			RequestID:        RequestIDFromContext(r.Context()),
			OriginType:       "browser_api",
			Operation:        "category.update",
			CategoryID:       categoryID,
			Code:             request.Code,
			Name:             request.Name,
			CategoryType:     request.CategoryType,
			ParentCategoryID: request.ParentCategoryID,
			ClearParent:      request.ClearParent,
			AllowsPostings:   request.AllowsPostings,
			Icon:             request.Icon,
			OpenedOn:         request.OpenedOn,
			EffectiveFrom:    request.EffectiveFrom,
			ChangeReason:     request.ChangeReason,
		})
		if err != nil {
			writeCategoryServiceError(w, r, logger, "update category", err)
			return
		}

		writeJSON(w, http.StatusOK, toCategoryResponse(category))
	}))
}

func disableCategory(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions) http.HandlerFunc {
	return categoryLifecycleMutation(logger, authService, categoryService, options, "disable category", func(owner app.Owner, categoryID int64, r *http.Request, request categoryLifecycleRequest) (app.Category, error) {
		return categoryService.DisableCategory(r.Context(), app.CategoryLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			CategoryID:    categoryID,
			EffectiveFrom: request.EffectiveFrom,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func restoreCategory(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions) http.HandlerFunc {
	return categoryLifecycleMutation(logger, authService, categoryService, options, "restore category", func(owner app.Owner, categoryID int64, r *http.Request, request categoryLifecycleRequest) (app.Category, error) {
		return categoryService.RestoreCategory(r.Context(), app.CategoryLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			CategoryID:    categoryID,
			EffectiveFrom: request.EffectiveFrom,
			ChangeReason:  request.ChangeReason,
		})
	})
}

func categoryLifecycleMutation(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions, action string, mutate func(app.Owner, int64, *http.Request, categoryLifecycleRequest) (app.Category, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		categoryID, ok := readCategoryID(w, r)
		if !ok {
			return
		}

		var request categoryLifecycleRequest
		if err := decodeOptionalJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		category, err := mutate(owner, categoryID, r, request)
		if err != nil {
			writeCategoryServiceError(w, r, logger, action, err)
			return
		}

		writeJSON(w, http.StatusOK, toCategoryResponse(category))
	}))
}

func deleteCategory(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		categoryID, ok := readCategoryID(w, r)
		if !ok {
			return
		}

		err := categoryService.DeleteCategory(r.Context(), app.CategoryLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			CategoryID:    categoryID,
		})
		if err != nil {
			writeCategoryServiceError(w, r, logger, "delete category", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}))
}

func completeCategoriesSetup(logger *slog.Logger, authService *app.AuthService, categoryService *app.CategoryService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		result, err := categoryService.EnsureCategories(r.Context(), app.EnsureCategoriesInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "setup",
			Operation:     "categories.setup",
		})
		if err != nil {
			writeCategoryServiceError(w, r, logger, "complete categories setup", err)
			return
		}

		writeJSON(w, http.StatusCreated, completeCategoriesSetupResponse{
			Categories: toCategoryResponses(result.Categories),
			Setup:      toSetupStatusResponse(result.SetupStatus),
		})
	}))
}

func readCategoryID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	categoryID, err := strconv.ParseInt(r.PathValue("category_id"), 10, 64)
	if err != nil || categoryID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "category id is invalid")
		return 0, false
	}

	return categoryID, true
}

func writeCategoryServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrCategoryNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "category not found")
	case errors.Is(err, app.ErrCategoryExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "category already exists")
	case errors.Is(err, app.ErrCategoryParentInvalid):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "category parent is invalid")
	case errors.Is(err, app.ErrCategoryHasChildren):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "category has active child categories")
	case errors.Is(err, app.ErrCategoryBuiltInProtected):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "built-in categories can only be disabled")
	case errors.Is(err, app.ErrCategoryUsed):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "category is used by financial records")
	case errors.Is(err, app.ErrSetupAlreadyComplete):
		writeAPIError(w, http.StatusConflict, "SETUP_ALREADY_COMPLETE", "categories setup is already complete")
	default:
		writeServiceInternalError(w, r, logger, action, err)
	}
}

func toCategoryResponse(category app.Category) categoryResponse {
	return categoryResponse{
		ID:               category.ID,
		BookID:           category.BookID,
		Status:           category.Status,
		Code:             category.Code,
		Name:             category.Name,
		CategoryType:     category.CategoryType,
		ParentCategoryID: category.ParentCategoryID,
		AllowsPostings:   category.AllowsPostings,
		Icon:             category.Icon,
		IsBuiltin:        category.IsBuiltin,
		IsStarter:        category.IsStarter,
		BuiltinKey:       category.BuiltinKey,
		Metadata:         json.RawMessage(category.MetadataJSON),
		OpenedOn:         category.OpenedOn,
		ClosedOn:         category.ClosedOn,
		EffectiveFrom:    category.EffectiveFrom,
		ChangeReason:     category.ChangeReason,
		CreatedAt:        category.CreatedAt,
		UpdatedAt:        category.UpdatedAt,
	}
}

func toCategoryResponses(categories []app.Category) []categoryResponse {
	responses := make([]categoryResponse, 0, len(categories))
	for _, category := range categories {
		responses = append(responses, toCategoryResponse(category))
	}

	return responses
}
