package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"rekenraam/backend/internal/db"
)

const (
	institutionNameMaxBytes    = 200
	institutionWebsiteMaxBytes = 500
	institutionAssetMaxBytes   = 1000
	institutionCommentMaxBytes = 5000
	institutionJSONMaxBytes    = 10000
	institutionListLimit       = 2000
)

var ErrInstitutionNotFound = errors.New("institution not found")

var institutionKinds = map[string]bool{
	"bank":            true,
	"credit_union":    true,
	"brokerage":       true,
	"card_issuer":     true,
	"lender":          true,
	"insurance":       true,
	"employer":        true,
	"rewards_program": true,
	"government":      true,
	"other":           true,
}

type Institution struct {
	ID              int64
	BookID          int64
	Status          string
	Name            string
	Kind            string
	CountryCode     string
	Website         string
	LogoURL         string
	LogoSmallURL    string
	BackdropURL     string
	AddressJSON     string
	CommentMarkdown string
	MetadataJSON    string
	EffectiveFrom   string
	ChangeReason    string
	CreatedAt       string
	UpdatedAt       string
}

type ListInstitutionsInput struct {
	Status          string
	IncludeArchived bool
	Query           string
}

type CreateInstitutionInput struct {
	OwnerUserID     int64
	AuthSessionID   int64
	RequestID       string
	Name            string
	Kind            string
	CountryCode     string
	Website         string
	LogoURL         string
	LogoSmallURL    string
	BackdropURL     string
	AddressJSON     string
	CommentMarkdown string
	MetadataJSON    string
	EffectiveFrom   string
	ChangeReason    string
}

type UpdateInstitutionInput struct {
	OwnerUserID     int64
	AuthSessionID   int64
	RequestID       string
	InstitutionID   int64
	Name            string
	Kind            string
	CountryCode     string
	Website         string
	LogoURL         string
	LogoSmallURL    string
	BackdropURL     string
	AddressJSON     string
	CommentMarkdown string
	MetadataJSON    string
	EffectiveFrom   string
	ChangeReason    string
}

type InstitutionLifecycleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	InstitutionID int64
	EffectiveFrom string
	ChangeReason  string
}

type InstitutionService struct {
	repository *db.InstitutionRepository
	now        func() time.Time
}

func NewInstitutionService(repository *db.InstitutionRepository) *InstitutionService {
	return &InstitutionService{
		repository: repository,
		now:        time.Now,
	}
}

func (s *InstitutionService) ListInstitutions(ctx context.Context, input ListInstitutionsInput) ([]Institution, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && status != "active" && status != "archived" {
		return nil, ValidationError{Message: "institution status is invalid"}
	}

	records, err := s.repository.ListInstitutions(ctx, db.ListInstitutionsParams{
		BookID:          BookID,
		Status:          status,
		IncludeArchived: input.IncludeArchived,
		Query:           strings.TrimSpace(input.Query),
		Limit:           institutionListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list institutions: %w", err)
	}

	return toInstitutions(records), nil
}

func (s *InstitutionService) Institution(ctx context.Context, institutionID int64) (Institution, error) {
	if institutionID <= 0 {
		return Institution{}, ValidationError{Message: "institution id is required"}
	}

	record, err := s.repository.CurrentInstitutionByID(ctx, BookID, institutionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Institution{}, ErrInstitutionNotFound
		}
		return Institution{}, fmt.Errorf("read institution: %w", err)
	}

	return toInstitution(record), nil
}

func (s *InstitutionService) InstitutionVersions(ctx context.Context, institutionID int64) ([]Institution, error) {
	if institutionID <= 0 {
		return nil, ValidationError{Message: "institution id is required"}
	}

	records, err := s.repository.ListInstitutionVersions(ctx, BookID, institutionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrInstitutionNotFound
		}
		return nil, fmt.Errorf("read institution versions: %w", err)
	}

	return toInstitutions(records), nil
}

