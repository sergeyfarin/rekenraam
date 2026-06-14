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
	accountNameMaxBytes        = 200
	accountCodeMaxBytes        = 100
	accountNumberLast4MaxBytes = 8
	accountExternalRefMaxBytes = 200
	accountCommentMaxBytes     = 5000
	accountJSONMaxBytes        = 10000
	accountListLimit           = 2000
	systemAccountDate          = "0001-01-01"
)

var (
	ErrAccountNotFound         = errors.New("account not found")
	ErrSystemAccountProtected  = errors.New("system account cannot be changed through account management")
	ErrAccountInvalidLifecycle = errors.New("account lifecycle transition is invalid")
	ErrAccountParentInvalid    = errors.New("account parent is invalid")
	ErrAccountReferenceInvalid = errors.New("account reference is invalid")
	ErrAccountCurrencyLocked   = errors.New("account currency cannot be changed after postings exist")
	ErrAccountDeleteNotAllowed = errors.New("account cannot be deleted")
)

var accountClasses = map[string]bool{
	"asset":     true,
	"liability": true,
	"equity":    true,
	"income":    true,
	"expense":   true,
}

var systemAccountSpecs = []db.SystemAccountSpec{
	{Role: "opening_balance", AccountClass: "equity", AccountKind: "equity"},
	{Role: "import_imbalance", AccountClass: "equity", AccountKind: "equity"},
	{Role: "retained_earnings", AccountClass: "equity", AccountKind: "equity"},
	{Role: "unassigned_income", AccountClass: "income", AccountKind: "income"},
	{Role: "unassigned_expense", AccountClass: "expense", AccountKind: "expense"},
	{Role: "transfer_clearing", AccountClass: "asset", AccountKind: "receivable"},
	{Role: "commodity_trading", AccountClass: "equity", AccountKind: "equity"},
}

type Account struct {
	ID                    int64
	BookID                int64
	IsSystem              bool
	SystemRole            string
	Status                string
	Code                  string
	Name                  string
	AccountClass          string
	AccountKind           string
	ParentAccountID       *int64
	InstitutionID         *int64
	CountryCode           string
	DefaultCommodityID    *int64
	DefaultCommodityCode  string
	QuantityScaleOverride *int
	AllowsPostings        bool
	NumberLast4           string
	ExternalRefHint       string
	CommentMarkdown       string
	MetadataJSON          string
	OpenedOn              string
	ClosedOn              string
	EffectiveFrom         string
	ChangeReason          string
	CreatedAt             string
	UpdatedAt             string
}

type ListAccountsInput struct {
	Status          string
	AccountClass    string
	IncludeArchived bool
	IncludeSystem   bool
	Query           string
}

type CreateAccountInput struct {
	OwnerUserID           int64
	AuthSessionID         int64
	RequestID             string
	OriginType            string
	Operation             string
	Code                  string
	Name                  string
	AccountClass          string
	AccountKind           string
	ParentAccountID       *int64
	InstitutionID         *int64
	CountryCode           string
	DefaultCommodityID    *int64
	DefaultCommodityCode  string
	QuantityScaleOverride *int
	AllowsPostings        *bool
	NumberLast4           string
	ExternalRefHint       string
	CommentMarkdown       string
	MetadataJSON          string
	OpenedOn              string
	EffectiveFrom         string
	ChangeReason          string
}

type UpdateAccountInput struct {
	OwnerUserID           int64
	AuthSessionID         int64
	RequestID             string
	OriginType            string
	Operation             string
	AccountID             int64
	Code                  string
	Name                  string
	AccountClass          string
	AccountKind           string
	ParentAccountID       *int64
	InstitutionID         *int64
	CountryCode           string
	DefaultCommodityID    *int64
	DefaultCommodityCode  string
	QuantityScaleOverride *int
	AllowsPostings        *bool
	NumberLast4           string
	ExternalRefHint       string
	CommentMarkdown       string
	MetadataJSON          string
	OpenedOn              string
	EffectiveFrom         string
	ChangeReason          string
}

type AccountLifecycleInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	AccountID     int64
	EffectiveFrom string
	ClosedOn      string
	ChangeReason  string
}

type DeleteAccountInput struct {
	OwnerUserID int64
	AccountID   int64
}

type EnsureSystemAccountsInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	OriginType    string
	Operation     string
}

type EnsureSystemAccountsResult struct {
	Accounts    []Account
	SetupStatus SetupStatus
}

type AccountService struct {
	repository            *db.AccountRepository
	institutionRepository *db.InstitutionRepository
	setupService          *SetupService
	now                   func() time.Time
}

func NewAccountService(repository *db.AccountRepository, institutionRepository *db.InstitutionRepository, setupService *SetupService) *AccountService {
	return &AccountService{
		repository:            repository,
		institutionRepository: institutionRepository,
		setupService:          setupService,
		now:                   time.Now,
	}
}

func (s *AccountService) ListAccounts(ctx context.Context, input ListAccountsInput) ([]Account, error) {
	status := strings.TrimSpace(input.Status)
	if status != "" && status != "active" && status != "closed" && status != "archived" {
		return nil, ValidationError{Message: "account status is invalid"}
	}

	accountClass := strings.TrimSpace(input.AccountClass)
	if accountClass != "" && !accountClasses[accountClass] {
		return nil, ValidationError{Message: "account class is invalid"}
	}

	records, err := s.repository.ListAccounts(ctx, db.ListAccountsParams{
		BookID:          BookID,
		Status:          status,
		AccountClass:    accountClass,
		IncludeArchived: input.IncludeArchived,
		IncludeSystem:   input.IncludeSystem,
		Query:           strings.TrimSpace(input.Query),
		Limit:           accountListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	return toAccounts(records), nil
}

func (s *AccountService) Account(ctx context.Context, accountID int64) (Account, error) {
	if accountID <= 0 {
		return Account{}, ValidationError{Message: "account id is required"}
	}

	record, err := s.repository.CurrentAccountByID(ctx, BookID, accountID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, fmt.Errorf("read account: %w", err)
	}

	return toAccount(record), nil
}

func (s *AccountService) AccountVersions(ctx context.Context, accountID int64) ([]Account, error) {
	if accountID <= 0 {
		return nil, ValidationError{Message: "account id is required"}
	}

	records, err := s.repository.ListAccountVersions(ctx, BookID, accountID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("read account versions: %w", err)
	}

	return toAccounts(records), nil
}

func (s *AccountService) CreateAccount(ctx context.Context, input CreateAccountInput) (Account, error) {
	if input.OwnerUserID <= 0 {
		return Account{}, ValidationError{Message: "owner user is required"}
	}

	spec, err := s.accountSpec(ctx, accountSpecInput{
		Code:                  input.Code,
		Name:                  input.Name,
		AccountClass:          input.AccountClass,
		AccountKind:           input.AccountKind,
		ParentAccountID:       input.ParentAccountID,
		InstitutionID:         input.InstitutionID,
		CountryCode:           input.CountryCode,
		DefaultCommodityID:    input.DefaultCommodityID,
		DefaultCommodityCode:  input.DefaultCommodityCode,
		QuantityScaleOverride: input.QuantityScaleOverride,
		AllowsPostings:        input.AllowsPostings,
		NumberLast4:           input.NumberLast4,
		ExternalRefHint:       input.ExternalRefHint,
		CommentMarkdown:       input.CommentMarkdown,
		MetadataJSON:          input.MetadataJSON,
	})
	if err != nil {
		return Account{}, err
	}

	now := s.now().UTC()
	openedOn, err := cleanAccountOpenedOn(input.OpenedOn, input.EffectiveFrom, now)
	if err != nil {
		return Account{}, err
	}
	effectiveInput := input.EffectiveFrom
	if strings.TrimSpace(effectiveInput) == "" {
		effectiveInput = openedOn
	}
	effectiveFrom, err := cleanEffectiveFrom(effectiveInput, now)
	if err != nil {
		return Account{}, err
	}
	if effectiveFrom < openedOn {
		return Account{}, ValidationError{Message: "account effective date must not be before opened date"}
	}
	spec.OpenedOn = openedOn
	changeReason, err := cleanChangeReason(input.ChangeReason, "created account")
	if err != nil {
		return Account{}, err
	}
	record, err := s.repository.CreateAccount(ctx, db.CreateAccountParams{
		BookID:          BookID,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       input.Operation,
		Spec:            spec,
		ChangeReason:    changeReason,
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Account{}, ErrAccountReferenceInvalid
		}
		return Account{}, fmt.Errorf("persist account: %w", err)
	}

	return toAccount(record), nil
}

func (s *AccountService) UpdateAccount(ctx context.Context, input UpdateAccountInput) (Account, error) {
	if input.OwnerUserID <= 0 {
		return Account{}, ValidationError{Message: "owner user is required"}
	}
	if input.AccountID <= 0 {
		return Account{}, ValidationError{Message: "account id is required"}
	}

	current, err := s.Account(ctx, input.AccountID)
	if err != nil {
		return Account{}, err
	}
	if current.IsSystem {
		return Account{}, ErrSystemAccountProtected
	}

	spec, err := s.accountSpec(ctx, accountSpecInput{
		AccountID:             input.AccountID,
		Code:                  input.Code,
		Name:                  input.Name,
		AccountClass:          input.AccountClass,
		AccountKind:           input.AccountKind,
		ParentAccountID:       input.ParentAccountID,
		InstitutionID:         input.InstitutionID,
		CountryCode:           input.CountryCode,
		DefaultCommodityID:    input.DefaultCommodityID,
		DefaultCommodityCode:  input.DefaultCommodityCode,
		QuantityScaleOverride: input.QuantityScaleOverride,
		AllowsPostings:        input.AllowsPostings,
		NumberLast4:           input.NumberLast4,
		ExternalRefHint:       input.ExternalRefHint,
		CommentMarkdown:       input.CommentMarkdown,
		MetadataJSON:          input.MetadataJSON,
	})
	if err != nil {
		return Account{}, err
	}
	commodityChanged, err := s.defaultCommodityChanged(ctx, current, spec, input.DefaultCommodityCode)
	if err != nil {
		return Account{}, err
	}
	if commodityChanged {
		hasPostings, err := s.repository.AccountHasPostings(ctx, BookID, input.AccountID)
		if err != nil {
			return Account{}, fmt.Errorf("check account postings: %w", err)
		}
		if hasPostings {
			return Account{}, ErrAccountCurrencyLocked
		}
	}

	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return Account{}, err
	}
	openedOn, err := cleanAccountOpenedOn(input.OpenedOn, current.OpenedOn, now)
	if err != nil {
		return Account{}, err
	}
	if effectiveFrom < openedOn {
		return Account{}, ValidationError{Message: "account effective date must not be before opened date"}
	}
	if current.ClosedOn != "" && openedOn > current.ClosedOn {
		return Account{}, ValidationError{Message: "account opened date must not be after closed date"}
	}
	spec.OpenedOn = openedOn
	spec.ClosedOn = nullableSQLString(current.ClosedOn)
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated account")
	if err != nil {
		return Account{}, err
	}
	record, err := s.repository.UpdateAccount(ctx, db.UpdateAccountParams{
		BookID:          BookID,
		AccountID:       input.AccountID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       input.Operation,
		Spec:            spec,
		ChangeReason:    changeReason,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, fmt.Errorf("persist account update: %w", err)
	}

	return toAccount(record), nil
}

func (s *AccountService) CloseAccount(ctx context.Context, input AccountLifecycleInput) (Account, error) {
	return s.changeAccountStatus(ctx, input, "closed", "closed account", "account.close", "active")
}

func (s *AccountService) ReopenAccount(ctx context.Context, input AccountLifecycleInput) (Account, error) {
	return s.changeAccountStatus(ctx, input, "active", "reopened account", "account.reopen", "closed")
}

func (s *AccountService) ArchiveAccount(ctx context.Context, input AccountLifecycleInput) (Account, error) {
	return s.changeAccountStatus(ctx, input, "archived", "archived account", "account.archive", "closed")
}

func (s *AccountService) RestoreAccount(ctx context.Context, input AccountLifecycleInput) (Account, error) {
	return s.changeAccountStatus(ctx, input, "closed", "restored account", "account.restore", "archived")
}

func (s *AccountService) DeleteUnusedAccount(ctx context.Context, input DeleteAccountInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if input.AccountID <= 0 {
		return ValidationError{Message: "account id is required"}
	}

	current, err := s.Account(ctx, input.AccountID)
	if err != nil {
		return err
	}
	if current.IsSystem {
		return ErrSystemAccountProtected
	}

	if err := s.repository.DeleteUnusedAccount(ctx, db.DeleteAccountParams{
		BookID:    BookID,
		AccountID: input.AccountID,
	}); err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			return ErrAccountNotFound
		case errors.Is(err, db.ErrAccountDeleteNotPermitted):
			return ErrAccountDeleteNotAllowed
		default:
			return fmt.Errorf("delete account: %w", err)
		}
	}

	return nil
}

