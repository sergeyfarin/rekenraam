package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"rekenraam/backend/internal/db"
)

const (
	payeeNameMaxBytes = 200
	payeeJSONMaxBytes = 10000
	payeeListLimit    = 2000
)

var (
	ErrPayeeNotFound = errors.New("payee not found")
	ErrPayeeExists   = errors.New("payee already exists")
)

type Payee struct {
	ID                       int64
	BookID                   int64
	Status                   string
	Name                     string
	DefaultAccountID         *int64
	DefaultCategoryAccountID *int64
	MetadataJSON             string
	EffectiveFrom            string
	ChangeReason             string
	CreatedAt                string
	UpdatedAt                string
}

type ListPayeesInput struct {
	Status          string
	IncludeArchived bool
	Query           string
}

type CreatePayeeInput struct {
	OwnerUserID              int64
	AuthSessionID            int64
	RequestID                string
	OriginType               string
	Operation                string
	Name                     string
	DefaultAccountID         *int64
	DefaultCategoryAccountID *int64
	MetadataJSON             string
	EffectiveFrom            string
	ChangeReason             string
}

type UpdatePayeeInput struct {
	OwnerUserID              int64
	AuthSessionID            int64
	RequestID                string
	OriginType               string
	Operation                string
	PayeeID                  int64
	Name                     *string
	DefaultAccountID         *int64
	ClearDefaultAccount      bool
	DefaultCategoryAccountID *int64
	ClearDefaultCategory     bool
	MetadataJSON             *string
	EffectiveFrom            string
	ChangeReason             string
}

type PayeeLifecycleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	PayeeID       int64
	EffectiveFrom string
	ChangeReason  string
}

type PayeeService struct {
	repository        *db.PayeeRepository
	accountRepository *db.AccountRepository
	now               func() time.Time
}

func NewPayeeService(repository *db.PayeeRepository, accountRepository *db.AccountRepository) *PayeeService {
	return &PayeeService{
		repository:        repository,
		accountRepository: accountRepository,
		now:               time.Now,
	}
}

func (s *PayeeService) ListPayees(ctx context.Context, input ListPayeesInput) ([]Payee, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && status != "active" && status != "archived" {
		return nil, ValidationError{Message: "payee status is invalid"}
	}

	records, err := s.repository.ListPayees(ctx, db.ListPayeesParams{
		BookID:          BookID,
		Status:          status,
		IncludeArchived: input.IncludeArchived,
		Query:           strings.TrimSpace(input.Query),
		Limit:           payeeListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list payees: %w", err)
	}

	return toPayees(records), nil
}

func (s *PayeeService) Payee(ctx context.Context, payeeID int64) (Payee, error) {
	if payeeID <= 0 {
		return Payee{}, ValidationError{Message: "payee id is required"}
	}

	record, err := s.repository.PayeeByID(ctx, BookID, payeeID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Payee{}, ErrPayeeNotFound
		}
		return Payee{}, fmt.Errorf("read payee: %w", err)
	}

	return toPayee(record), nil
}

