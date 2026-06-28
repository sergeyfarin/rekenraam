package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

const (
	investmentTextMaxBytes = 500
	investmentJSONMaxBytes = 10000
	investmentSearchLimit  = 25
)

var (
	ErrInvestmentInstrumentNotFound = errors.New("investment instrument not found")
	ErrInvestmentInstrumentExists   = errors.New("investment instrument already exists")
	ErrCostBasisProfileNotFound     = errors.New("cost basis profile not found")
	ErrCostBasisProfileExists       = errors.New("cost basis profile already exists")
	ErrDividendDefaultNotFound      = errors.New("dividend default not found")
	ErrInvestmentLotsInsufficient   = errors.New("insufficient investment lots")
	ErrInvestmentSuggestionNotFound = errors.New("investment event suggestion not found")
	ErrAutomationRuleNotFound       = errors.New("investment automation rule not found")
)

type InstrumentSearchProvider interface {
	SearchInstruments(ctx context.Context, query string) ([]InstrumentSearchCandidate, error)
}

type PriceProvider interface {
	FetchPrices(ctx context.Context, request PriceProviderRequest) ([]PriceProviderObservation, error)
}

type DividendProvider interface {
	FetchDividends(ctx context.Context, request CorporateEventProviderRequest) ([]ProviderInvestmentEvent, error)
}

type CorporateActionProvider interface {
	FetchCorporateActions(ctx context.Context, request CorporateEventProviderRequest) ([]ProviderInvestmentEvent, error)
}

type InstrumentSearchCandidate struct {
	SourceCode           string
	ProviderInstrumentID string
	Symbol               string
	DisplayName          string
	InstrumentType       string
	ExchangeCode         string
	MIC                  string
	CountryCode          string
	QuoteCode            string
	ConfidenceBPS        int
	RawJSON              string
}

type PriceProviderRequest struct {
	InstrumentID     int64
	CommodityID      int64
	QuoteCommodityID int64
	AfterDate        string
	BeforeDate       string
}

type PriceProviderObservation struct {
	ProviderObservationID string
	PriceValue            int64
	PriceScale            int
	PriceDate             string
	RawJSON               string
}

type CorporateEventProviderRequest struct {
	InstrumentID int64
	AfterDate    string
	BeforeDate   string
}

type ProviderInvestmentEvent struct {
	ProviderEventID string
	EventFamily     string
	EventDate       string
	NormalizedJSON  string
	RawJSON         string
}

type InvestmentInstrument struct {
	ID                 int64
	BookID             int64
	CommodityID        int64
	CommodityCode      string
	InstrumentType     string
	DisplayName        string
	Symbol             string
	ExchangeCode       string
	MIC                string
	Issuer             string
	CountryCode        string
	QuoteCommodityID   *int64
	TradingCommodityID *int64
	QuantityScale      int
	PriceScale         int
	Status             string
	IdentifiersJSON    string
	MetadataJSON       string
	CreatedAt          string
	UpdatedAt          string
}

type InvestmentInstrumentInput struct {
	OwnerUserID        int64
	AuthSessionID      int64
	RequestID          string
	OriginType         string
	Operation          string
	InstrumentID       int64
	CommodityID        *int64
	CommodityCode      string
	InstrumentType     string
	DisplayName        string
	Symbol             string
	ExchangeCode       string
	MIC                string
	Issuer             string
	CountryCode        string
	QuoteCommodityID   *int64
	TradingCommodityID *int64
	QuantityScale      int
	PriceScale         int
	IdentifiersJSON    string
	MetadataJSON       string
	EffectiveFrom      string
	ChangeReason       string
}

type HoldingAccountInput struct {
	OwnerUserID     int64
	AuthSessionID   int64
	RequestID       string
	InstrumentID    int64
	Name            string
	ParentAccountID *int64
	InstitutionID   *int64
	OpenedOn        string
	EffectiveFrom   string
	QuantityScale   *int
	ChangeReason    string
	CostBasisMethod string
}

type CostBasisProfile struct {
	ID           int64
	BookID       int64
	Name         string
	Method       string
	IsDefault    bool
	Status       string
	Description  string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
}

type CostBasisProfileInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	ProfileID     int64
	Name          string
	Method        string
	IsDefault     bool
	Status        string
	Description   string
	MetadataJSON  string
	ChangeReason  string
}

type DividendDefault struct {
	ID                      int64
	BookID                  int64
	CommodityID             *int64
	IncomeAccountID         int64
	WithholdingAccountID    *int64
	DefaultWithholdingValue *int64
	DefaultWithholdingScale *int
	WithholdingRateBPS      *int64
	TaxCountryCode          string
	TaxTreatment            string
	Status                  string
	EffectiveFrom           string
	EffectiveTo             string
	MetadataJSON            string
	CreatedAt               string
	UpdatedAt               string
}

type DividendDefaultInput struct {
	OwnerUserID             int64
	AuthSessionID           int64
	RequestID               string
	DefaultID               int64
	CommodityID             *int64
	IncomeAccountID         int64
	WithholdingAccountID    *int64
	DefaultWithholdingValue *int64
	DefaultWithholdingScale *int
	WithholdingRateBPS      *int64
	TaxCountryCode          string
	TaxTreatment            string
	Status                  string
	EffectiveFrom           string
	EffectiveTo             string
	MetadataJSON            string
	ChangeReason            string
}

type InvestmentTradeInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	TransactionDate  string
	CommodityID      int64
	HoldingAccountID int64
	CashAccountID    int64
	QuantityValue    exact.Coefficient
	QuantityScale    int
	CashAmountValue  int64
	CashAmountScale  int
	CashCommodityID  int64
	Memo             string
	PayeeID          *int64
	Status           string
	LotAllocations   []InvestmentLotAllocationInput
	ChangeReason     string
	CostBasisMethod  string
}

type SellPreviewResult struct {
	CostBasisMethod string
	Allocations     []InvestmentLotDisposal
	RealizedGain    int64
	CashAmountValue int64
	CashAmountScale int
}

type InvestmentLotAllocationInput struct {
	LotID         int64
	QuantityValue exact.Coefficient
	QuantityScale int
}

type InvestmentTradeResult struct {
	Transaction Transaction
	LotID       *int64
	Allocations []InvestmentLotDisposal
}

type InvestmentLotDisposal struct {
	LotID          int64
	QuantityValue  exact.Coefficient
	QuantityScale  int
	CostBasisValue int64
	CostBasisScale int
}

type DividendInput struct {
	OwnerUserID          int64
	AuthSessionID        int64
	RequestID            string
	TransactionDate      string
	CommodityID          *int64
	CashAccountID        int64
	CashCommodityID      int64
	IncomeAccountID      *int64
	AmountValue          int64
	AmountScale          int
	WithholdingValue     *int64
	WithholdingScale     *int
	WithholdingAccountID *int64
	Memo                 string
	PayeeID              *int64
	Status               string
	ChangeReason         string
}

type ReinvestedDividendInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	TransactionDate  string
	CommodityID      int64
	HoldingAccountID int64
	IncomeAccountID  *int64
	QuantityValue    exact.Coefficient
	QuantityScale    int
	AmountValue      int64
	AmountScale      int
	CashCommodityID  int64
	Memo             string
	PayeeID          *int64
	Status           string
	ChangeReason     string
}

type InvestmentLot struct {
	ID                      int64
	BookID                  int64
	AccountID               int64
	CommodityID             int64
	OpenedOn                string
	SourceTransactionID     *int64
	Status                  string
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingQuantityValue  exact.Coefficient
	RemainingQuantityScale  int
	CostBasisValue          int64
	CostBasisScale          int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	CostCommodityID         int64
	MetadataJSON            string
	CreatedAt               string
	UpdatedAt               string
}