func (s *AccountService) EnsureSystemAccounts(ctx context.Context, input EnsureSystemAccountsInput) (EnsureSystemAccountsResult, error) {
	if input.OwnerUserID <= 0 {
		return EnsureSystemAccountsResult{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	starterCashSpecs, err := s.repository.ListActiveCurrencyStarterSpecs(ctx, BookID)
	if err != nil {
		return EnsureSystemAccountsResult{}, fmt.Errorf("read starter cash account currencies: %w", err)
	}

	result, err := s.repository.EnsureSystemAccounts(ctx, db.EnsureSystemAccountsParams{
		BookID:               BookID,
		ChangedByUserID:      input.OwnerUserID,
		AuthSessionID:        input.AuthSessionID,
		RequestID:            input.RequestID,
		OriginType:           input.OriginType,
		Operation:            input.Operation,
		Specs:                systemAccountSpecs,
		StarterCashSpecs:     starterCashSpecs,
		CreatedAt:            now.Format(time.RFC3339),
		EffectiveFrom:        systemAccountDate,
		StarterEffectiveFrom: now.Format(time.DateOnly),
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrSystemAccountsSetupComplete):
			return EnsureSystemAccountsResult{}, ErrSetupAlreadyComplete
		case errors.Is(err, db.ErrNotFound):
			return EnsureSystemAccountsResult{}, ErrAccountReferenceInvalid
		default:
			return EnsureSystemAccountsResult{}, fmt.Errorf("persist system accounts setup: %w", err)
		}
	}

	setupStatus, err := s.setupService.Status(ctx)
	if err != nil {
		return EnsureSystemAccountsResult{}, fmt.Errorf("reload setup status: %w", err)
	}

	return EnsureSystemAccountsResult{
		Accounts:    toAccounts(result.Accounts),
		SetupStatus: setupStatus,
	}, nil
}

func (s *AccountService) changeAccountStatus(ctx context.Context, input AccountLifecycleInput, status string, reason string, operation string, allowedCurrentStatuses ...string) (Account, error) {
	if input.OwnerUserID <= 0 {
		return Account{}, ValidationError{Message: "owner user is required"}
	}
	if input.AccountID <= 0 {
		return Account{}, ValidationError{Message: "account id is required"}
	}

	current, err := s.Account(ctx, input.AccountID)
	if err != nil {
		return Account{}, err
	}
	if current.IsSystem {
		return Account{}, ErrSystemAccountProtected
	}
	if !statusAllowed(current.Status, allowedCurrentStatuses) {
		return Account{}, ErrAccountInvalidLifecycle
	}

	now := s.now().UTC()
	effectiveInput := input.EffectiveFrom
	closedOn := current.ClosedOn
	if status == "closed" {
		fallbackClosedOn := current.ClosedOn
		if fallbackClosedOn == "" {
			fallbackClosedOn = effectiveInput
		}
		closedOn, err = cleanAccountLifecycleDate(input.ClosedOn, fallbackClosedOn, now)
		if err != nil {
			return Account{}, err
		}
		if closedOn < current.OpenedOn {
			return Account{}, ValidationError{Message: "account closed date must not be before opened date"}
		}
		if current.Status != "archived" {
			if strings.TrimSpace(effectiveInput) != "" && strings.TrimSpace(effectiveInput) != closedOn {
				return Account{}, ValidationError{Message: "account close effective date must match closed date"}
			}
			effectiveInput = closedOn
		}
	} else if status == "active" {
		closedOn = ""
	}
	effectiveFrom, err := cleanEffectiveFrom(effectiveInput, now)
	if err != nil {
		return Account{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, reason)
	if err != nil {
		return Account{}, err
	}
	record, err := s.repository.UpdateAccount(ctx, db.UpdateAccountParams{
		BookID:          BookID,
		AccountID:       input.AccountID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       operation,
		Spec:            accountSpecWithLifecycle(accountSpecFromStored(current), current.OpenedOn, closedOn),
		Status:          status,
		ChangeReason:    changeReason,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, fmt.Errorf("persist account status: %w", err)
	}

	return toAccount(record), nil
}

func statusAllowed(status string, allowed []string) bool {
	for _, allowedStatus := range allowed {
		if status == allowedStatus {
			return true
		}
	}
	return false
}

type accountSpecInput struct {
	AccountID             int64
	Code                  string
	Name                  string
	AccountClass          string
	AccountKind           string
	ParentAccountID       *int64
	InstitutionID         *int64
	CountryCode           string
	DefaultCommodityID    *int64
	DefaultCommodityCode  string
	QuantityScaleOverride *int
	AllowsPostings        *bool
	NumberLast4           string
	ExternalRefHint       string
	CommentMarkdown       string
	MetadataJSON          string
}

func (s *AccountService) accountSpec(ctx context.Context, input accountSpecInput) (db.AccountSpec, error) {
	code := strings.TrimSpace(input.Code)
	if len(code) > accountCodeMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("account code must be at most %d bytes", accountCodeMaxBytes)}
	}
	if hasControlCharacter(code) {
		return db.AccountSpec{}, ValidationError{Message: "account code must not contain control characters"}
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return db.AccountSpec{}, ValidationError{Message: "account name is required"}
	}
	if len(name) > accountNameMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("account name must be at most %d bytes", accountNameMaxBytes)}
	}
	if hasControlCharacter(name) {
		return db.AccountSpec{}, ValidationError{Message: "account name must not contain control characters"}
	}

	accountClass := strings.TrimSpace(input.AccountClass)
	if !accountClasses[accountClass] {
		return db.AccountSpec{}, ValidationError{Message: "account class is invalid"}
	}
	if accountClass == "equity" {
		return db.AccountSpec{}, ValidationError{Message: "equity accounts are managed by system workflows"}
	}

	accountKind := strings.TrimSpace(input.AccountKind)
	if accountKind == "" {
		accountKind = defaultAccountKind(accountClass)
	}
	kind, err := s.repository.AccountKindByCode(ctx, accountKind)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return db.AccountSpec{}, ValidationError{Message: "account kind is invalid"}
		}
		return db.AccountSpec{}, fmt.Errorf("read account kind: %w", err)
	}
	if kind.AccountClass != accountClass || !kind.IsUserAssignable {
		return db.AccountSpec{}, ValidationError{Message: "account kind is invalid for account class"}
	}

	allowsPostings := true
	if input.AllowsPostings != nil {
		allowsPostings = *input.AllowsPostings
	}

	parentAccountID := nullableInt64(input.ParentAccountID)
	parentInstitutionID := sql.NullInt64{}
	parentCountryCode := ""
	if parentAccountID.Valid {
		if input.AccountID > 0 && parentAccountID.Int64 == input.AccountID {
			return db.AccountSpec{}, ErrAccountParentInvalid
		}
		parent, err := s.Account(ctx, parentAccountID.Int64)
		if err != nil {
			return db.AccountSpec{}, ErrAccountParentInvalid
		}
		if parent.Status == "archived" {
			return db.AccountSpec{}, ErrAccountParentInvalid
		}
		if parent.AccountClass != accountClass {
			return db.AccountSpec{}, ErrAccountParentInvalid
		}
		cycle, err := s.repository.WouldCreateCycle(ctx, BookID, input.AccountID, parentAccountID.Int64)
		if err != nil {
			return db.AccountSpec{}, fmt.Errorf("check account parent cycle: %w", err)
		}
		if cycle {
			return db.AccountSpec{}, ErrAccountParentInvalid
		}
		parentInstitutionID = nullableInt64(parent.InstitutionID)
		parentCountryCode = parent.CountryCode
	}

	institutionID := nullableInt64(input.InstitutionID)
	countryCode := strings.ToUpper(strings.TrimSpace(input.CountryCode))
	if parentAccountID.Valid {
		institutionID = parentInstitutionID
		if countryCode == "" {
			countryCode = parentCountryCode
		}
	} else if institutionID.Valid {
		institution, err := s.institutionRepository.CurrentInstitutionByID(ctx, BookID, institutionID.Int64)
		if err != nil {
			return db.AccountSpec{}, ErrAccountReferenceInvalid
		}
		if institution.Status == "archived" {
			return db.AccountSpec{}, ErrAccountReferenceInvalid
		}
		if countryCode == "" {
			countryCode = nullableString(institution.CountryCode)
		}
	}
	if countryCode != "" {
		if len(countryCode) != 2 {
			return db.AccountSpec{}, ValidationError{Message: "country code must be a 2-letter code"}
		}
		for _, r := range countryCode {
			if r < 'A' || r > 'Z' {
				return db.AccountSpec{}, ValidationError{Message: "country code must use letters A-Z"}
			}
		}
	}

	defaultCommodityID := nullableInt64(input.DefaultCommodityID)
	defaultCommodityCode := normalizeCurrencyCode(input.DefaultCommodityCode)
	var ensureDefaultCurrency *db.CurrencySpec
	if defaultCommodityID.Valid && defaultCommodityCode != "" {
		return db.AccountSpec{}, ValidationError{Message: "use either default commodity id or default commodity code"}
	}
	var commodityMaxScale int
	if defaultCommodityID.Valid {
		commodity, err := s.repository.CurrentCommodityByID(ctx, BookID, defaultCommodityID.Int64)
		if err != nil {
			return db.AccountSpec{}, ErrAccountReferenceInvalid
		}
		if commodity.Status != "active" {
			return db.AccountSpec{}, ErrAccountReferenceInvalid
		}
		commodityMaxScale = commodity.MaxQuantityScale
	} else if defaultCommodityCode != "" {
		spec, err := currencySpecFromCatalog(defaultCommodityCode, "")
		if err != nil {
			if errors.Is(err, ErrCurrencyNotFound) {
				return db.AccountSpec{}, ErrAccountReferenceInvalid
			}
			return db.AccountSpec{}, err
		}
		ensureDefaultCurrency = &spec
		commodityMaxScale = spec.MaxQuantityScale
		defaultCommodityID = sql.NullInt64{Int64: -1, Valid: true}
	}
	if allowsPostings && defaultCommodityID.Valid == false && accountKindRequiresDefaultCommodity(accountClass, accountKind) {
		return db.AccountSpec{}, ValidationError{Message: "default commodity is required for this posting account"}
	}

	quantityScaleOverride := nullableInt(input.QuantityScaleOverride)
	if quantityScaleOverride.Valid {
		if !defaultCommodityID.Valid {
			return db.AccountSpec{}, ValidationError{Message: "quantity scale override requires a default commodity"}
		}
		if quantityScaleOverride.Int64 > int64(commodityMaxScale) {
			return db.AccountSpec{}, ValidationError{Message: "quantity scale override exceeds commodity maximum scale"}
		}
	}

	numberLast4 := strings.TrimSpace(input.NumberLast4)
	if len(numberLast4) > accountNumberLast4MaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("account number hint must be at most %d bytes", accountNumberLast4MaxBytes)}
	}
	if hasControlCharacter(numberLast4) {
		return db.AccountSpec{}, ValidationError{Message: "account number hint must not contain control characters"}
	}

	externalRefHint := strings.TrimSpace(input.ExternalRefHint)
	if len(externalRefHint) > accountExternalRefMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("account external reference hint must be at most %d bytes", accountExternalRefMaxBytes)}
	}
	if hasControlCharacter(externalRefHint) {
		return db.AccountSpec{}, ValidationError{Message: "account external reference hint must not contain control characters"}
	}

	commentMarkdown := strings.TrimSpace(input.CommentMarkdown)
	if len(commentMarkdown) > accountCommentMaxBytes {
		return db.AccountSpec{}, ValidationError{Message: fmt.Sprintf("account comment must be at most %d bytes", accountCommentMaxBytes)}
	}

	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "account metadata", accountJSONMaxBytes)
	if err != nil {
		return db.AccountSpec{}, err
	}

	return db.AccountSpec{
		Code:                  code,
		Name:                  name,
		AccountClass:          accountClass,
		AccountKind:           accountKind,
		ParentAccountID:       parentAccountID,
		InstitutionID:         institutionID,
		CountryCode:           countryCode,
		DefaultCommodityID:    defaultCommodityID,
		EnsureDefaultCurrency: ensureDefaultCurrency,
		QuantityScaleOverride: quantityScaleOverride,
		AllowsPostings:        allowsPostings,
		NumberLast4:           numberLast4,
		ExternalRefHint:       externalRefHint,
		CommentMarkdown:       commentMarkdown,
		MetadataJSON:          metadataJSON,
	}, nil
}

