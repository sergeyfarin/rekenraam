package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

const (
	tagNameMaxBytes     = 120
	tagIconMaxBytes     = 64
	tagJSONMaxBytes     = 10000
	tagListLimit        = 2000
	defaultTagKind      = "custom"
	tagLifecycleArchive = "archived tag"
	tagLifecycleRestore = "restored tag"
)

var (
	ErrTagNotFound  = errors.New("tag not found")
	ErrTagExists    = errors.New("tag already exists")
	tagColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	tagIconPattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

var tagKinds = map[string]bool{
	"project": true,
	"person":  true,
	"flag":    true,
	"place":   true,
	"custom":  true,
}

type Tag struct {
	ID           int64
	BookID       int64
	Name         string
	Kind         string
	Color        string
	Icon         string
	Status       string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
}

type ListTagsInput struct {
	Status          string
	Kind            string
	IncludeArchived bool
	Query           string
}

type CreateTagInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	Name          string
	Kind          string
	Color         string
	Icon          string
	MetadataJSON  string
}

type UpdateTagInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
	TagID         int64
	Name          string
	Kind          string
	Color         string
	Icon          string
	MetadataJSON  string
}

type TagLifecycleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	TagID         int64
}

type TagService struct {
	repository *db.TagRepository
	now        func() time.Time
}

func NewTagService(repository *db.TagRepository) *TagService {
	return &TagService{
		repository: repository,
		now:        time.Now,
	}
}