type InvestmentPosition struct {
	AccountID               int64
	CommodityID             int64
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	CostCommodityID         int64
	LatestPriceValue        *int64
	LatestPriceScale        *int
	LatestPriceDate         string
}

type InvestmentProviderEvent struct {
	ID              int64
	BookID          int64
	SourceID        int64
	ProviderEventID string
	InstrumentID    *int64
	EventFamily     string
	EventDate       string
	Status          string
	NormalizedJSON  string
	RawJSON         string
	CreatedAt       string
}

type InvestmentEventSuggestion struct {
	ID                      int64
	BookID                  int64
	ProviderEventID         int64
	InstrumentID            *int64
	ConfidenceBPS           int
	Status                  string
	ProposedTransactionJSON string
	GeneratedTransactionID  *int64
	FailureReason           string
	CreatedAt               string
	UpdatedAt               string
}

type InvestmentAutomationRule struct {
	ID                     int64
	BookID                 int64
	SourceID               *int64
	InstrumentID           *int64
	EventFamily            string
	Mode                   string
	ConfidenceThresholdBPS int
	RequiredAccountsJSON   string
	Status                 string
	EffectiveFrom          string
	EffectiveTo            string
	CreatedAt              string
	UpdatedAt              string
}

type InvestmentAutomationRuleInput struct {
	RuleID                 int64
	SourceID               *int64
	InstrumentID           *int64
	EventFamily            string
	Mode                   string
	ConfidenceThresholdBPS int
	RequiredAccountsJSON   string
	Status                 string
	EffectiveFrom          string
	EffectiveTo            string
}

type SaveAutomationRulesInput struct {
	OwnerUserID   int64
	AuthSessionID int64
	RequestID     string
	ChangeReason  string
	Rules         []InvestmentAutomationRuleInput
}

type InvestmentService struct {
	repository         *db.InvestmentRepository
	accountService     *AccountService
	transactionService *TransactionService
	pricingService     *PricingService
	now                func() time.Time
}

func NewInvestmentService(repository *db.InvestmentRepository, accountService *AccountService, transactionService *TransactionService, pricingService *PricingService) *InvestmentService {
	return &InvestmentService{
		repository:         repository,
		accountService:     accountService,
		transactionService: transactionService,
		pricingService:     pricingService,
		now:                time.Now,
	}
}

func (s *InvestmentService) SetNowForTest(now func() time.Time) {
	s.now = now
}

func (s *InvestmentService) Search(ctx context.Context, query string) ([]InvestmentInstrument, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []InvestmentInstrument{}, nil
	}
	records, err := s.repository.SearchInstruments(ctx, BookID, query, investmentSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("search investment instruments: %w", err)
	}
	return toInvestmentInstruments(records), nil
}

func (s *InvestmentService) ListInstruments(ctx context.Context) ([]InvestmentInstrument, error) {
	records, err := s.repository.ListInstruments(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list investment instruments: %w", err)
	}
	return toInvestmentInstruments(records), nil
}

func (s *InvestmentService) Instrument(ctx context.Context, instrumentID int64) (InvestmentInstrument, error) {
	if instrumentID <= 0 {
		return InvestmentInstrument{}, ValidationError{Message: "instrument id is required"}
	}
	record, err := s.repository.InstrumentByID(ctx, BookID, instrumentID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentInstrument{}, ErrInvestmentInstrumentNotFound
		}
		return InvestmentInstrument{}, fmt.Errorf("read investment instrument: %w", err)
	}
	return toInvestmentInstrument(record), nil
}