func (s *InstitutionService) CreateInstitution(ctx context.Context, input CreateInstitutionInput) (Institution, error) {
	if input.OwnerUserID <= 0 {
		return Institution{}, ValidationError{Message: "owner user is required"}
	}

	spec, err := institutionSpec(institutionSpecInput{
		Name:            input.Name,
		Kind:            input.Kind,
		CountryCode:     input.CountryCode,
		Website:         input.Website,
		LogoURL:         input.LogoURL,
		LogoSmallURL:    input.LogoSmallURL,
		BackdropURL:     input.BackdropURL,
		AddressJSON:     input.AddressJSON,
		CommentMarkdown: input.CommentMarkdown,
		MetadataJSON:    input.MetadataJSON,
	})
	if err != nil {
		return Institution{}, err
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Institution{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "created institution")
	if err != nil {
		return Institution{}, err
	}
	record, err := s.repository.CreateInstitution(ctx, db.CreateInstitutionParams{
		BookID:          BookID,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		Spec:            spec,
		ChangeReason:    changeReason,
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Institution{}, ErrInstitutionNotFound
		}
		return Institution{}, fmt.Errorf("persist institution: %w", err)
	}

	return toInstitution(record), nil
}

func (s *InstitutionService) UpdateInstitution(ctx context.Context, input UpdateInstitutionInput) (Institution, error) {
	if input.OwnerUserID <= 0 {
		return Institution{}, ValidationError{Message: "owner user is required"}
	}
	if input.InstitutionID <= 0 {
		return Institution{}, ValidationError{Message: "institution id is required"}
	}

	spec, err := institutionSpec(institutionSpecInput{
		Name:            input.Name,
		Kind:            input.Kind,
		CountryCode:     input.CountryCode,
		Website:         input.Website,
		LogoURL:         input.LogoURL,
		LogoSmallURL:    input.LogoSmallURL,
		BackdropURL:     input.BackdropURL,
		AddressJSON:     input.AddressJSON,
		CommentMarkdown: input.CommentMarkdown,
		MetadataJSON:    input.MetadataJSON,
	})
	if err != nil {
		return Institution{}, err
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Institution{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated institution")
	if err != nil {
		return Institution{}, err
	}
	record, err := s.repository.UpdateInstitution(ctx, db.UpdateInstitutionParams{
		BookID:          BookID,
		InstitutionID:   input.InstitutionID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		Operation:       "institution.update",
		Spec:            spec,
		ChangeReason:    changeReason,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Institution{}, ErrInstitutionNotFound
		}
		return Institution{}, fmt.Errorf("persist institution update: %w", err)
	}

	return toInstitution(record), nil
}

func (s *InstitutionService) ArchiveInstitution(ctx context.Context, input InstitutionLifecycleInput) (Institution, error) {
	return s.changeInstitutionStatus(ctx, input, "archived", "archived institution", "institution.archive")
}

func (s *InstitutionService) RestoreInstitution(ctx context.Context, input InstitutionLifecycleInput) (Institution, error) {
	return s.changeInstitutionStatus(ctx, input, "active", "restored institution", "institution.restore")
}

func (s *InstitutionService) changeInstitutionStatus(ctx context.Context, input InstitutionLifecycleInput, status string, reason string, operation string) (Institution, error) {
	if input.OwnerUserID <= 0 {
		return Institution{}, ValidationError{Message: "owner user is required"}
	}
	if input.InstitutionID <= 0 {
		return Institution{}, ValidationError{Message: "institution id is required"}
	}

	current, err := s.Institution(ctx, input.InstitutionID)
	if err != nil {
		return Institution{}, err
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Institution{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, reason)
	if err != nil {
		return Institution{}, err
	}
	record, err := s.repository.UpdateInstitution(ctx, db.UpdateInstitutionParams{
		BookID:          BookID,
		InstitutionID:   input.InstitutionID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		Operation:       operation,
		Spec:            institutionSpecFromStored(current),
		Status:          status,
		ChangeReason:    changeReason,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Institution{}, ErrInstitutionNotFound
		}
		return Institution{}, fmt.Errorf("persist institution status: %w", err)
	}

	return toInstitution(record), nil
}

type institutionSpecInput struct {
	Name            string
	Kind            string
	CountryCode     string
	Website         string
	LogoURL         string
	LogoSmallURL    string
	BackdropURL     string
	AddressJSON     string
	CommentMarkdown string
	MetadataJSON    string
}

func institutionSpec(input institutionSpecInput) (db.InstitutionSpec, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return db.InstitutionSpec{}, ValidationError{Message: "institution name is required"}
	}
	if len(name) > institutionNameMaxBytes {
		return db.InstitutionSpec{}, ValidationError{Message: fmt.Sprintf("institution name must be at most %d bytes", institutionNameMaxBytes)}
	}

	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "other"
	}
	if !institutionKinds[kind] {
		return db.InstitutionSpec{}, ValidationError{Message: "institution kind is invalid"}
	}

	countryCode := strings.ToUpper(strings.TrimSpace(input.CountryCode))
	if countryCode != "" {
		if len(countryCode) != 2 {
			return db.InstitutionSpec{}, ValidationError{Message: "country code must be a 2-letter code"}
		}
		for _, r := range countryCode {
			if r < 'A' || r > 'Z' {
				return db.InstitutionSpec{}, ValidationError{Message: "country code must use letters A-Z"}
			}
		}
	}

	website, err := cleanHTTPURL(input.Website, "institution website", institutionWebsiteMaxBytes)
	if err != nil {
		return db.InstitutionSpec{}, err
	}
	logoURL, err := cleanAssetReference(input.LogoURL, "institution logo URL")
	if err != nil {
		return db.InstitutionSpec{}, err
	}
	logoSmallURL, err := cleanAssetReference(input.LogoSmallURL, "institution small logo URL")
	if err != nil {
		return db.InstitutionSpec{}, err
	}
	backdropURL, err := cleanAssetReference(input.BackdropURL, "institution backdrop URL")
	if err != nil {
		return db.InstitutionSpec{}, err
	}

	addressJSON, err := cleanJSONObject(input.AddressJSON, "institution address")
	if err != nil {
		return db.InstitutionSpec{}, err
	}

	commentMarkdown := strings.TrimSpace(input.CommentMarkdown)
	if len(commentMarkdown) > institutionCommentMaxBytes {
		return db.InstitutionSpec{}, ValidationError{Message: fmt.Sprintf("institution comment must be at most %d bytes", institutionCommentMaxBytes)}
	}

	metadataJSON, err := cleanJSONObject(input.MetadataJSON, "institution metadata")
	if err != nil {
		return db.InstitutionSpec{}, err
	}

	return db.InstitutionSpec{
		Name:            name,
		Kind:            kind,
		CountryCode:     countryCode,
		Website:         website,
		LogoURL:         logoURL,
		LogoSmallURL:    logoSmallURL,
		BackdropURL:     backdropURL,
		AddressJSON:     addressJSON,
		CommentMarkdown: commentMarkdown,
		MetadataJSON:    metadataJSON,
	}, nil
}

func institutionSpecFromStored(institution Institution) db.InstitutionSpec {
	return db.InstitutionSpec{
		Name:            institution.Name,
		Kind:            institution.Kind,
		CountryCode:     institution.CountryCode,
		Website:         institution.Website,
		LogoURL:         institution.LogoURL,
		LogoSmallURL:    institution.LogoSmallURL,
		BackdropURL:     institution.BackdropURL,
		AddressJSON:     institution.AddressJSON,
		CommentMarkdown: institution.CommentMarkdown,
		MetadataJSON:    institution.MetadataJSON,
	}
}

func cleanJSONObject(value string, field string) (string, error) {
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
	if len(cleaned) > institutionJSONMaxBytes {
		return "", ValidationError{Message: fmt.Sprintf("%s must be at most %d bytes", field, institutionJSONMaxBytes)}
	}

	return cleaned, nil
}

func cleanHTTPURL(value string, field string, maxBytes int) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", nil
	}
	if len(cleaned) > maxBytes {
		return "", ValidationError{Message: fmt.Sprintf("%s must be at most %d bytes", field, maxBytes)}
	}

	parsedURL, err := url.Parse(cleaned)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", ValidationError{Message: fmt.Sprintf("%s must be an absolute URL", field)}
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return "", ValidationError{Message: fmt.Sprintf("%s must use http or https", field)}
	}

	return cleaned, nil
}