func (s *TagService) ListTags(ctx context.Context, input ListTagsInput) ([]Tag, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && status != "active" && status != "archived" {
		return nil, ValidationError{Message: "tag status is invalid"}
	}

	kind := strings.TrimSpace(input.Kind)
	if kind != "" && !tagKinds[kind] {
		return nil, ValidationError{Message: "tag kind is invalid"}
	}

	records, err := s.repository.ListTags(ctx, db.ListTagsParams{
		BookID:          BookID,
		Status:          status,
		Kind:            kind,
		IncludeArchived: input.IncludeArchived,
		Query:           strings.TrimSpace(input.Query),
		Limit:           tagListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	return toTags(records), nil
}

func (s *TagService) Tag(ctx context.Context, tagID int64) (Tag, error) {
	if tagID <= 0 {
		return Tag{}, ValidationError{Message: "tag id is required"}
	}

	record, err := s.repository.TagByID(ctx, BookID, tagID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Tag{}, ErrTagNotFound
		}
		return Tag{}, fmt.Errorf("read tag: %w", err)
	}

	return toTag(record), nil
}

func (s *TagService) CreateTag(ctx context.Context, input CreateTagInput) (Tag, error) {
	if input.OwnerUserID <= 0 {
		return Tag{}, ValidationError{Message: "owner user is required"}
	}

	spec, err := tagSpec(tagSpecInput{
		Name:         input.Name,
		Kind:         input.Kind,
		Color:        input.Color,
		Icon:         input.Icon,
		MetadataJSON: input.MetadataJSON,
	})
	if err != nil {
		return Tag{}, err
	}

	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.CreateTag(ctx, db.CreateTagParams{
		BookID:        BookID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     input.Operation,
		Spec:          spec,
		CreatedAt:     now,
	})
	if err != nil {
		return Tag{}, tagRepositoryError("persist tag", err)
	}

	return toTag(record), nil
}

func (s *TagService) UpdateTag(ctx context.Context, input UpdateTagInput) (Tag, error) {
	if input.OwnerUserID <= 0 {
		return Tag{}, ValidationError{Message: "owner user is required"}
	}
	if input.TagID <= 0 {
		return Tag{}, ValidationError{Message: "tag id is required"}
	}

	spec, err := tagSpec(tagSpecInput{
		Name:         input.Name,
		Kind:         input.Kind,
		Color:        input.Color,
		Icon:         input.Icon,
		MetadataJSON: input.MetadataJSON,
	})
	if err != nil {
		return Tag{}, err
	}

	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.UpdateTag(ctx, db.UpdateTagParams{
		BookID:        BookID,
		TagID:         input.TagID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     input.Operation,
		Spec:          spec,
		UpdatedAt:     now,
	})
	if err != nil {
		return Tag{}, tagRepositoryError("persist tag update", err)
	}

	return toTag(record), nil
}

func (s *TagService) ArchiveTag(ctx context.Context, input TagLifecycleInput) (Tag, error) {
	return s.changeTagStatus(ctx, input, "archived", tagLifecycleArchive, "tag.archive")
}

func (s *TagService) RestoreTag(ctx context.Context, input TagLifecycleInput) (Tag, error) {
	return s.changeTagStatus(ctx, input, "active", tagLifecycleRestore, "tag.restore")
}

func (s *TagService) changeTagStatus(ctx context.Context, input TagLifecycleInput, status string, reason string, operation string) (Tag, error) {
	if input.OwnerUserID <= 0 {
		return Tag{}, ValidationError{Message: "owner user is required"}
	}
	if input.TagID <= 0 {
		return Tag{}, ValidationError{Message: "tag id is required"}
	}

	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.ChangeTagStatus(ctx, db.TagLifecycleParams{
		BookID:        BookID,
		TagID:         input.TagID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     operation,
		Status:        status,
		Reason:        reason,
		UpdatedAt:     now,
	})
	if err != nil {
		return Tag{}, tagRepositoryError("persist tag status", err)
	}

	return toTag(record), nil
}

type tagSpecInput struct {
	Name         string
	Kind         string
	Color        string
	Icon         string
	MetadataJSON string
}

func tagSpec(input tagSpecInput) (db.TagSpec, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return db.TagSpec{}, ValidationError{Message: "tag name is required"}
	}
	if len(name) > tagNameMaxBytes {
		return db.TagSpec{}, ValidationError{Message: fmt.Sprintf("tag name must be at most %d bytes", tagNameMaxBytes)}
	}
	if hasControlCharacter(name) {
		return db.TagSpec{}, ValidationError{Message: "tag name must not contain control characters"}
	}

	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = defaultTagKind
	}
	if !tagKinds[kind] {
		return db.TagSpec{}, ValidationError{Message: "tag kind is invalid"}
	}

	color := strings.TrimSpace(input.Color)
	if color != "" && !tagColorPattern.MatchString(color) {
		return db.TagSpec{}, ValidationError{Message: "tag color must use #RRGGBB"}
	}

	icon := strings.TrimSpace(input.Icon)
	if len(icon) > tagIconMaxBytes {
		return db.TagSpec{}, ValidationError{Message: fmt.Sprintf("tag icon must be at most %d bytes", tagIconMaxBytes)}
	}
	if icon != "" && !tagIconPattern.MatchString(icon) {
		return db.TagSpec{}, ValidationError{Message: "tag icon may contain lowercase letters, numbers, underscores, and hyphens"}
	}

	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "tag metadata", tagJSONMaxBytes)
	if err != nil {
		return db.TagSpec{}, err
	}

	return db.TagSpec{
		Name:         name,
		Kind:         kind,
		Color:        color,
		Icon:         icon,
		MetadataJSON: metadataJSON,
	}, nil
}

func cleanSizedJSONObject(value string, field string, maxBytes int) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(cleaned)) {
		return "", ValidationError{Message: fmt.Sprintf("%s must be valid JSON", field)}
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(cleaned)); err != nil {
		return "", ValidationError{Message: fmt.Sprintf("%s must be valid JSON", field)}
	}
	cleaned = compact.String()
	if cleaned[0] != '{' {
		return "", ValidationError{Message: fmt.Sprintf("%s must be a JSON object", field)}
	}
	if len(cleaned) > maxBytes {
		return "", ValidationError{Message: fmt.Sprintf("%s must be at most %d bytes", field, maxBytes)}
	}

	return cleaned, nil
}

func tagRepositoryError(action string, err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrTagNotFound
	case errors.Is(err, db.ErrTagExists):
		return ErrTagExists
	default:
		return fmt.Errorf("%s: %w", action, err)
	}
}

func toTag(record db.TagRecord) Tag {
	return Tag{
		ID:           record.ID,
		BookID:       record.BookID,
		Name:         record.Name,
		Kind:         record.Kind,
		Color:        nullableString(record.Color),
		Icon:         nullableString(record.Icon),
		Status:       record.Status,
		MetadataJSON: record.MetadataJSON,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

func toTags(records []db.TagRecord) []Tag {
	tags := make([]Tag, 0, len(records))
	for _, record := range records {
		tags = append(tags, toTag(record))
	}

	return tags
}