func (s *InvestmentService) CreateInstrument(ctx context.Context, input InvestmentInstrumentInput) (InvestmentInstrument, error) {
	if input.OwnerUserID <= 0 {
		return InvestmentInstrument{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := cleanInvestmentInstrumentSpec(input)
	if err != nil {
		return InvestmentInstrument{}, err
	}
	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return InvestmentInstrument{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "created investment instrument")
	if err != nil {
		return InvestmentInstrument{}, err
	}
	record, err := s.repository.CreateInstrument(ctx, db.CreateInvestmentInstrumentParams{
		BookID:          BookID,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       defaultOperation(input.Operation, "investment.instrument.create"),
		Spec:            spec,
		CreatedAt:       now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
		ChangeReason:    changeReason,
	})
	if err != nil {
		if errors.Is(err, db.ErrInvestmentInstrumentExists) || errors.Is(err, db.ErrCommodityExists) {
			return InvestmentInstrument{}, ErrInvestmentInstrumentExists
		}
		return InvestmentInstrument{}, fmt.Errorf("persist investment instrument: %w", err)
	}
	return toInvestmentInstrument(record), nil
}

func (s *InvestmentService) UpdateInstrument(ctx context.Context, input InvestmentInstrumentInput) (InvestmentInstrument, error) {
	if input.OwnerUserID <= 0 {
		return InvestmentInstrument{}, ValidationError{Message: "owner user is required"}
	}
	if input.InstrumentID <= 0 {
		return InvestmentInstrument{}, ValidationError{Message: "instrument id is required"}
	}
	spec, err := cleanInvestmentInstrumentSpec(input)
	if err != nil {
		return InvestmentInstrument{}, err
	}
	now := s.now().UTC()
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return InvestmentInstrument{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "updated investment instrument")
	if err != nil {
		return InvestmentInstrument{}, err
	}
	record, err := s.repository.UpdateInstrument(ctx, db.UpdateInvestmentInstrumentParams{
		BookID:          BookID,
		InstrumentID:    input.InstrumentID,
		ChangedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      input.OriginType,
		Operation:       defaultOperation(input.Operation, "investment.instrument.update"),
		Spec:            spec,
		RecordedAt:      now.Format(time.RFC3339),
		EffectiveFrom:   effectiveFrom,
		ChangeReason:    changeReason,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentInstrument{}, ErrInvestmentInstrumentNotFound
		}
		return InvestmentInstrument{}, fmt.Errorf("persist investment instrument update: %w", err)
	}
	return toInvestmentInstrument(record), nil
}

func (s *InvestmentService) CreateHoldingAccount(ctx context.Context, input HoldingAccountInput) (Account, error) {
	instrument, err := s.Instrument(ctx, input.InstrumentID)
	if err != nil {
		return Account{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = instrument.DisplayName
	}
	scale := input.QuantityScale
	if scale == nil {
		scale = &instrument.QuantityScale
	}
	return s.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID:           input.OwnerUserID,
		AuthSessionID:         input.AuthSessionID,
		RequestID:             input.RequestID,
		OriginType:            "browser_api",
		Operation:             "investment.holding_account.create",
		Name:                  name,
		AccountClass:          "asset",
		AccountKind:           "security_holding",
		ParentAccountID:       input.ParentAccountID,
		InstitutionID:         input.InstitutionID,
		DefaultCommodityID:    &instrument.CommodityID,
		QuantityScaleOverride: scale,
		AllowsPostings:        boolPtr(true),
		OpenedOn:              input.OpenedOn,
		EffectiveFrom:         input.EffectiveFrom,
		ChangeReason:          input.ChangeReason,
		CostBasisMethod:       input.CostBasisMethod,
	})
}

func (s *InvestmentService) ListCostBasisProfiles(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string) ([]CostBasisProfile, error) {
	now := s.now().UTC().Format(time.RFC3339)
	if ownerUserID > 0 {
		if _, err := s.repository.EnsureDefaultCostBasisProfile(ctx, BookID, ownerUserID, authSessionID, requestID, now); err != nil {
			return nil, fmt.Errorf("ensure default cost basis profile: %w", err)
		}
	}
	records, err := s.repository.ListCostBasisProfiles(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list cost basis profiles: %w", err)
	}
	return toCostBasisProfiles(records), nil
}

func (s *InvestmentService) SaveCostBasisProfile(ctx context.Context, input CostBasisProfileInput) (CostBasisProfile, error) {
	if input.OwnerUserID <= 0 {
		return CostBasisProfile{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := cleanCostBasisProfileSpec(input)
	if err != nil {
		return CostBasisProfile{}, err
	}
	now := s.now().UTC()
	changeReason, err := cleanChangeReason(input.ChangeReason, "saved cost basis profile")
	if err != nil {
		return CostBasisProfile{}, err
	}
	record, err := s.repository.SaveCostBasisProfile(ctx, db.SaveCostBasisProfileParams{
		BookID:        BookID,
		ProfileID:     input.ProfileID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     profileOperation(input.ProfileID),
		Spec:          spec,
		RecordedAt:    now.Format(time.RFC3339),
		ChangeReason:  changeReason,
	})
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			return CostBasisProfile{}, ErrCostBasisProfileNotFound
		case errors.Is(err, db.ErrCostBasisProfileExists):
			return CostBasisProfile{}, ErrCostBasisProfileExists
		default:
			return CostBasisProfile{}, fmt.Errorf("save cost basis profile: %w", err)
		}
	}
	return toCostBasisProfile(record), nil
}

func (s *InvestmentService) ListDividendDefaults(ctx context.Context, commodityID int64) ([]DividendDefault, error) {
	records, err := s.repository.ListDividendDefaults(ctx, BookID, commodityID)
	if err != nil {
		return nil, fmt.Errorf("list dividend defaults: %w", err)
	}
	return toDividendDefaults(records), nil
}

func (s *InvestmentService) SaveDividendDefault(ctx context.Context, input DividendDefaultInput) (DividendDefault, error) {
	if input.OwnerUserID <= 0 {
		return DividendDefault{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := cleanDividendDefaultSpec(input, s.now().UTC())
	if err != nil {
		return DividendDefault{}, err
	}
	changeReason, err := cleanChangeReason(input.ChangeReason, "saved dividend default")
	if err != nil {
		return DividendDefault{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.SaveDividendDefault(ctx, db.SaveDividendDefaultParams{
		BookID:        BookID,
		DefaultID:     input.DefaultID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     dividendDefaultOperation(input.DefaultID),
		Spec:          spec,
		RecordedAt:    now,
		ChangeReason:  changeReason,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return DividendDefault{}, ErrDividendDefaultNotFound
		}
		return DividendDefault{}, fmt.Errorf("save dividend default: %w", err)
	}
	return toDividendDefault(record), nil
}

func (s *InvestmentService) Buy(ctx context.Context, input InvestmentTradeInput) (InvestmentTradeResult, error) {
	if err := validateTradeInput(input); err != nil {
		return InvestmentTradeResult{}, err
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentTradeResult{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return InvestmentTradeResult{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	transactionParams, err := s.transactionService.prepareCreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "investment.buy",
		ChangeReason:  input.ChangeReason,
		Spec: TransactionInput{
			Status:          status,
			TransactionKind: "investment",
			TransactionDate: input.TransactionDate,
			PayeeID:         input.PayeeID,
			Description:     memo,
			JournalEntries: []JournalEntryInput{{
				EntryDate: input.TransactionDate,
				EntryKind: "investment",
				Memo:      memo,
				Postings: []PostingInput{
					{AccountID: input.HoldingAccountID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
					{AccountID: tradingAccountID, QuantityValue: input.QuantityValue.Negated(), QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
					{AccountID: tradingAccountID, QuantityValue: exact.New(input.CashAmountValue), QuantityScale: input.CashAmountScale, CommodityID: input.CashCommodityID, Memo: memo},
					{AccountID: input.CashAccountID, QuantityValue: exact.New(-input.CashAmountValue), QuantityScale: input.CashAmountScale, CommodityID: input.CashCommodityID, Memo: memo},
				},
			}},
		},
	})
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	transactionRecord, lot, err := s.repository.CreateTransactionAndLot(ctx, transactionParams, db.CreateInvestmentLotParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		OpenedOn:        input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		CostBasisValue:  input.CashAmountValue,
		CostBasisScale:  input.CashAmountScale,
		CostCommodityID: input.CashCommodityID,
		MetadataJSON:    "{}",
		CreatedAt:       now,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.create",
		ChangeReason:    "created lot from buy transaction",
		EventKind:       "acquisition",
	})
	if err != nil {
		return InvestmentTradeResult{}, fmt.Errorf("create buy transaction and lot: %w", mapTransactionDBError(err))
	}
	transaction := toTransaction(transactionRecord)
	if s.pricingService != nil {
		_ = s.pricingService.CreateTradeImpliedPrice(ctx, CreateTradeImpliedPriceInput{
			OwnerUserID:      input.OwnerUserID,
			AuthSessionID:    input.AuthSessionID,
			RequestID:        input.RequestID,
			CommodityID:      input.CommodityID,
			QuoteCommodityID: input.CashCommodityID,
			PriceDate:        input.TransactionDate,
			QuantityValue:    input.QuantityValue,
			QuantityScale:    input.QuantityScale,
			CashValue:        input.CashAmountValue,
			CashScale:        input.CashAmountScale,
			PriceScale:       8,
		})
	}
	return InvestmentTradeResult{Transaction: transaction, LotID: &lot.ID}, nil
}

func (s *InvestmentService) resolveCostBasisMethod(ctx context.Context, holdingAccountID int64, txnOverride string) (string, error) {
	if txnOverride != "" {
		return txnOverride, nil
	}
	account, err := s.accountService.Account(ctx, holdingAccountID)
	if err == nil && account.CostBasisMethod != "" {
		return account.CostBasisMethod, nil
	}
	profile, err := s.repository.DefaultCostBasisProfile(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "fifo", nil
		}
		return "", fmt.Errorf("resolve cost basis method: %w", err)
	}
	return profile.Method, nil
}

func (s *InvestmentService) computeSellDisposals(ctx context.Context, input InvestmentTradeInput, method string) ([]db.LotDisposalRecord, error) {
	allocations := make([]db.LotAllocation, 0, len(input.LotAllocations))
	for _, a := range input.LotAllocations {
		allocations = append(allocations, db.LotAllocation{LotID: a.LotID, QuantityValue: a.QuantityValue, QuantityScale: a.QuantityScale})
	}
	return s.repository.SimulateDisposeLots(ctx, db.DisposeLotsParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		EventDate:       input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		Allocations:     allocations,
		CostBasisMethod: method,
	})
}

func (s *InvestmentService) PreviewSell(ctx context.Context, input InvestmentTradeInput) (SellPreviewResult, error) {
	if err := validateTradeInput(input); err != nil {
		return SellPreviewResult{}, err
	}
	method, err := s.resolveCostBasisMethod(ctx, input.HoldingAccountID, input.CostBasisMethod)
	if err != nil {
		return SellPreviewResult{}, err
	}
	disposals, err := s.computeSellDisposals(ctx, input, method)
	if err != nil {
		if errors.Is(err, db.ErrInsufficientLots) {
			return SellPreviewResult{}, ErrInvestmentLotsInsufficient
		}
		if errors.Is(err, db.ErrInvalidDisposalParams) {
			return SellPreviewResult{}, ValidationError{Message: err.Error()}
		}
		return SellPreviewResult{}, fmt.Errorf("preview sell disposals: %w", err)
	}
	var totalDisposedBasis int64
	for _, d := range disposals {
		totalDisposedBasis += d.CostBasisValue
	}
	realizedGain := input.CashAmountValue - totalDisposedBasis
	return SellPreviewResult{
		CostBasisMethod: method,
		Allocations:     toInvestmentLotDisposals(disposals),
		RealizedGain:    realizedGain,
		CashAmountValue: input.CashAmountValue,
		CashAmountScale: input.CashAmountScale,
	}, nil
}

func (s *InvestmentService) Sell(ctx context.Context, input InvestmentTradeInput) (InvestmentTradeResult, error) {
	if err := validateTradeInput(input); err != nil {
		return InvestmentTradeResult{}, err
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentTradeResult{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return InvestmentTradeResult{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	transactionParams, err := s.transactionService.prepareCreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "investment.sell",
		ChangeReason:  input.ChangeReason,
		Spec: TransactionInput{
			Status:          status,
			TransactionKind: "investment",
			TransactionDate: input.TransactionDate,
			PayeeID:         input.PayeeID,
			Description:     memo,
			JournalEntries: []JournalEntryInput{{
				EntryDate: input.TransactionDate,
				EntryKind: "investment",
				Memo:      memo,
				Postings: []PostingInput{
					{AccountID: input.HoldingAccountID, QuantityValue: input.QuantityValue.Negated(), QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
					{AccountID: tradingAccountID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
					{AccountID: input.CashAccountID, QuantityValue: exact.New(input.CashAmountValue), QuantityScale: input.CashAmountScale, CommodityID: input.CashCommodityID, Memo: memo},
					{AccountID: tradingAccountID, QuantityValue: exact.New(-input.CashAmountValue), QuantityScale: input.CashAmountScale, CommodityID: input.CashCommodityID, Memo: memo},
				},
			}},
		},
	})
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	method, err := s.resolveCostBasisMethod(ctx, input.HoldingAccountID, input.CostBasisMethod)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	allocations := make([]db.LotAllocation, 0, len(input.LotAllocations))
	for _, allocation := range input.LotAllocations {
		allocations = append(allocations, db.LotAllocation{LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	transactionRecord, disposals, err := s.repository.CreateTransactionAndDisposeLots(ctx, transactionParams, db.DisposeLotsParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		EventDate:       input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		Allocations:     allocations,
		CostBasisMethod: method,
		CreatedAt:       s.now().UTC().Format(time.RFC3339),
		ActorUserID:     input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.dispose",
		ChangeReason:    "disposed lots from sell transaction",
	})
	if err != nil {
		if errors.Is(err, db.ErrInsufficientLots) {
			return InvestmentTradeResult{}, ErrInvestmentLotsInsufficient
		}
		if errors.Is(err, db.ErrInvalidDisposalParams) {
			return InvestmentTradeResult{}, ValidationError{Message: err.Error()}
		}
		return InvestmentTradeResult{}, fmt.Errorf("dispose sell lots: %w", err)
	}
	transaction := toTransaction(transactionRecord)
	if s.pricingService != nil {
		_ = s.pricingService.CreateTradeImpliedPrice(ctx, CreateTradeImpliedPriceInput{
			OwnerUserID:      input.OwnerUserID,
			AuthSessionID:    input.AuthSessionID,
			RequestID:        input.RequestID,
			CommodityID:      input.CommodityID,
			QuoteCommodityID: input.CashCommodityID,
			PriceDate:        input.TransactionDate,
			QuantityValue:    input.QuantityValue,
			QuantityScale:    input.QuantityScale,
			CashValue:        input.CashAmountValue,
			CashScale:        input.CashAmountScale,
			PriceScale:       8,
		})
	}
	return InvestmentTradeResult{Transaction: transaction, Allocations: toInvestmentLotDisposals(disposals)}, nil
}

func (s *InvestmentService) Dividend(ctx context.Context, input DividendInput) (Transaction, error) {
	date, err := validateDividendInput(input)
	if err != nil {
		return Transaction{}, err
	}
	if input.CommodityID != nil && (input.IncomeAccountID == nil || input.WithholdingAccountID == nil) {
		defaultRecord, err := s.repository.ResolveDividendDefault(ctx, BookID, *input.CommodityID, date)
		if err == nil {
			if input.IncomeAccountID == nil {
				input.IncomeAccountID = &defaultRecord.IncomeAccountID
			}
			if input.WithholdingAccountID == nil && defaultRecord.WithholdingAccountID.Valid {
				input.WithholdingAccountID = &defaultRecord.WithholdingAccountID.Int64
			}
		} else if !errors.Is(err, db.ErrNotFound) {
			return Transaction{}, fmt.Errorf("resolve dividend default: %w", err)
		}
	}
	incomeAccountID := int64(0)
	if input.IncomeAccountID != nil {
		incomeAccountID = *input.IncomeAccountID
	}
	if incomeAccountID <= 0 {
		return Transaction{}, ValidationError{Message: "dividend income account is required"}
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return Transaction{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	postings := []PostingInput{
		{AccountID: input.CashAccountID, QuantityValue: exact.New(input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
		{AccountID: incomeAccountID, QuantityValue: exact.New(-input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
	}
	if input.WithholdingValue != nil && *input.WithholdingValue > 0 {
		if input.WithholdingAccountID == nil || *input.WithholdingAccountID <= 0 {
			return Transaction{}, ValidationError{Message: "withholding account is required"}
		}
		scale := input.AmountScale
		if input.WithholdingScale != nil {
			scale = *input.WithholdingScale
		}
		// Withholding uses CashCommodityID by design: withholding is always in the
		// same currency as the dividend cash. If cross-currency withholding is ever
		// needed, add WithholdingCommodityID to DividendInput and validate it here.
		postings = append(postings,
			PostingInput{AccountID: *input.WithholdingAccountID, QuantityValue: exact.New(*input.WithholdingValue), QuantityScale: scale, CommodityID: input.CashCommodityID, Memo: memo},
			PostingInput{AccountID: input.CashAccountID, QuantityValue: exact.New(-*input.WithholdingValue), QuantityScale: scale, CommodityID: input.CashCommodityID, Memo: memo},
		)
	}
	return s.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "investment.dividend",
		ChangeReason:  input.ChangeReason,
		Spec: TransactionInput{
			Status:          status,
			TransactionKind: "investment",
			TransactionDate: date,
			PayeeID:         input.PayeeID,
			Description:     memo,
			JournalEntries: []JournalEntryInput{{
				EntryDate: date,
				EntryKind: "investment",
				Memo:      memo,
				Postings:  postings,
			}},
		},
	})
}

func (s *InvestmentService) ReinvestedDividend(ctx context.Context, input ReinvestedDividendInput) (InvestmentTradeResult, error) {
	date, err := validateReinvestedDividendInput(input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	incomeAccountID := int64(0)
	if input.IncomeAccountID != nil {
		incomeAccountID = *input.IncomeAccountID
	} else {
		defaultRecord, err := s.repository.ResolveDividendDefault(ctx, BookID, input.CommodityID, date)
		if err == nil {
			incomeAccountID = defaultRecord.IncomeAccountID
		} else if !errors.Is(err, db.ErrNotFound) {
			return InvestmentTradeResult{}, fmt.Errorf("resolve dividend default: %w", err)
		}
	}
	if incomeAccountID <= 0 {
		return InvestmentTradeResult{}, ValidationError{Message: "dividend income account is required"}
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentTradeResult{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return InvestmentTradeResult{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	transactionParams, err := s.transactionService.prepareCreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "investment.reinvested_dividend",
		ChangeReason:  input.ChangeReason,
		Spec: TransactionInput{
			Status:          status,
			TransactionKind: "investment",
			TransactionDate: date,
			PayeeID:         input.PayeeID,
			Description:     memo,
			JournalEntries: []JournalEntryInput{{
				EntryDate: date,
				EntryKind: "investment",
				Memo:      memo,
				Postings: []PostingInput{
					{AccountID: input.HoldingAccountID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
					{AccountID: tradingAccountID, QuantityValue: input.QuantityValue.Negated(), QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
					{AccountID: tradingAccountID, QuantityValue: exact.New(input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
					{AccountID: incomeAccountID, QuantityValue: exact.New(-input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
				},
			}},
		},
	})
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	transactionRecord, lot, err := s.repository.CreateTransactionAndLot(ctx, transactionParams, db.CreateInvestmentLotParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		OpenedOn:        date,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		CostBasisValue:  input.AmountValue,
		CostBasisScale:  input.AmountScale,
		CostCommodityID: input.CashCommodityID,
		MetadataJSON:    `{"source":"reinvested_dividend"}`,
		CreatedAt:       s.now().UTC().Format(time.RFC3339),
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.create",
		ChangeReason:    "created lot from reinvested dividend",
		EventKind:       "reinvested_dividend",
	})
	if err != nil {
		return InvestmentTradeResult{}, fmt.Errorf("create reinvested dividend transaction and lot: %w", mapTransactionDBError(err))
	}
	transaction := toTransaction(transactionRecord)
	if s.pricingService != nil {
		_ = s.pricingService.CreateTradeImpliedPrice(ctx, CreateTradeImpliedPriceInput{
			OwnerUserID:      input.OwnerUserID,
			AuthSessionID:    input.AuthSessionID,
			RequestID:        input.RequestID,
			CommodityID:      input.CommodityID,
			QuoteCommodityID: input.CashCommodityID,
			PriceDate:        date,
			QuantityValue:    input.QuantityValue,
			QuantityScale:    input.QuantityScale,
			CashValue:        input.AmountValue,
			CashScale:        input.AmountScale,
			PriceScale:       8,
		})
	}
	return InvestmentTradeResult{Transaction: transaction, LotID: &lot.ID}, nil
}

func (s *InvestmentService) ListLots(ctx context.Context, accountID int64, commodityID int64) ([]InvestmentLot, error) {
	records, err := s.repository.ListLots(ctx, BookID, accountID, commodityID)
	if err != nil {
		return nil, fmt.Errorf("list investment lots: %w", err)
	}
	return toInvestmentLots(records), nil
}

func (s *InvestmentService) Positions(ctx context.Context) ([]InvestmentPosition, error) {
	records, err := s.repository.Positions(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("read investment positions: %w", err)
	}
	return toInvestmentPositions(records), nil
}

func (s *InvestmentService) ListProviderEvents(ctx context.Context) ([]InvestmentProviderEvent, error) {
	records, err := s.repository.ListProviderEvents(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list investment provider events: %w", err)
	}
	return toInvestmentProviderEvents(records), nil
}

func (s *InvestmentService) ListEventSuggestions(ctx context.Context) ([]InvestmentEventSuggestion, error) {
	records, err := s.repository.ListEventSuggestions(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list investment event suggestions: %w", err)
	}
	return toInvestmentEventSuggestions(records), nil
}

func (s *InvestmentService) AcceptSuggestion(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, suggestionID int64) (InvestmentEventSuggestion, error) {
	return s.setSuggestionStatus(ctx, ownerUserID, authSessionID, requestID, suggestionID, "accepted")
}

func (s *InvestmentService) IgnoreSuggestion(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, suggestionID int64) (InvestmentEventSuggestion, error) {
	return s.setSuggestionStatus(ctx, ownerUserID, authSessionID, requestID, suggestionID, "ignored")
}

func (s *InvestmentService) setSuggestionStatus(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, suggestionID int64, status string) (InvestmentEventSuggestion, error) {
	if ownerUserID <= 0 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "owner user is required"}
	}
	if suggestionID <= 0 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "suggestion id is required"}
	}
	record, err := s.repository.SetSuggestionStatus(ctx, BookID, suggestionID, status, ownerUserID, authSessionID, requestID, s.now().UTC().Format(time.RFC3339))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotFound
		}
		return InvestmentEventSuggestion{}, fmt.Errorf("update investment suggestion: %w", err)
	}
	return toInvestmentEventSuggestion(record), nil
}

func (s *InvestmentService) ListAutomationRules(ctx context.Context) ([]InvestmentAutomationRule, error) {
	records, err := s.repository.ListAutomationRules(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	return toInvestmentAutomationRules(records), nil
}

func (s *InvestmentService) SaveAutomationRules(ctx context.Context, input SaveAutomationRulesInput) ([]InvestmentAutomationRule, error) {
	if input.OwnerUserID <= 0 {
		return nil, ValidationError{Message: "owner user is required"}
	}
	if len(input.Rules) == 0 {
		return nil, ValidationError{Message: "at least one automation rule is required"}
	}
	now := s.now().UTC()
	recordedAt := now.Format(time.RFC3339)
	rules := make([]InvestmentAutomationRule, 0, len(input.Rules))
	for _, ruleInput := range input.Rules {
		spec, err := cleanAutomationRuleSpec(ruleInput, now)
		if err != nil {
			return nil, err
		}
		operation := "investment.automation_rule.create"
		if ruleInput.RuleID > 0 {
			operation = "investment.automation_rule.update"
		}
		record, err := s.repository.SaveAutomationRule(ctx, db.SaveAutomationRuleParams{
			BookID:        BookID,
			RuleID:        ruleInput.RuleID,
			ActorUserID:   input.OwnerUserID,
			AuthSessionID: input.AuthSessionID,
			RequestID:     input.RequestID,
			OriginType:    "browser_api",
			Operation:     operation,
			Spec:          spec,
			RecordedAt:    recordedAt,
			ChangeReason:  input.ChangeReason,
		})
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return nil, ErrAutomationRuleNotFound
			}
			return nil, fmt.Errorf("save investment automation rule: %w", err)
		}
		rules = append(rules, toInvestmentAutomationRule(record))
	}
	return rules, nil
}

func cleanAutomationRuleSpec(input InvestmentAutomationRuleInput, now time.Time) (db.AutomationRuleSpec, error) {
	eventFamily := strings.TrimSpace(input.EventFamily)
	if !map[string]bool{"dividend": true, "distribution": true, "split": true, "merger": true, "spin_off": true, "ticker_change": true, "delisting": true, "cash_in_lieu": true, "return_of_capital": true, "corporate_action": true}[eventFamily] {
		return db.AutomationRuleSpec{}, ValidationError{Message: "event family is invalid"}
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "suggest"
	}
	if mode != "suggest" && mode != "auto_post" {
		return db.AutomationRuleSpec{}, ValidationError{Message: "automation mode is invalid"}
	}
	if input.ConfidenceThresholdBPS < 0 || input.ConfidenceThresholdBPS > 10000 {
		return db.AutomationRuleSpec{}, ValidationError{Message: "confidence threshold is invalid"}
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "archived" {
		return db.AutomationRuleSpec{}, ValidationError{Message: "automation rule status is invalid"}
	}
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return db.AutomationRuleSpec{}, err
	}
	effectiveTo, err := cleanOptionalDate(input.EffectiveTo, "effective to")
	if err != nil {
		return db.AutomationRuleSpec{}, err
	}
	if effectiveTo != "" && effectiveTo < effectiveFrom {
		return db.AutomationRuleSpec{}, ValidationError{Message: "effective to must be on or after effective from"}
	}
	requiredAccountsJSON, err := cleanSizedJSONObject(input.RequiredAccountsJSON, "required accounts", investmentJSONMaxBytes)
	if err != nil {
		return db.AutomationRuleSpec{}, err
	}
	if mode == "auto_post" && requiredAccountsJSON == "{}" {
		return db.AutomationRuleSpec{}, ValidationError{Message: "auto-post automation rules require account mappings"}
	}
	return db.AutomationRuleSpec{
		SourceID:               nullableInt64(input.SourceID),
		InstrumentID:           nullableInt64(input.InstrumentID),
		EventFamily:            eventFamily,
		Mode:                   mode,
		ConfidenceThresholdBPS: input.ConfidenceThresholdBPS,
		RequiredAccountsJSON:   requiredAccountsJSON,
		Status:                 status,
		EffectiveFrom:          effectiveFrom,
		EffectiveTo:            nullableSQLString(effectiveTo),
	}, nil
}

func cleanInvestmentInstrumentSpec(input InvestmentInstrumentInput) (db.InvestmentInstrumentSpec, error) {
	instrumentType := strings.TrimSpace(input.InstrumentType)
	if instrumentType == "" {
		instrumentType = "stock"
	}
	if !map[string]bool{"stock": true, "etf": true, "fund": true, "bond": true, "option": true, "future": true, "other": true}[instrumentType] {
		return db.InvestmentInstrumentSpec{}, ValidationError{Message: "instrument type is invalid"}
	}
	displayName, err := cleanOptionalText(input.DisplayName, "display name", 200)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	if displayName == "" {
		return db.InvestmentInstrumentSpec{}, ValidationError{Message: "display name is required"}
	}
	symbol, err := cleanOptionalText(strings.ToUpper(strings.TrimSpace(input.Symbol)), "symbol", 64)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	exchangeCode, err := cleanOptionalText(strings.ToUpper(strings.TrimSpace(input.ExchangeCode)), "exchange code", 32)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	mic, err := cleanOptionalText(strings.ToUpper(strings.TrimSpace(input.MIC)), "MIC", 16)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	issuer, err := cleanOptionalText(input.Issuer, "issuer", 200)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	countryCode := strings.ToUpper(strings.TrimSpace(input.CountryCode))
	if countryCode != "" {
		if len(countryCode) != 2 {
			return db.InvestmentInstrumentSpec{}, ValidationError{Message: "country code must be a 2-letter code"}
		}
		for _, character := range countryCode {
			if character < 'A' || character > 'Z' {
				return db.InvestmentInstrumentSpec{}, ValidationError{Message: "country code must use letters A-Z"}
			}
		}
	}
	if input.QuantityScale < 0 || input.QuantityScale > 12 {
		return db.InvestmentInstrumentSpec{}, ValidationError{Message: "quantity scale is invalid"}
	}
	if input.PriceScale < 0 || input.PriceScale > 12 {
		return db.InvestmentInstrumentSpec{}, ValidationError{Message: "price scale is invalid"}
	}
	identifiersJSON, err := cleanSizedJSONObject(input.IdentifiersJSON, "identifiers", investmentJSONMaxBytes)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	return db.InvestmentInstrumentSpec{
		CommodityID:        nullableInt64(input.CommodityID),
		CommodityCode:      strings.TrimSpace(input.CommodityCode),
		InstrumentType:     instrumentType,
		DisplayName:        displayName,
		Symbol:             symbol,
		ExchangeCode:       exchangeCode,
		MIC:                mic,
		Issuer:             issuer,
		CountryCode:        countryCode,
		QuoteCommodityID:   nullableInt64(input.QuoteCommodityID),
		TradingCommodityID: nullableInt64(input.TradingCommodityID),
		QuantityScale:      input.QuantityScale,
		PriceScale:         input.PriceScale,
		IdentifiersJSON:    identifiersJSON,
		MetadataJSON:       metadataJSON,
	}, nil
}

func cleanCostBasisProfileSpec(input CostBasisProfileInput) (db.CostBasisProfileSpec, error) {
	name, err := cleanOptionalText(input.Name, "name", 120)
	if err != nil {
		return db.CostBasisProfileSpec{}, err
	}
	if name == "" {
		return db.CostBasisProfileSpec{}, ValidationError{Message: "name is required"}
	}
	method := strings.TrimSpace(input.Method)
	if method == "" {
		method = "fifo"
	}
	if !map[string]bool{"fifo": true, "lifo": true, "average_cost": true, "specific_lot": true}[method] {
		return db.CostBasisProfileSpec{}, ValidationError{Message: "cost basis method is invalid"}
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "archived" {
		return db.CostBasisProfileSpec{}, ValidationError{Message: "cost basis profile status is invalid"}
	}
	description, err := cleanOptionalText(input.Description, "description", investmentTextMaxBytes)
	if err != nil {
		return db.CostBasisProfileSpec{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return db.CostBasisProfileSpec{}, err
	}
	return db.CostBasisProfileSpec{Name: name, Method: method, IsDefault: input.IsDefault, Status: status, Description: description, MetadataJSON: metadataJSON}, nil
}

func cleanDividendDefaultSpec(input DividendDefaultInput, now time.Time) (db.DividendDefaultSpec, error) {
	if input.IncomeAccountID <= 0 {
		return db.DividendDefaultSpec{}, ValidationError{Message: "income account is required"}
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "archived" {
		return db.DividendDefaultSpec{}, ValidationError{Message: "dividend default status is invalid"}
	}
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, now)
	if err != nil {
		return db.DividendDefaultSpec{}, err
	}
	effectiveTo, err := cleanOptionalDate(input.EffectiveTo, "effective to")
	if err != nil {
		return db.DividendDefaultSpec{}, err
	}
	if effectiveTo != "" && effectiveTo < effectiveFrom {
		return db.DividendDefaultSpec{}, ValidationError{Message: "effective to must be on or after effective from"}
	}
	taxCountryCode := strings.ToUpper(strings.TrimSpace(input.TaxCountryCode))
	if taxCountryCode != "" && len(taxCountryCode) != 2 {
		return db.DividendDefaultSpec{}, ValidationError{Message: "tax country code must be a 2-letter code"}
	}
	taxTreatment, err := cleanOptionalText(input.TaxTreatment, "tax treatment", 64)
	if err != nil {
		return db.DividendDefaultSpec{}, err
	}
	if input.DefaultWithholdingScale != nil && (*input.DefaultWithholdingScale < 0 || *input.DefaultWithholdingScale > 12) {
		return db.DividendDefaultSpec{}, ValidationError{Message: "withholding scale is invalid"}
	}
	if input.WithholdingRateBPS != nil && (*input.WithholdingRateBPS < 0 || *input.WithholdingRateBPS > 10000) {
		return db.DividendDefaultSpec{}, ValidationError{Message: "withholding rate is invalid"}
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return db.DividendDefaultSpec{}, err
	}
	return db.DividendDefaultSpec{
		CommodityID:             nullableInt64(input.CommodityID),
		IncomeAccountID:         input.IncomeAccountID,
		WithholdingAccountID:    nullableInt64(input.WithholdingAccountID),
		DefaultWithholdingValue: nullableInt64(input.DefaultWithholdingValue),
		DefaultWithholdingScale: nullableInt(input.DefaultWithholdingScale),
		WithholdingRateBPS:      nullableInt64(input.WithholdingRateBPS),
		TaxCountryCode:          taxCountryCode,
		TaxTreatment:            taxTreatment,
		Status:                  status,
		EffectiveFrom:           effectiveFrom,
		EffectiveTo:             nullableSQLString(effectiveTo),
		MetadataJSON:            metadataJSON,
	}, nil
}

var validCostBasisMethods = map[string]bool{
	"fifo": true, "lifo": true, "average_cost": true, "specific_lot": true,
}

func validateTradeInput(input InvestmentTradeInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if _, err := cleanRequiredDate(input.TransactionDate, "transaction date"); err != nil {
		return err
	}
	if input.CommodityID <= 0 || input.HoldingAccountID <= 0 || input.CashAccountID <= 0 || input.CashCommodityID <= 0 {
		return ValidationError{Message: "investment trade references are required"}
	}
	if input.QuantityValue.Sign() <= 0 {
		return ValidationError{Message: "quantity is required"}
	}
	if input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale || input.CashAmountScale < 0 || input.CashAmountScale > exact.MaxStandardScale {
		return ValidationError{Message: "quantity scale is invalid"}
	}
	if input.CashAmountValue <= 0 {
		return ValidationError{Message: "cash amount is required"}
	}
	method := strings.TrimSpace(input.CostBasisMethod)
	if method != "" && !validCostBasisMethods[method] {
		return ValidationError{Message: "cost basis method is invalid"}
	}
	if len(input.LotAllocations) > 0 && method != "specific_lot" {
		return ValidationError{Message: "lot allocations are only permitted with specific_lot cost basis method"}
	}
	if method == "specific_lot" && len(input.LotAllocations) == 0 {
		return ValidationError{Message: "specific_lot cost basis method requires lot allocations"}
	}
	return nil
}

func validateDividendInput(input DividendInput) (string, error) {
	if input.OwnerUserID <= 0 {
		return "", ValidationError{Message: "owner user is required"}
	}
	if input.CashAccountID <= 0 {
		return "", ValidationError{Message: "cash account is required"}
	}
	if input.CashCommodityID <= 0 {
		return "", ValidationError{Message: "cash commodity is required"}
	}
	if input.AmountValue <= 0 {
		return "", ValidationError{Message: "dividend amount is required"}
	}
	date, err := cleanRequiredDate(input.TransactionDate, "transaction date")
	if err != nil {
		return "", err
	}
	return date, nil
}

func validateReinvestedDividendInput(input ReinvestedDividendInput) (string, error) {
	if input.OwnerUserID <= 0 {
		return "", ValidationError{Message: "owner user is required"}
	}
	if input.CommodityID <= 0 {
		return "", ValidationError{Message: "commodity is required"}
	}
	if input.HoldingAccountID <= 0 {
		return "", ValidationError{Message: "holding account is required"}
	}
	if input.CashCommodityID <= 0 {
		return "", ValidationError{Message: "cash commodity is required"}
	}
	if input.QuantityValue.Sign() <= 0 {
		return "", ValidationError{Message: "quantity is required"}
	}
	if input.AmountValue <= 0 {
		return "", ValidationError{Message: "dividend amount is required"}
	}
	date, err := cleanRequiredDate(input.TransactionDate, "transaction date")
	if err != nil {
		return "", err
	}
	return date, nil
}

func toInvestmentInstrument(record db.InvestmentInstrumentRecord) InvestmentInstrument {
	return InvestmentInstrument{
		ID:                 record.ID,
		BookID:             record.BookID,
		CommodityID:        record.CommodityID,
		CommodityCode:      record.CommodityCode,
		InstrumentType:     record.InstrumentType,
		DisplayName:        record.DisplayName,
		Symbol:             nullableString(record.Symbol),
		ExchangeCode:       nullableString(record.ExchangeCode),
		MIC:                nullableString(record.MIC),
		Issuer:             nullableString(record.Issuer),
		CountryCode:        nullableString(record.CountryCode),
		QuoteCommodityID:   nullableSQLInt64Ptr(record.QuoteCommodityID),
		TradingCommodityID: nullableSQLInt64Ptr(record.TradingCommodityID),
		QuantityScale:      record.QuantityScale,
		PriceScale:         record.PriceScale,
		Status:             record.Status,
		IdentifiersJSON:    record.IdentifiersJSON,
		MetadataJSON:       record.MetadataJSON,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.RecordedAt,
	}
}

func toInvestmentInstruments(records []db.InvestmentInstrumentRecord) []InvestmentInstrument {
	instruments := make([]InvestmentInstrument, 0, len(records))
	for _, record := range records {
		instruments = append(instruments, toInvestmentInstrument(record))
	}
	return instruments
}

func toCostBasisProfile(record db.CostBasisProfileRecord) CostBasisProfile {
	return CostBasisProfile{ID: record.ID, BookID: record.BookID, Name: record.Name, Method: record.Method, IsDefault: record.IsDefault, Status: record.Status, Description: record.Description, MetadataJSON: record.MetadataJSON, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func toCostBasisProfiles(records []db.CostBasisProfileRecord) []CostBasisProfile {
	profiles := make([]CostBasisProfile, 0, len(records))
	for _, record := range records {
		profiles = append(profiles, toCostBasisProfile(record))
	}
	return profiles
}

func toDividendDefault(record db.DividendDefaultRecord) DividendDefault {
	var withholdingScale *int
	if record.DefaultWithholdingScale.Valid {
		scale := int(record.DefaultWithholdingScale.Int64)
		withholdingScale = &scale
	}
	return DividendDefault{
		ID:                      record.ID,
		BookID:                  record.BookID,
		CommodityID:             nullableSQLInt64Ptr(record.CommodityID),
		IncomeAccountID:         record.IncomeAccountID,
		WithholdingAccountID:    nullableSQLInt64Ptr(record.WithholdingAccountID),
		DefaultWithholdingValue: nullableSQLInt64Ptr(record.DefaultWithholdingValue),
		DefaultWithholdingScale: withholdingScale,
		WithholdingRateBPS:      nullableSQLInt64Ptr(record.WithholdingRateBPS),
		TaxCountryCode:          nullableString(record.TaxCountryCode),
		TaxTreatment:            nullableString(record.TaxTreatment),
		Status:                  record.Status,
		EffectiveFrom:           record.EffectiveFrom,
		EffectiveTo:             nullableString(record.EffectiveTo),
		MetadataJSON:            record.MetadataJSON,
		CreatedAt:               record.CreatedAt,
		UpdatedAt:               record.UpdatedAt,
	}
}

func toDividendDefaults(records []db.DividendDefaultRecord) []DividendDefault {
	defaults := make([]DividendDefault, 0, len(records))
	for _, record := range records {
		defaults = append(defaults, toDividendDefault(record))
	}
	return defaults
}

func toInvestmentLot(record db.InvestmentLotRecord) InvestmentLot {
	return InvestmentLot{
		ID:                      record.ID,
		BookID:                  record.BookID,
		AccountID:               record.AccountID,
		CommodityID:             record.CommodityID,
		OpenedOn:                record.OpenedOn,
		SourceTransactionID:     nullableSQLInt64Ptr(record.SourceTransactionID),
		Status:                  record.Status,
		QuantityValue:           record.QuantityValue,
		QuantityScale:           record.QuantityScale,
		RemainingQuantityValue:  record.RemainingQuantityValue,
		RemainingQuantityScale:  record.RemainingQuantityScale,
		CostBasisValue:          record.CostBasisValue,
		CostBasisScale:          record.CostBasisScale,
		RemainingCostBasisValue: record.RemainingCostBasisValue,
		RemainingCostBasisScale: record.RemainingCostBasisScale,
		CostCommodityID:         record.CostCommodityID,
		MetadataJSON:            record.MetadataJSON,
		CreatedAt:               record.CreatedAt,
		UpdatedAt:               record.UpdatedAt,
	}
}

func toInvestmentLots(records []db.InvestmentLotRecord) []InvestmentLot {
	lots := make([]InvestmentLot, 0, len(records))
	for _, record := range records {
		lots = append(lots, toInvestmentLot(record))
	}
	return lots
}

func toInvestmentPositions(records []db.InvestmentPositionRecord) []InvestmentPosition {
	positions := make([]InvestmentPosition, 0, len(records))
	for _, record := range records {
		var priceScale *int
		if record.LatestPriceScale.Valid {
			scale := int(record.LatestPriceScale.Int64)
			priceScale = &scale
		}
		positions = append(positions, InvestmentPosition{
			AccountID:               record.AccountID,
			CommodityID:             record.CommodityID,
			QuantityValue:           record.QuantityValue,
			QuantityScale:           record.QuantityScale,
			RemainingCostBasisValue: record.RemainingCostBasisValue,
			RemainingCostBasisScale: record.RemainingCostBasisScale,
			CostCommodityID:         record.CostCommodityID,
			LatestPriceValue:        nullableSQLInt64Ptr(record.LatestPriceValue),
			LatestPriceScale:        priceScale,
			LatestPriceDate:         nullableString(record.LatestPriceDate),
		})
	}
	return positions
}

func toInvestmentLotDisposals(records []db.LotDisposalRecord) []InvestmentLotDisposal {
	disposals := make([]InvestmentLotDisposal, 0, len(records))
	for _, record := range records {
		disposals = append(disposals, InvestmentLotDisposal{LotID: record.LotID, QuantityValue: record.QuantityValue, QuantityScale: record.QuantityScale, CostBasisValue: record.CostBasisValue, CostBasisScale: record.CostBasisScale})
	}
	return disposals
}

func toInvestmentProviderEvents(records []db.InvestmentProviderEventRecord) []InvestmentProviderEvent {
	events := make([]InvestmentProviderEvent, 0, len(records))
	for _, record := range records {
		events = append(events, InvestmentProviderEvent{ID: record.ID, BookID: record.BookID, SourceID: record.SourceID, ProviderEventID: nullableString(record.ProviderEventID), InstrumentID: nullableSQLInt64Ptr(record.InstrumentID), EventFamily: record.EventFamily, EventDate: record.EventDate, Status: record.Status, NormalizedJSON: record.NormalizedJSON, RawJSON: record.RawJSON, CreatedAt: record.CreatedAt})
	}
	return events
}

func toInvestmentEventSuggestion(record db.EventSuggestionRecord) InvestmentEventSuggestion {
	return InvestmentEventSuggestion{ID: record.ID, BookID: record.BookID, ProviderEventID: record.ProviderEventID, InstrumentID: nullableSQLInt64Ptr(record.InstrumentID), ConfidenceBPS: record.ConfidenceBPS, Status: record.Status, ProposedTransactionJSON: record.ProposedTransactionJSON, GeneratedTransactionID: nullableSQLInt64Ptr(record.GeneratedTransactionID), FailureReason: record.FailureReason, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func toInvestmentEventSuggestions(records []db.EventSuggestionRecord) []InvestmentEventSuggestion {
	suggestions := make([]InvestmentEventSuggestion, 0, len(records))
	for _, record := range records {
		suggestions = append(suggestions, toInvestmentEventSuggestion(record))
	}
	return suggestions
}

func toInvestmentAutomationRule(record db.AutomationRuleRecord) InvestmentAutomationRule {
	return InvestmentAutomationRule{
		ID:                     record.ID,
		BookID:                 record.BookID,
		SourceID:               nullableSQLInt64Ptr(record.SourceID),
		InstrumentID:           nullableSQLInt64Ptr(record.InstrumentID),
		EventFamily:            record.EventFamily,
		Mode:                   record.Mode,
		ConfidenceThresholdBPS: record.ConfidenceThresholdBPS,
		RequiredAccountsJSON:   record.RequiredAccountsJSON,
		Status:                 record.Status,
		EffectiveFrom:          record.EffectiveFrom,
		EffectiveTo:            record.EffectiveTo.String,
		CreatedAt:              record.CreatedAt,
		UpdatedAt:              record.UpdatedAt,
	}
}

func toInvestmentAutomationRules(records []db.AutomationRuleRecord) []InvestmentAutomationRule {
	rules := make([]InvestmentAutomationRule, 0, len(records))
	for _, record := range records {
		rules = append(rules, toInvestmentAutomationRule(record))
	}
	return rules
}

func defaultOperation(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func profileOperation(profileID int64) string {
	if profileID > 0 {
		return "investment.cost_basis_profile.update"
	}
	return "investment.cost_basis_profile.create"
}

func dividendDefaultOperation(defaultID int64) string {
	if defaultID > 0 {
		return "investment.dividend_default.update"
	}
	return "investment.dividend_default.create"
}

func boolPtr(value bool) *bool {
	return &value
}

func scaledDivision(numerator int64, numeratorScale int, denominator exact.Coefficient, denominatorScale int, resultScale int) (int64, error) {
	if denominator.Sign() == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	// Compute result = (numerator * 10^exponent) / denominator. When the
	// exponent is negative, scale the denominator instead of truncating the
	// numerator so all information remains available at the rounding boundary.
	exponent := resultScale + denominatorScale - numeratorScale
	n := new(big.Int).SetInt64(numerator)
	d := denominator.BigInt()
	if exponent >= 0 {
		n.Mul(n, pow10(exponent))
	} else {
		d.Mul(d, pow10(-exponent))
	}

	sign := n.Sign() * d.Sign()
	absN := new(big.Int).Abs(n)
	absD := new(big.Int).Abs(d)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(absN, absD, r)
	if new(big.Int).Lsh(r, 1).Cmp(absD) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if sign < 0 {
		q.Neg(q)
	}

	if !q.IsInt64() {
		return 0, fmt.Errorf("scaled division overflow")
	}
	return q.Int64(), nil
}

// GainsReportParams filters for the realized gains query.
type GainsReportParams struct {
	From string // optional ISO date, inclusive
	To   string // optional ISO date, inclusive
}

// RealizedGainEntry is one disposal event with its cash proceeds and gain.
type RealizedGainEntry struct {
	AccountID          int64
	CommodityID        int64
	CostCommodityID    int64
	DisposalDate       string
	TransactionID      *int64
	QuantityValue      exact.Coefficient
	QuantityScale      int
	DisposedBasisValue int64
	DisposedBasisScale int
	ProceedsValue      int64
	ProceedsScale      int
	RealizedGainValue  int64
}

// UnrealizedGainEntry is an open position with unrealized gain when a price is available.
type UnrealizedGainEntry struct {
	AccountID               int64
	CommodityID             int64
	CostCommodityID         int64
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	LatestPriceValue        *int64
	LatestPriceScale        *int
	LatestPriceDate         string
	MarketValueValue        *int64
	MarketValueScale        *int
	UnrealizedGainValue     *int64
	UnrealizedGainScale     *int
}

func (s *InvestmentService) ListRealizedGains(ctx context.Context, params GainsReportParams) ([]RealizedGainEntry, error) {
	records, err := s.repository.ListRealizedGains(ctx, BookID, db.RealizedGainsParams{From: params.From, To: params.To})
	if err != nil {
		return nil, fmt.Errorf("list realized gains: %w", err)
	}
	entries := make([]RealizedGainEntry, 0, len(records))
	for _, r := range records {
		entries = append(entries, RealizedGainEntry{
			AccountID:          r.AccountID,
			CommodityID:        r.CommodityID,
			CostCommodityID:    r.CostCommodityID,
			DisposalDate:       r.DisposalDate,
			TransactionID:      nullableSQLInt64Ptr(r.TransactionID),
			QuantityValue:      r.QuantityValue,
			QuantityScale:      r.QuantityScale,
			DisposedBasisValue: r.DisposedBasisValue,
			DisposedBasisScale: r.DisposedBasisScale,
			ProceedsValue:      r.ProceedsValue,
			ProceedsScale:      r.ProceedsScale,
			RealizedGainValue:  r.RealizedGainValue,
		})
	}
	return entries, nil
}

func (s *InvestmentService) ListUnrealizedGains(ctx context.Context) ([]UnrealizedGainEntry, error) {
	records, err := s.repository.PositionsWithGains(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list unrealized gains: %w", err)
	}
	entries := make([]UnrealizedGainEntry, 0, len(records))
	for _, r := range records {
		var priceScale *int
		if r.LatestPriceScale.Valid {
			s := int(r.LatestPriceScale.Int64)
			priceScale = &s
		}
		entries = append(entries, UnrealizedGainEntry{
			AccountID:               r.AccountID,
			CommodityID:             r.CommodityID,
			CostCommodityID:         r.CostCommodityID,
			QuantityValue:           r.QuantityValue,
			QuantityScale:           r.QuantityScale,
			RemainingCostBasisValue: r.RemainingCostBasisValue,
			RemainingCostBasisScale: r.RemainingCostBasisScale,
			LatestPriceValue:        nullableSQLInt64Ptr(r.LatestPriceValue),
			LatestPriceScale:        priceScale,
			LatestPriceDate:         nullableString(r.LatestPriceDate),
			MarketValueValue:        r.MarketValueValue,
			MarketValueScale:        r.MarketValueScale,
			UnrealizedGainValue:     r.UnrealizedGainValue,
			UnrealizedGainScale:     r.UnrealizedGainScale,
		})
	}
	return entries, nil
}