func defaultAccountKind(accountClass string) string {
	switch accountClass {
	case "asset":
		return "other_asset"
	case "liability":
		return "other_liability"
	case "equity":
		return "equity"
	case "income":
		return "income"
	default:
		return "expense"
	}
}

func accountKindRequiresDefaultCommodity(accountClass string, accountKind string) bool {
	if accountClass != "asset" && accountClass != "liability" {
		return false
	}

	return accountKind != "receivable" && accountKind != "payable"
}

func (s *AccountService) defaultCommodityChanged(ctx context.Context, current Account, spec db.AccountSpec, defaultCommodityCode string) (bool, error) {
	if strings.TrimSpace(defaultCommodityCode) != "" {
		record, err := s.repository.CurrentCurrencyByCode(ctx, BookID, normalizeCurrencyCode(defaultCommodityCode))
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return true, nil
			}
			return false, fmt.Errorf("read requested account currency: %w", err)
		}

		return current.DefaultCommodityID == nil || *current.DefaultCommodityID != record.ID, nil
	}

	if !spec.DefaultCommodityID.Valid {
		return current.DefaultCommodityID != nil, nil
	}
	if current.DefaultCommodityID == nil {
		return true, nil
	}

	return *current.DefaultCommodityID != spec.DefaultCommodityID.Int64, nil
}