func (s *PayeeService) CreatePayee(ctx context.Context, input CreatePayeeInput) (Payee, error) {
	if input.OwnerUserID <= 0 {
		return Payee{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	spec, err := s.payeeSpec(ctx, payeeSpecInput{
		Name:                     input.Name,
		DefaultAccountID:         input.DefaultAccountID,
		DefaultCategoryAccountID: input.DefaultCategoryAccountID,
		MetadataJSON:             input.MetadataJSON,
	})
	if err != nil {
		return Payee{}, err
	}

	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Payee{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "created payee")
	if err != nil {
		return Payee{}, err
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "payee.create"
	}

	record, err := s.repository.CreatePayee(ctx, db.CreatePayeeParams{
		BookID:        BookID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     operation,
		Spec:          spec,
		CreatedAt:     now.Format(time.RFC3339),
		EffectiveFrom: effectiveFrom,
		ChangeReason:  changeReason,
	})
	if err != nil {
		return Payee{}, mapPayeeDBError(err)
	}

	return toPayee(record), nil
}

func (s *PayeeService) UpdatePayee(ctx context.Context, input UpdatePayeeInput) (Payee, error) {
	if input.OwnerUserID <= 0 {
		return Payee{}, ValidationError{Message: "owner user is required"}
	}
	if input.PayeeID <= 0 {
		return Payee{}, ValidationError{Message: "payee id is required"}
	}

	current, err := s.repository.PayeeByID(ctx, BookID, input.PayeeID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Payee{}, ErrPayeeNotFound
		}
		return Payee{}, fmt.Errorf("read payee: %w", err)
	}

	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	defaultAccountID := nullableInt64FromSQL(current.DefaultAccountID)
	if input.ClearDefaultAccount {
		defaultAccountID = nil
	} else if input.DefaultAccountID != nil {
		defaultAccountID = input.DefaultAccountID
	}
	defaultCategoryAccountID := nullableInt64FromSQL(current.DefaultCategoryAccountID)
	if input.ClearDefaultCategory {
		defaultCategoryAccountID = nil
	} else if input.DefaultCategoryAccountID != nil {
		defaultCategoryAccountID = input.DefaultCategoryAccountID
	}
	metadataJSON := current.MetadataJSON
	if input.MetadataJSON != nil {
		metadataJSON = *input.MetadataJSON
	}

	spec, err := s.payeeSpec(ctx, payeeSpecInput{
		Name:                     name,
		DefaultAccountID:         defaultAccountID,
		DefaultCategoryAccountID: defaultCategoryAccountID,
		MetadataJSON:             metadataJSON,
	})
	if err != nil {
		return Payee{}, err
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Payee{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated payee")
	if err != nil {
		return Payee{}, err
	}
	operation := strings.TrimSpace(input.Operation)
	if operation == "" {
		operation = "payee.update"
	}

	record, err := s.repository.UpdatePayee(ctx, db.UpdatePayeeParams{
		BookID:        BookID,
		PayeeID:       input.PayeeID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     operation,
		Spec:          spec,
		RecordedAt:    now.Format(time.RFC3339),
		EffectiveFrom: effectiveFrom,
		ChangeReason:  changeReason,
	})
	if err != nil {
		return Payee{}, mapPayeeDBError(err)
	}

	return toPayee(record), nil
}

func (s *PayeeService) ArchivePayee(ctx context.Context, input PayeeLifecycleInput) (Payee, error) {
	return s.changePayeeStatus(ctx, input, "archived", "payee.archive", "archived payee")
}

func (s *PayeeService) RestorePayee(ctx context.Context, input PayeeLifecycleInput) (Payee, error) {
	return s.changePayeeStatus(ctx, input, "active", "payee.restore", "restored payee")
}

func (s *PayeeService) changePayeeStatus(ctx context.Context, input PayeeLifecycleInput, status string, operation string, fallbackReason string) (Payee, error) {
	if input.OwnerUserID <= 0 {
		return Payee{}, ValidationError{Message: "owner user is required"}
	}
	if input.PayeeID <= 0 {
		return Payee{}, ValidationError{Message: "payee id is required"}
	}

	current, err := s.repository.PayeeByID(ctx, BookID, input.PayeeID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Payee{}, ErrPayeeNotFound
		}
		return Payee{}, fmt.Errorf("read payee: %w", err)
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Payee{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, fallbackReason)
	if err != nil {
		return Payee{}, err
	}

	record, err := s.repository.UpdatePayee(ctx, db.UpdatePayeeParams{
		BookID:        BookID,
		PayeeID:       input.PayeeID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    input.OriginType,
		Operation:     operation,
		Spec: db.PayeeSpec{
			Name:                     current.Name,
			NormalizedName:           current.NormalizedName,
			DefaultAccountID:         current.DefaultAccountID,
			DefaultCategoryAccountID: current.DefaultCategoryAccountID,
			MetadataJSON:             current.MetadataJSON,
		},
		Status:        status,
		RecordedAt:    now.Format(time.RFC3339),
		EffectiveFrom: effectiveFrom,
		ChangeReason:  changeReason,
	})
	if err != nil {
		return Payee{}, mapPayeeDBError(err)
	}

	return toPayee(record), nil
}

type payeeSpecInput struct {
	Name                     string
	DefaultAccountID         *int64
	DefaultCategoryAccountID *int64
	MetadataJSON             string
}

func (s *PayeeService) payeeSpec(ctx context.Context, input payeeSpecInput) (db.PayeeSpec, error) {
	name, normalizedName, err := cleanPayeeName(input.Name)
	if err != nil {
		return db.PayeeSpec{}, err
	}

	if input.DefaultAccountID != nil {
		account, err := s.accountRepository.CurrentAccountByID(ctx, BookID, *input.DefaultAccountID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return db.PayeeSpec{}, ValidationError{Message: "default account is invalid"}
			}
			return db.PayeeSpec{}, fmt.Errorf("read default account: %w", err)
		}
		if account.Status != "active" || !account.AllowsPostings || account.IsSystem || (account.AccountClass != "asset" && account.AccountClass != "liability") {
			return db.PayeeSpec{}, ValidationError{Message: "default account is invalid"}
		}
	}

	if input.DefaultCategoryAccountID != nil {
		account, err := s.accountRepository.CurrentAccountByID(ctx, BookID, *input.DefaultCategoryAccountID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return db.PayeeSpec{}, ValidationError{Message: "default category is invalid"}
			}
			return db.PayeeSpec{}, fmt.Errorf("read default category: %w", err)
		}
		if account.Status != "active" || !account.AllowsPostings || (account.AccountClass != "income" && account.AccountClass != "expense") {
			return db.PayeeSpec{}, ValidationError{Message: "default category is invalid"}
		}
	}

	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", payeeJSONMaxBytes)
	if err != nil {
		return db.PayeeSpec{}, err
	}

	return db.PayeeSpec{
		Name:                     name,
		NormalizedName:           normalizedName,
		DefaultAccountID:         nullableInt64(input.DefaultAccountID),
		DefaultCategoryAccountID: nullableInt64(input.DefaultCategoryAccountID),
		MetadataJSON:             metadataJSON,
	}, nil
}

func cleanPayeeName(value string) (string, string, error) {
	cleaned := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if cleaned == "" {
		return "", "", ValidationError{Message: "payee name is required"}
	}
	if len(cleaned) > payeeNameMaxBytes {
		return "", "", ValidationError{Message: fmt.Sprintf("payee name must be at most %d bytes", payeeNameMaxBytes)}
	}
	for _, r := range cleaned {
		if unicode.IsControl(r) {
			return "", "", ValidationError{Message: "payee name must not contain control characters"}
		}
	}

	return cleaned, strings.ToLower(cleaned), nil
}

func mapPayeeDBError(err error) error {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return ErrPayeeNotFound
	case errors.Is(err, db.ErrPayeeExists):
		return ErrPayeeExists
	default:
		return fmt.Errorf("payee repository: %w", err)
	}
}

func toPayees(records []db.PayeeRecord) []Payee {
	payees := make([]Payee, 0, len(records))
	for _, record := range records {
		payees = append(payees, toPayee(record))
	}

	return payees
}

func toPayee(record db.PayeeRecord) Payee {
	return Payee{
		ID:                       record.ID,
		BookID:                   record.BookID,
		Status:                   record.Status,
		Name:                     record.Name,
		DefaultAccountID:         nullableInt64FromSQL(record.DefaultAccountID),
		DefaultCategoryAccountID: nullableInt64FromSQL(record.DefaultCategoryAccountID),
		MetadataJSON:             record.MetadataJSON,
		EffectiveFrom:            record.EffectiveFrom,
		ChangeReason:             record.ChangeReason,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.RecordedAt,
	}
}

func nullableInt64FromSQL(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copied := value.Int64
	return &copied
}
