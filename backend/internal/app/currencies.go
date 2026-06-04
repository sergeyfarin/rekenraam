package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
)

const (
	// BookID is the current single-book runtime boundary; multi-book support must
	// replace this deliberately rather than spreading selector assumptions.
	BookID = int64(1)

	currencyCodeLength   = 3
	currencyNameMaxBytes = 200
)

var (
	ErrCurrencyNotFound       = errors.New("currency not found")
	ErrCurrencyAlreadyExists  = errors.New("currency already exists")
	ErrCurrenciesAlreadySetup = errors.New("currencies setup already complete")
)

type CurrencyCatalogEntry struct {
	Code             string
	Symbol           string
	EnglishName      string
	StandardScale    int
	MaxQuantityScale int
}

type Currency struct {
	ID               int64
	BookID           int64
	Code             string
	Name             string
	Symbol           string
	DisplaySymbol    string
	Status           string
	StandardScale    int
	MaxQuantityScale int
	IsBuiltin        bool
	CreatedAt        string
	UpdatedAt        string
}

type CurrencySelectionInput struct {
	Code string
	Name string
}

type CreateCurrencyInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	Code          string
	Name          string
}

type CompleteCurrencySetupInput struct {
	OwnerUserID         int64
	AuthSessionID       int64
	RequestID           string
	DefaultCurrencyCode string
	Currencies          []CurrencySelectionInput
}

type CompleteCurrencySetupResult struct {
	Currencies      []Currency
	DefaultCurrency Currency
	SetupStatus     SetupStatus
}

type SetDefaultCurrencyInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	CommodityID   int64
}

type SetDefaultCurrencyResult struct {
	DefaultCurrency Currency
}

type CurrencyService struct {
	repository   *db.CommodityRepository
	setupService *SetupService
	now          func() time.Time
	catalog      map[string]CurrencyCatalogEntry
	catalogList  []CurrencyCatalogEntry
}

func NewCurrencyService(repository *db.CommodityRepository, setupService *SetupService) *CurrencyService {
	catalog := make(map[string]CurrencyCatalogEntry, len(currencyCatalog))
	catalogList := append([]CurrencyCatalogEntry(nil), currencyCatalog...)
	sort.Slice(catalogList, func(i, j int) bool {
		return catalogList[i].Code < catalogList[j].Code
	})
	for _, entry := range catalogList {
		catalog[entry.Code] = entry
	}

	return &CurrencyService{
		repository:   repository,
		setupService: setupService,
		now:          time.Now,
		catalog:      catalog,
		catalogList:  catalogList,
	}
}

func (s *CurrencyService) Catalog() []CurrencyCatalogEntry {
	return append([]CurrencyCatalogEntry(nil), s.catalogList...)
}

func (s *CurrencyService) ListCurrencies(ctx context.Context) ([]Currency, error) {
	records, err := s.repository.ListCurrencies(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}

	return toCurrencies(records), nil
}

func (s *CurrencyService) CreateCurrency(ctx context.Context, input CreateCurrencyInput) (Currency, error) {
	spec, err := s.currencySpec(input.Code, input.Name)
	if err != nil {
		return Currency{}, err
	}
	if input.OwnerUserID <= 0 {
		return Currency{}, ValidationError{Message: "owner user is required"}
	}

	now := s.now().UTC()
	record, err := s.repository.CreateCurrency(ctx, db.CreateCurrencyParams{
		BookID:          BookID,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		Spec:            spec,
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   now.Format(time.DateOnly),
	})
	if err != nil {
		if errors.Is(err, db.ErrCommodityExists) {
			return Currency{}, ErrCurrencyAlreadyExists
		}
		return Currency{}, fmt.Errorf("persist currency: %w", err)
	}

	return toCurrency(record), nil
}