func accountSpecFromStored(account Account) db.AccountSpec {
	return db.AccountSpec{
		Code:                  account.Code,
		Name:                  account.Name,
		AccountClass:          account.AccountClass,
		AccountKind:           account.AccountKind,
		ParentAccountID:       nullableInt64(account.ParentAccountID),
		InstitutionID:         nullableInt64(account.InstitutionID),
		CountryCode:           account.CountryCode,
		DefaultCommodityID:    nullableInt64(account.DefaultCommodityID),
		QuantityScaleOverride: nullableInt(account.QuantityScaleOverride),
		AllowsPostings:        account.AllowsPostings,
		NumberLast4:           account.NumberLast4,
		ExternalRefHint:       account.ExternalRefHint,
		CommentMarkdown:       account.CommentMarkdown,
		MetadataJSON:          account.MetadataJSON,
		OpenedOn:              account.OpenedOn,
		ClosedOn:              nullableSQLString(account.ClosedOn),
	}
}

func accountSpecWithLifecycle(spec db.AccountSpec, openedOn string, closedOn string) db.AccountSpec {
	spec.OpenedOn = openedOn
	spec.ClosedOn = nullableSQLString(closedOn)
	return spec
}

func cleanAccountOpenedOn(value string, fallback string, now time.Time) (string, error) {
	return cleanOptionalAccountDate(value, fallback, now, "account opened date")
}