func cleanAssetReference(value string, field string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", nil
	}
	if len(cleaned) > institutionAssetMaxBytes {
		return "", ValidationError{Message: fmt.Sprintf("%s must be at most %d bytes", field, institutionAssetMaxBytes)}
	}

	if strings.HasPrefix(cleaned, "/") && !strings.HasPrefix(cleaned, "//") {
		for _, r := range cleaned {
			if unicode.IsControl(r) {
				return "", ValidationError{Message: fmt.Sprintf("%s must not contain control characters", field)}
			}
		}
		return cleaned, nil
	}

	return cleanHTTPURL(cleaned, field, institutionAssetMaxBytes)
}

func toInstitution(record db.InstitutionRecord) Institution {
	return Institution{
		ID:              record.ID,
		BookID:          record.BookID,
		Status:          record.Status,
		Name:            record.Name,
		Kind:            record.Kind,
		CountryCode:     nullableString(record.CountryCode),
		Website:         nullableString(record.Website),
		LogoURL:         nullableString(record.LogoURL),
		LogoSmallURL:    nullableString(record.LogoSmallURL),
		BackdropURL:     nullableString(record.BackdropURL),
		AddressJSON:     record.AddressJSON,
		CommentMarkdown: record.CommentMarkdown,
		MetadataJSON:    record.MetadataJSON,
		EffectiveFrom:   record.EffectiveFrom,
		ChangeReason:    record.ChangeReason,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.RecordedAt,
	}
}

func toInstitutions(records []db.InstitutionRecord) []Institution {
	institutions := make([]Institution, 0, len(records))
	for _, record := range records {
		institutions = append(institutions, toInstitution(record))
	}

	return institutions
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}

	return value.String
}
