package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

const (
	bookCodeMaxBytes = 64
	bookNameMaxBytes = 200
	defaultBookCode  = "personal"
)

var (
	ErrBookAlreadyExists = errors.New("book already exists")
	ErrBookNotFound      = errors.New("book not found")
	ErrUnauthenticated   = errors.New("unauthenticated")
	bookCodePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

type BookService struct {
	repository   *db.BookRepository
	setupService *SetupService
	now          func() time.Time
}

type CreateBookInput struct {
	OwnerUserID int64
	Code        string
	Name        string
}

type Book struct {
	ID                      int64
	OwnerUserID             int64
	Code                    string
	Name                    string
	BaseCurrencyCommodityID *int64
	CreatedAt               string
	UpdatedAt               string
}

type CreateBookResult struct {
	Book        Book
	SetupStatus SetupStatus
}

func NewBookService(repository *db.BookRepository, setupService *SetupService) *BookService {
	return &BookService{
		repository:   repository,
		setupService: setupService,
		now:          time.Now,
	}
}

func (s *BookService) CurrentBook(ctx context.Context) (Book, error) {
	record, err := s.repository.CurrentBook(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Book{}, ErrBookNotFound
		}
		return Book{}, fmt.Errorf("read current book: %w", err)
	}

	return toBook(record), nil
}

func (s *BookService) CreateBook(ctx context.Context, input CreateBookInput) (CreateBookResult, error) {
	if input.OwnerUserID <= 0 {
		return CreateBookResult{}, ErrUnauthenticated
	}

	code := normalizeBookCode(input.Code)
	if code == "" {
		code = defaultBookCode
	}
	if err := validateBookCode(code); err != nil {
		return CreateBookResult{}, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreateBookResult{}, ValidationError{Message: "book name is required"}
	}
	if len(name) > bookNameMaxBytes {
		return CreateBookResult{}, ValidationError{Message: fmt.Sprintf("book name must be at most %d bytes", bookNameMaxBytes)}
	}

	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.CompleteBookSetup(ctx, db.CompleteBookSetupParams{
		OwnerUserID: input.OwnerUserID,
		Code:        code,
		Name:        name,
		CreatedAt:   now,
	})
	if err != nil {
		if errors.Is(err, db.ErrBookExists) {
			return CreateBookResult{}, ErrBookAlreadyExists
		}
		return CreateBookResult{}, fmt.Errorf("persist book setup: %w", err)
	}

	setupStatus, err := s.setupService.Status(ctx)
	if err != nil {
		return CreateBookResult{}, fmt.Errorf("reload setup status: %w", err)
	}

	return CreateBookResult{
		Book:        toBook(record),
		SetupStatus: setupStatus,
	}, nil
}

func normalizeBookCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func validateBookCode(code string) error {
	if len(code) > bookCodeMaxBytes {
		return ValidationError{Message: fmt.Sprintf("book code must be at most %d bytes", bookCodeMaxBytes)}
	}
	if !bookCodePattern.MatchString(code) {
		return ValidationError{Message: "book code may contain lowercase letters, numbers, underscores, and hyphens"}
	}
	return nil
}

func toBook(record db.BookRecord) Book {
	var baseCurrencyCommodityID *int64
	if record.BaseCurrencyCommodityID.Valid {
		value := record.BaseCurrencyCommodityID.Int64
		baseCurrencyCommodityID = &value
	}

	return Book{
		ID:                      record.ID,
		OwnerUserID:             record.OwnerUserID,
		Code:                    record.Code,
		Name:                    record.Name,
		BaseCurrencyCommodityID: baseCurrencyCommodityID,
		CreatedAt:               record.CreatedAt,
		UpdatedAt:               record.UpdatedAt,
	}
}