func cleanAccountLifecycleDate(value string, fallback string, now time.Time) (string, error) {
	return cleanOptionalAccountDate(value, fallback, now, "account closed date")
}

func cleanOptionalAccountDate(value string, fallback string, now time.Time, field string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		cleaned = strings.TrimSpace(fallback)
	}
	if cleaned == "" {
		cleaned = now.UTC().Format(time.DateOnly)
	}

	parsed, err := time.Parse(time.DateOnly, cleaned)
	if err != nil {
		return "", ValidationError{Message: field + " must use YYYY-MM-DD"}
	}

	today, _ := time.Parse(time.DateOnly, now.UTC().Format(time.DateOnly))
	if parsed.After(today) {
		return "", ValidationError{Message: field + " must not be in the future"}
	}

	return cleaned, nil
}

func hasControlCharacter(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func nullableInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullableInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func nullableSQLString(value string) sql.NullString {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: cleaned, Valid: true}
}

func int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func intPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func toAccount(record db.AccountRecord) Account {
	return Account{
		ID:                    record.ID,
		BookID:                record.BookID,
		IsSystem:              record.IsSystem,
		SystemRole:            nullableString(record.SystemRole),
		Status:                record.Status,
		Code:                  nullableString(record.Code),
		Name:                  nullableString(record.Name),
		AccountClass:          record.AccountClass,
		AccountKind:           record.AccountKind,
		ParentAccountID:       int64Ptr(record.ParentAccountID),
		InstitutionID:         int64Ptr(record.InstitutionID),
		CountryCode:           nullableString(record.CountryCode),
		DefaultCommodityID:    int64Ptr(record.DefaultCommodityID),
		QuantityScaleOverride: intPtr(record.QuantityScaleOverride),
		AllowsPostings:        record.AllowsPostings,
		NumberLast4:           nullableString(record.NumberLast4),
		ExternalRefHint:       nullableString(record.ExternalRefHint),
		CommentMarkdown:       record.CommentMarkdown,
		MetadataJSON:          record.MetadataJSON,
		OpenedOn:              record.OpenedOn,
		ClosedOn:              nullableString(record.ClosedOn),
		EffectiveFrom:         record.EffectiveFrom,
		ChangeReason:          record.ChangeReason,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.RecordedAt,
	}
}

func toAccounts(records []db.AccountRecord) []Account {
	accounts := make([]Account, 0, len(records))
	for _, record := range records {
		accounts = append(accounts, toAccount(record))
	}

	return accounts
}
