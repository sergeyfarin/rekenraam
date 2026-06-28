package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
)

type tagResponse struct {
	ID        int64           `json:"id"`
	BookID    int64           `json:"book_id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Color     string          `json:"color,omitempty"`
	Icon      string          `json:"icon,omitempty"`
	Status    string          `json:"status"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type tagsResponse struct {
	Tags []tagResponse `json:"tags"`
}

type tagRequest struct {
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Color    string          `json:"color"`
	Icon     string          `json:"icon"`
	Metadata json.RawMessage `json:"metadata"`
}

func listTags(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		includeArchived, err := parseOptionalBool(r.URL.Query().Get("include_archived"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "include_archived is invalid")
			return
		}

		tags, err := tagService.ListTags(r.Context(), app.ListTagsInput{
			Status:          r.URL.Query().Get("status"),
			Kind:            r.URL.Query().Get("kind"),
			IncludeArchived: includeArchived,
			Query:           r.URL.Query().Get("q"),
		})
		if err != nil {
			writeTagServiceError(w, r, logger, "list tags", err)
			return
		}

		writeJSON(w, http.StatusOK, tagsResponse{Tags: toTagResponses(tags)})
	}
}

func readTag(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		tagID, ok := readTagID(w, r)
		if !ok {
			return
		}

		tag, err := tagService.Tag(r.Context(), tagID)
		if err != nil {
			writeTagServiceError(w, r, logger, "read tag", err)
			return
		}

		writeJSON(w, http.StatusOK, toTagResponse(tag))
	}
}

func createTag(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		var request tagRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		tag, err := tagService.CreateTag(r.Context(), app.CreateTagInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			Operation:     "tag.create",
			Name:          request.Name,
			Kind:          request.Kind,
			Color:         request.Color,
			Icon:          request.Icon,
			MetadataJSON:  rawJSONText(request.Metadata),
		})
		if err != nil {
			writeTagServiceError(w, r, logger, "create tag", err)
			return
		}

		writeJSON(w, http.StatusCreated, toTagResponse(tag))
	}))
}

func updateTag(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		tagID, ok := readTagID(w, r)
		if !ok {
			return
		}

		var request tagRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}

		tag, err := tagService.UpdateTag(r.Context(), app.UpdateTagInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			Operation:     "tag.update",
			TagID:         tagID,
			Name:          request.Name,
			Kind:          request.Kind,
			Color:         request.Color,
			Icon:          request.Icon,
			MetadataJSON:  rawJSONText(request.Metadata),
		})
		if err != nil {
			writeTagServiceError(w, r, logger, "update tag", err)
			return
		}

		writeJSON(w, http.StatusOK, toTagResponse(tag))
	}))
}

func archiveTag(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService, options HandlerOptions) http.HandlerFunc {
	return tagLifecycleMutation(logger, authService, tagService, options, "archive tag", func(owner app.Owner, tagID int64, r *http.Request) (app.Tag, error) {
		return tagService.ArchiveTag(r.Context(), app.TagLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TagID:         tagID,
		})
	})
}

func restoreTag(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService, options HandlerOptions) http.HandlerFunc {
	return tagLifecycleMutation(logger, authService, tagService, options, "restore tag", func(owner app.Owner, tagID int64, r *http.Request) (app.Tag, error) {
		return tagService.RestoreTag(r.Context(), app.TagLifecycleInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			OriginType:    "browser_api",
			TagID:         tagID,
		})
	})
}

func tagLifecycleMutation(logger *slog.Logger, authService *app.AuthService, tagService *app.TagService, options HandlerOptions, action string, mutate func(app.Owner, int64, *http.Request) (app.Tag, error)) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}

		tagID, ok := readTagID(w, r)
		if !ok {
			return
		}

		tag, err := mutate(owner, tagID, r)
		if err != nil {
			writeTagServiceError(w, r, logger, action, err)
			return
		}

		writeJSON(w, http.StatusOK, toTagResponse(tag))
	}))
}

func readTagID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	tagID, err := strconv.ParseInt(r.PathValue("tag_id"), 10, 64)
	if err != nil || tagID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", "tag id is invalid")
		return 0, false
	}

	return tagID, true
}

func writeTagServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.Is(err, app.ErrTagNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "tag not found")
	case errors.Is(err, app.ErrTagExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "tag already exists")
	case errors.Is(err, app.ErrTagArchived):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "archived tag cannot be updated")
	default:
		writeServiceInternalError(w, r, logger, action, err)
	}
}

func toTagResponse(tag app.Tag) tagResponse {
	return tagResponse{
		ID:        tag.ID,
		BookID:    tag.BookID,
		Name:      tag.Name,
		Kind:      tag.Kind,
		Color:     tag.Color,
		Icon:      tag.Icon,
		Status:    tag.Status,
		Metadata:  json.RawMessage(tag.MetadataJSON),
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	}
}

func toTagResponses(tags []app.Tag) []tagResponse {
	responses := make([]tagResponse, 0, len(tags))
	for _, tag := range tags {
		responses = append(responses, toTagResponse(tag))
	}

	return responses
}