func (s *CurrencyService) CompleteCurrencySetup(ctx context.Context, input CompleteCurrencySetupInput) (CompleteCurrencySetupResult, error) {
	if input.OwnerUserID <= 0 {
		return CompleteCurrencySetupResult{}, ValidationError{Message: "owner user is required"}
	}

	defaultCode := normalizeCurrencyCode(input.DefaultCurrencyCode)
	if defaultCode == "" {
		return CompleteCurrencySetupResult{}, ValidationError{Message: "default currency is required"}
	}

	selectionByCode := make(map[string]CurrencySelectionInput, len(input.Currencies)+1)
	for _, selection := range input.Currencies {
		code := normalizeCurrencyCode(selection.Code)
		if code == "" {
			return CompleteCurrencySetupResult{}, ValidationError{Message: "currency code is required"}
		}
		selection.Code = code
		selectionByCode[code] = selection
	}
	if _, ok := selectionByCode[defaultCode]; !ok {
		selectionByCode[defaultCode] = CurrencySelectionInput{Code: defaultCode}
	}

	specs := make([]db.CurrencySpec, 0, len(selectionByCode))
	for _, selection := range selectionByCode {
		spec, err := s.currencySpec(selection.Code, selection.Name)
		if err != nil {
			return CompleteCurrencySetupResult{}, err
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Code < specs[j].Code
	})

	now := s.now().UTC()
	result, err := s.repository.CompleteCurrencySetup(ctx, db.CompleteCurrencySetupParams{
		BookID:          BookID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		DefaultCode:     defaultCode,
		Specs:           specs,
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   now.Format(time.DateOnly),
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrCurrenciesSetupComplete):
			return CompleteCurrencySetupResult{}, ErrCurrenciesAlreadySetup
		case errors.Is(err, db.ErrNotFound):
			return CompleteCurrencySetupResult{}, ErrCurrencyNotFound
		default:
			return CompleteCurrencySetupResult{}, fmt.Errorf("persist currencies setup: %w", err)
		}
	}

	setupStatus, err := s.setupService.Status(ctx)
	if err != nil {
		return CompleteCurrencySetupResult{}, fmt.Errorf("reload setup status: %w", err)
	}

	return CompleteCurrencySetupResult{
		Currencies:      toCurrencies(result.Currencies),
		DefaultCurrency: toCurrency(result.DefaultCurrency),
		SetupStatus:     setupStatus,
	}, nil
}

func (s *CurrencyService) SetDefaultCurrency(ctx context.Context, input SetDefaultCurrencyInput) (SetDefaultCurrencyResult, error) {
	if input.OwnerUserID <= 0 {
		return SetDefaultCurrencyResult{}, ValidationError{Message: "owner user is required"}
	}
	if input.CommodityID <= 0 {
		return SetDefaultCurrencyResult{}, ValidationError{Message: "currency id is required"}
	}

	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.SetDefaultCurrency(ctx, db.SetDefaultCurrencyParams{
		BookID:          BookID,
		CommodityID:     input.CommodityID,
		UpdatedAt:       now,
		UpdatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return SetDefaultCurrencyResult{}, ErrCurrencyNotFound
		}
		return SetDefaultCurrencyResult{}, fmt.Errorf("set default currency: %w", err)
	}

	return SetDefaultCurrencyResult{DefaultCurrency: toCurrency(record)}, nil
}

func (s *CurrencyService) currencySpec(code string, name string) (db.CurrencySpec, error) {
	code = normalizeCurrencyCode(code)
	if len(code) != currencyCodeLength {
		return db.CurrencySpec{}, ValidationError{Message: "currency code must be a 3-letter code"}
	}

	entry, ok := s.catalog[code]
	if !ok {
		return db.CurrencySpec{}, ErrCurrencyNotFound
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = entry.EnglishName
	}
	if len(name) > currencyNameMaxBytes {
		return db.CurrencySpec{}, ValidationError{Message: fmt.Sprintf("currency name must be at most %d bytes", currencyNameMaxBytes)}
	}

	return db.CurrencySpec{
		Code:             entry.Code,
		Name:             name,
		Symbol:           entry.Symbol,
		StandardScale:    entry.StandardScale,
		MaxQuantityScale: entry.MaxQuantityScale,
	}, nil
}

func normalizeCurrencyCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func toCurrency(record db.CommodityRecord) Currency {
	return Currency{
		ID:               record.ID,
		BookID:           record.BookID,
		Code:             record.Code,
		Name:             record.Name,
		Symbol:           record.Symbol,
		DisplaySymbol:    record.DisplaySymbol,
		Status:           record.Status,
		StandardScale:    record.StandardScale,
		MaxQuantityScale: record.MaxQuantityScale,
		IsBuiltin:        record.IsBuiltin,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.RecordedAt,
	}
}

func toCurrencies(records []db.CommodityRecord) []Currency {
	currencies := make([]Currency, 0, len(records))
	for _, record := range records {
		currencies = append(currencies, toCurrency(record))
	}
	return currencies
}
