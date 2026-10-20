package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
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
	// ErrInvestmentEventOutOfOrder reports a trade dated before something that
	// has already rewritten the position's cost-basis projection. It is the
	// repository's sentinel under this package's own name, so the API can key
	// on it without importing db and the explanation the repository wrote —
	// which event, which date, which disposal — reaches the user intact. It is
	// not a plain validation failure because the rule is not discoverable from
	// the form: the user has to be told that a position is entered in order,
	// not merely that something is wrong (T-95).
	ErrInvestmentEventOutOfOrder      = db.ErrOutOfOrderPositionEvent
	ErrInvestmentSuggestionNotFound   = errors.New("investment event suggestion not found")
	ErrInvestmentSuggestionNotPending = errors.New("investment event suggestion is not pending")
	ErrAutomationRuleNotFound         = errors.New("investment automation rule not found")
	// ErrGainImpactAcknowledgementRequired and ErrGainImpactAcknowledgementStale
	// refuse an opted-in command whose replay changes committed disposal gains
	// without, or with a different, acknowledgement. Every write rolls back.
	ErrGainImpactAcknowledgementRequired = db.ErrGainImpactAcknowledgementRequired
	ErrGainImpactAcknowledgementStale    = db.ErrGainImpactAcknowledgementStale
)

type InstrumentSearchProvider interface {
	SearchInstruments(ctx context.Context, query string) ([]InstrumentSearchCandidate, error)
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
	QuoteIdentifiers   map[string]string
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
	VersionID    int64
	VersionSeq   int64
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
	// CostCommodityID pins a write-off's basis currency without a cash leg.
	// Other trades use CashCommodityID for their disposal currency.
	CostCommodityID int64
	// Gross and net are signed owner-perspective source amounts. Nil gross
	// keeps the existing net-only command, whose gross remains unknown.
	GrossAmountValue   *int64
	GrossAmountScale   int
	NetSettlementValue *int64
	NetSettlementScale int
	SettlementDate     string
	Charges            []InvestmentTradeChargeInput
	Memo               string
	ExternalRefHint    string
	MetadataJSON       string
	PayeeID            *int64
	Status             string
	LotAllocations     []InvestmentLotAllocationInput
	ChangeReason       string
	CostBasisMethod    string
	// ReconciliationOverride allows backdated investment trades to invalidate
	// affected checkpoints through the normal transaction write guard.
	ReconciliationOverride CheckpointOverride
	// GainImpactAcknowledgement is the token a preview returned for the
	// committed-disposal gain changes the user accepted (T-114). Commands that
	// opt into gain disclosure refuse a non-empty change set without the exact
	// token; an empty change set needs none.
	GainImpactAcknowledgement string
	// WriteOff records a disposal at zero proceeds: a fund closure, a worthless
	// delisting, any total loss. It is set only by WriteOff() — never by the
	// sell endpoint — so a mistyped sell amount can never become a write-off.
	// The trade carries no cash legs; its disposal decision records zero proceeds.
	WriteOff bool
	// CashInLieu marks the disposal of a split's fractional entitlement for
	// cash (T-147). It is set only by CashInLieu(); the operation kind is
	// cash_in_lieu and the cash posts on SettlementDate, its payment date.
	CashInLieu bool
	// OriginType/Operation override the default "browser_api"/"investment.buy"
	// (or "investment.sell") attribution — set by the Trading 212 import
	// commit path (B-T212-INVST) so an auto-booked trade is correctly
	// labeled OriginType="import" instead of looking like a manual browser
	// action. Empty (every existing caller) keeps the original defaults.
	OriginType string
	Operation  string
}

type InvestmentTradeChargeInput struct {
	Kind               string
	AmountValue        int64
	AmountScale        int
	CommodityID        int64
	Treatment          string
	ChargeAccountID    *int64
	CashAccountID      *int64
	PaidOn             string
	SourceEvidenceJSON string
}

// InvestmentWriteOffInput records a total loss on a holding. There is no cash
// account or amount: proceeds are zero by definition, which is what separates
// it from a sale (T-38).
type InvestmentWriteOffInput struct {
	OwnerUserID      int64
	AuthSessionID    int64
	RequestID        string
	TransactionDate  string
	CommodityID      int64
	HoldingAccountID int64
	// Zero keeps single-currency inference; an explicit currency selects only
	// that long position, including its historically closed lots (#199).
	CostCommodityID int64
	QuantityValue   exact.Coefficient
	QuantityScale   int
	// Reason is required: "delisted", "fund closed", "issuer liquidated". A
	// position declared worthless without a stated cause is not auditable.
	Reason          string
	Memo            string
	ExternalRefHint string
	MetadataJSON    string
	PayeeID         *int64
	Status          string
	LotAllocations  []InvestmentLotAllocationInput
	ChangeReason    string
	CostBasisMethod string
	// ReconciliationOverride lets a backdated write-off invalidate affected
	// checkpoints through the normal transaction write guard.
	ReconciliationOverride CheckpointOverride
	OriginType             string
	Operation              string
	// GainImpactAcknowledgement echoes the preview token when a backdated
	// write-off revises committed gains (T-117).
	GainImpactAcknowledgement string
}

// asTradeInput reuses the shared disposal planner, which only reads the lot
// selection fields. The zero cash fields are never consulted — WriteOff says so
// outright, so a validator handed this input knows the missing cash leg is the
// point rather than an omission.
func (input InvestmentWriteOffInput) asTradeInput() InvestmentTradeInput {
	return InvestmentTradeInput{
		WriteOff:         true,
		OwnerUserID:      input.OwnerUserID,
		AuthSessionID:    input.AuthSessionID,
		RequestID:        input.RequestID,
		TransactionDate:  input.TransactionDate,
		CommodityID:      input.CommodityID,
		HoldingAccountID: input.HoldingAccountID,
		CostCommodityID:  input.CostCommodityID,
		QuantityValue:    input.QuantityValue,
		QuantityScale:    input.QuantityScale,
		LotAllocations:   input.LotAllocations,
		CostBasisMethod:  input.CostBasisMethod,
	}
}

func validateWriteOffInput(input InvestmentWriteOffInput) (string, error) {
	if err := validatePostedInvestmentStatus(input.Status); err != nil {
		return "", err
	}
	if input.OwnerUserID <= 0 {
		return "", ValidationError{Message: "owner user is required"}
	}
	if _, err := cleanRequiredDate(input.TransactionDate, "transaction date"); err != nil {
		return "", err
	}
	if input.CommodityID <= 0 || input.HoldingAccountID <= 0 {
		return "", ValidationError{Message: "write-off references are required"}
	}
	if input.QuantityValue.Sign() <= 0 {
		return "", ValidationError{Message: "quantity is required"}
	}
	if input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale {
		return "", ValidationError{Message: "quantity scale is invalid"}
	}
	if strings.TrimSpace(input.Reason) == "" {
		return "", ValidationError{Message: "write-off reason is required"}
	}
	reason, err := cleanOptionalText(input.Reason, "write-off reason", investmentTextMaxBytes)
	if err != nil {
		return "", err
	}
	method := strings.TrimSpace(input.CostBasisMethod)
	if method != "" && !validCostBasisMethods[method] {
		return "", ValidationError{Message: "cost basis method is invalid"}
	}
	if len(input.LotAllocations) > 0 && method != "specific_lot" {
		return "", ValidationError{Message: "lot allocations are only permitted with specific_lot cost basis method"}
	}
	if method == "specific_lot" && len(input.LotAllocations) == 0 {
		return "", ValidationError{Message: "specific_lot cost basis method requires lot allocations"}
	}
	return reason, nil
}

type SellPreviewResult struct {
	CostBasisMethod   string
	DisposalDecision  DisposalDecision
	Allocations       []InvestmentLotDisposal
	RealizedGain      int64
	RealizedGainScale int
	// BasisKnowledge is unknown when the sale consumes unknown basis; the
	// disposed basis and RealizedGain are then unresolved (T-145).
	BasisKnowledge     string
	CashAmountValue    int64
	CashAmountScale    int
	GrossAmountValue   *int64
	GrossAmountScale   int
	NetSettlementValue int64
	NetSettlementScale int
	SettlementDate     string
	Charges            []InvestmentTradeChargeInput
}

type InvestmentLotAllocationInput struct {
	LotID         int64
	QuantityValue exact.Coefficient
	QuantityScale int
}

type InvestmentTradeResult struct {
	Transaction      Transaction
	LotID            *int64
	Allocations      []InvestmentLotDisposal
	DisposalDecision *DisposalDecision
}

type DisposalDecision struct {
	ID                   *int64
	TransactionID        *int64
	TransactionVersionID *int64
	CostBasisMethod      string
	ResolutionTier       string
	AccountVersionID     *int64
	ProfileID            *int64
	ProfileVersionID     *int64
	SourceEffectiveFrom  string
	SourceRecordedAt     string
	QuantityValue        exact.Coefficient
	QuantityScale        int
	DisposedBasisValue   exact.Coefficient
	DisposedBasisScale   int
	// BasisKnowledge is unknown when any allocation is unknown; the disposed
	// basis is then unused and the decision's gain is unresolved.
	BasisKnowledge  string
	ProceedsValue   int64
	ProceedsScale   int
	CostCommodityID int64
	AuditEventID    *int64
	Allocations     []InvestmentLotDisposal
}

type InvestmentLotDisposal struct {
	LotID          int64
	QuantityValue  exact.Coefficient
	QuantityScale  int
	CostBasisValue int64
	CostBasisScale int
	BasisKnowledge string // unknown leaves CostBasisValue/Scale unused
	ProceedsValue  int64
	ProceedsScale  int
}

// knownDisposedBasis totals the consumed basis, or reports false when any
// allocation consumed unknown basis: that total is never a partial sum.
func knownDisposedBasis(disposals []db.LotDisposalRecord) (*exact.ScaledInt, bool) {
	basis := exact.NewScaledInt()
	for _, disposal := range disposals {
		if disposal.BasisKnowledge == db.InvestmentBasisUnknown {
			return nil, false
		}
		basis.AddInt64(disposal.CostBasisValue, disposal.CostBasisScale)
	}
	return basis, true
}

func basisKnowledgeCode(known bool) string {
	if known {
		return db.InvestmentBasisKnown
	}
	return db.InvestmentBasisUnknown
}

func previewDisposalDecision(input InvestmentTradeInput, method string, source db.DisposalDecisionSource, disposals []db.LotDisposalRecord) (DisposalDecision, error) {
	basis, known := knownDisposedBasis(disposals)
	if !known {
		basis = exact.NewScaledInt()
	}
	basisValue, err := basis.Coefficient()
	if err != nil {
		return DisposalDecision{}, err
	}
	costCommodityID := input.CashCommodityID
	if costCommodityID == 0 && len(disposals) > 0 {
		costCommodityID = disposals[0].CostCommodityID
	}
	return DisposalDecision{
		CostBasisMethod: method, ResolutionTier: source.ResolutionTier,
		AccountVersionID: optionalPositiveID(source.AccountVersionID), ProfileID: optionalPositiveID(source.ProfileID),
		ProfileVersionID: optionalPositiveID(source.ProfileVersionID), SourceEffectiveFrom: source.SourceEffectiveFrom,
		SourceRecordedAt: source.SourceRecordedAt, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale,
		DisposedBasisValue: basisValue, DisposedBasisScale: basis.Scale(), BasisKnowledge: basisKnowledgeCode(known),
		CostCommodityID: costCommodityID, Allocations: toInvestmentLotDisposals(disposals),
	}, nil
}

func toDisposalDecision(record db.DisposalDecisionRecord) DisposalDecision {
	return DisposalDecision{
		ID: optionalPositiveID(record.ID), TransactionID: optionalPositiveID(record.TransactionID),
		TransactionVersionID: optionalPositiveID(record.TransactionVersionID), CostBasisMethod: record.CostBasisMethod,
		ResolutionTier: record.ResolutionTier, AccountVersionID: optionalPositiveID(record.AccountVersionID),
		ProfileID: optionalPositiveID(record.ProfileID), ProfileVersionID: optionalPositiveID(record.ProfileVersionID),
		SourceEffectiveFrom: record.SourceEffectiveFrom, SourceRecordedAt: record.SourceRecordedAt,
		QuantityValue: record.QuantityValue, QuantityScale: record.QuantityScale,
		DisposedBasisValue: record.DisposedBasisValue, DisposedBasisScale: record.DisposedBasisScale,
		BasisKnowledge: normalizedKnowledge(record.BasisKnowledge),
		ProceedsValue:  record.ProceedsValue, ProceedsScale: record.ProceedsScale,
		CostCommodityID: record.CostCommodityID, AuditEventID: optionalPositiveID(record.AuditEventID),
		Allocations: toInvestmentLotDisposals(record.Allocations),
	}
}

func optionalPositiveID(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

type DividendInput struct {
	PayeeName   string
	TagIDs      []int64
	NeedsReview bool
	// WithholdingOnly records a separate tax payment: AmountValue is tax, with no income leg.
	WithholdingOnly      bool
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
	ExternalRefHint      string
	MetadataJSON         string
	PayeeID              *int64
	Status               string
	ChangeReason         string
	// ReconciliationOverride allows backdated dividends to invalidate affected
	// checkpoints through the normal transaction write guard.
	ReconciliationOverride CheckpointOverride
	// OriginType/Operation: see InvestmentTradeInput's doc comment — same
	// override mechanism, same reason (B-T212-INVST import attribution).
	OriginType string
	Operation  string
}

type ReinvestedDividendInput struct {
	OwnerUserID            int64
	AuthSessionID          int64
	RequestID              string
	TransactionDate        string
	CommodityID            int64
	HoldingAccountID       int64
	IncomeAccountID        *int64
	QuantityValue          exact.Coefficient
	QuantityScale          int
	AmountValue            int64
	AmountScale            int
	CashCommodityID        int64
	Memo                   string
	PayeeID                *int64
	Status                 string
	ChangeReason           string
	ReconciliationOverride CheckpointOverride
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted (T-126).
	GainImpactAcknowledgement string
}

type InvestmentLot struct {
	OpeningBasisKnowledge   string
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
	BasisKnowledge          string
	// PositionSide is short for a lot of borrowed units, whose basis amounts
	// are its opening proceeds (#173).
	PositionSide string
	// Origin is set on the lot list for a lot opened by a transfer or share
	// exchange (#178).
	Origin *InvestmentLotOrigin
}

// InvestmentLotOrigin is the transfer link that opened a lot. Ratio and the
// source instrument describe a share exchange; OriginalAcquiredOn is empty
// when the original date is unknown.
type InvestmentLotOrigin struct {
	OperationID           int64
	TransferKind          string
	SourceCommodityID     int64
	SourceAccountID       *int64
	SourceLotID           *int64
	RatioNumerator        *int64
	RatioDenominator      *int64
	OriginalDateKnowledge string
	OriginalAcquiredOn    string
}

type InvestmentPosition struct {
	PriceStatus             string
	ValuationAsOf           string
	KnowledgeCutoff         string
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
	LatestPriceApproximate  bool
	BasisKnowledge          string
	// TransferBasisAllocation is how an internal transfer from this position
	// allocates basis: the position's method lock, else its resolved default.
	TransferBasisAllocation string
	// PositionSide is short for owed units; QuantityValue stays positive and
	// the basis amounts are remaining opening proceeds.
	PositionSide string
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
	EventFamily             string
	EventDate               string
	EventStatus             string
	SourceCode              string
	EventEvidenceJSON       string
	InstrumentID            *int64
	ConfidenceBPS           int
	Status                  string
	ProposedTransactionJSON string
	GeneratedTransactionID  *int64
	LinkedTransactionID     *int64
	FailureReason           string
	CreatedAt               string
	UpdatedAt               string
	// LinkCandidates are owner-recorded dividends, or payments in lieu for a
	// short-only suggestion (#258), a pending suggestion may be linked to
	// instead of posting again; composed only by the list read.
	LinkCandidates []InvestmentEventLinkCandidate
	// Revisions are later differing provider observations of the event, in
	// observation order (#254); composed only by the list read. They are
	// evidence beside the first observation and never change the decision.
	Revisions []InvestmentEventRevision
}

// InvestmentEventRevision is one appended provider revision: its normalized
// provider content and when it was observed.
type InvestmentEventRevision struct {
	EvidenceJSON string
	ObservedAt   string
}

// InvestmentEventLinkCandidate is an effective posted cash dividend of the
// suggestion's instrument dated within the link window after the ex-date, or
// for a payment-in-lieu suggestion an effective payment in lieu of the
// instrument with the event's ex-date for one of its short holdings. Gross is
// the dividend's gross or the amount owed.
type InvestmentEventLinkCandidate struct {
	TransactionID      int64
	TransactionDate    string
	CashCode           string
	GrossValue         exact.Coefficient
	GrossScale         int
	HoldingAccountID   *int64
	HoldingAccountName string
}

// SuggestionCompletion is what the owner supplies for an incomplete provider
// proposal: the facts a per-share announcement cannot state. Income and
// withholding accounts may still come from the instrument's dividend default.
type SuggestionCompletion struct {
	TransactionDate      string
	CashAccountID        int64
	CashCommodityID      int64
	AmountValue          int64
	AmountScale          int
	IncomeAccountID      *int64
	WithholdingValue     *int64
	WithholdingScale     *int
	WithholdingAccountID *int64
	// A payment in lieu completion (#255) names the expense account, the
	// short holding when the evidence lists several, and any recovery;
	// AmountValue is then the amount owed.
	HoldingAccountID       *int64
	ExpenseAccountID       *int64
	RecoveryValue          *int64
	RecoveryScale          *int
	RecoveryAccountID      *int64
	Memo                   string
	ReconciliationOverride CheckpointOverride
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
	gainsReadStageForTest func(string)
	consistentGainsRead   bool
	repository            *db.InvestmentRepository
	accountService        *AccountService
	transactionService    *TransactionService
	pricingService        *PricingService
	settings              *SettingsService
	now                   func() time.Time
}

// SetSettings resolves owner-entered version dates from the owner's day (#339).
func (s *InvestmentService) SetSettings(settings *SettingsService) { s.settings = settings }

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

	if input.CommodityID != nil {
		if err := s.validateInstrumentCommodity(ctx, *input.CommodityID, spec); err != nil {
			return InvestmentInstrument{}, err
		}
	}

	now := s.now().UTC()
	today, err := s.settings.OwnerTodayAt(ctx, input.OwnerUserID, now)
	if err != nil {
		return InvestmentInstrument{}, err
	}
	_, err = cleanEffectiveFrom(input.EffectiveFrom, today)
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
		EffectiveFrom:   db.CommodityGenesisDate,
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

// ResolveOrCreateInstrumentForImport finds an instrument by ISIN, falling
// back to ticker/symbol, creating a new one (named/coded from the ticker)
// only if neither matches. Used by the Trading 212 import commit path
// (B-T212-INVST): creation only ever happens here, at commit time, never at
// fetch/preview time — see docs/plans/import-connection-accounts-plan.md's
// discard-orphan concern (a discarded preview batch must not leave behind a
// durably-created instrument nobody asked for). Returns the resolved
// instrument's InstrumentID and CommodityID together, since callers need
// both (CommodityID for postings, InstrumentID for CreateHoldingAccount).
// FindInstrumentForImport is the read-only half of
// ResolveOrCreateInstrumentForImport: ISIN first, then ticker. Previews use it
// so they never create an instrument.
func (s *InvestmentService) FindInstrumentForImport(ctx context.Context, isin string, ticker string) (instrumentID int64, commodityID int64, found bool, err error) {
	isin = strings.TrimSpace(isin)
	ticker = strings.TrimSpace(ticker)
	if isin != "" {
		if rec, err := s.repository.InstrumentByISIN(ctx, BookID, isin); err == nil {
			return rec.ID, rec.CommodityID, true, nil
		} else if !errors.Is(err, db.ErrNotFound) {
			return 0, 0, false, fmt.Errorf("find instrument by isin: %w", err)
		}
	}
	if ticker != "" {
		if rec, err := s.repository.InstrumentBySymbol(ctx, BookID, ticker); err == nil {
			return rec.ID, rec.CommodityID, true, nil
		} else if !errors.Is(err, db.ErrNotFound) {
			return 0, 0, false, fmt.Errorf("find instrument by symbol: %w", err)
		}
	}
	return 0, 0, false, nil
}

func (s *InvestmentService) ResolveOrCreateInstrumentForImport(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, isin string, ticker string) (instrumentID int64, commodityID int64, created bool, err error) {
	isin = strings.TrimSpace(isin)
	ticker = strings.TrimSpace(ticker)
	if instrumentID, commodityID, found, err := s.FindInstrumentForImport(ctx, isin, ticker); err != nil || found {
		return instrumentID, commodityID, false, err
	}
	if ticker == "" {
		return 0, 0, false, fmt.Errorf("cannot create instrument: no ticker")
	}

	identifiersJSON := "{}"
	if isin != "" {
		if encoded, err := json.Marshal(map[string]string{"isin": isin}); err == nil {
			identifiersJSON = string(encoded)
		}
	}
	instrument, err := s.CreateInstrument(ctx, InvestmentInstrumentInput{
		OwnerUserID:    ownerUserID,
		AuthSessionID:  authSessionID,
		RequestID:      requestID,
		OriginType:     "import",
		Operation:      "investment.instrument.create_from_import",
		CommodityCode:  ticker,
		InstrumentType: "stock",
		DisplayName:    ticker,
		Symbol:         ticker,
		QuantityScale:  6,
		PriceScale:     4,
		// No EffectiveFrom: the commodity's first version is stamped
		// db.CommodityGenesisDate, so a back-dated fill resolves whatever the
		// instrument's own version date is (T-42). This used to have to be
		// the trade's date, and even then only covered the instrument's first
		// sighting — a later import carrying an *earlier* trade for an
		// already-known instrument still failed.
		IdentifiersJSON: identifiersJSON,
		MetadataJSON:    "{}",
		ChangeReason:    "created from Trading 212 import",
	})
	if err != nil {
		return 0, 0, false, fmt.Errorf("create instrument for import: %w", err)
	}
	return instrument.ID, instrument.CommodityID, true, nil
}

// DeleteUnusedInstrumentForImport compensates a failed import attempt that
// created an instrument but no durable trade. Posted instruments are protected
// by the repository's foreign-key checks and are never deleted here.
func (s *InvestmentService) DeleteUnusedInstrumentForImport(ctx context.Context, instrumentID int64) error {
	if err := s.repository.DeleteUnusedInstrument(ctx, BookID, instrumentID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrInvestmentInstrumentNotFound
		}
		return fmt.Errorf("delete unused import instrument: %w", err)
	}
	return nil
}

// DeleteUnusedHoldingAccountForImport compensates a holding account created
// for an import row that did not produce a trade. DeleteUnusedAccount refuses
// any account that has postings or other durable references.
func (s *InvestmentService) DeleteUnusedHoldingAccountForImport(ctx context.Context, ownerUserID int64, accountID int64) error {
	if err := s.accountService.DeleteUnusedAccount(ctx, DeleteAccountInput{OwnerUserID: ownerUserID, AccountID: accountID}); err != nil {
		return fmt.Errorf("delete unused import holding account: %w", err)
	}
	return nil
}

func (s *InvestmentService) UpdateInstrument(ctx context.Context, input InvestmentInstrumentInput) (InvestmentInstrument, error) {
	if input.OwnerUserID <= 0 {
		return InvestmentInstrument{}, ValidationError{Message: "owner user is required"}
	}
	if input.InstrumentID <= 0 {
		return InvestmentInstrument{}, ValidationError{Message: "instrument id is required"}
	}
	current, err := s.repository.InstrumentByID(ctx, BookID, input.InstrumentID)
	if errors.Is(err, db.ErrNotFound) {
		return InvestmentInstrument{}, ErrInvestmentInstrumentNotFound
	}
	if err != nil {
		return InvestmentInstrument{}, fmt.Errorf("read instrument for update: %w", err)
	}
	if strings.TrimSpace(input.IdentifiersJSON) == "" {
		input.IdentifiersJSON = current.IdentifiersJSON
	}
	if strings.TrimSpace(input.MetadataJSON) == "" {
		input.MetadataJSON = current.MetadataJSON
	}

	spec, err := cleanInvestmentInstrumentSpec(input)
	if err != nil {
		return InvestmentInstrument{}, err
	}

	if input.CommodityID != nil && *input.CommodityID != current.CommodityID {
		return InvestmentInstrument{}, ValidationError{Message: "instrument commodity cannot change"}
	}
	if err := s.validateInstrumentCommodity(ctx, current.CommodityID, spec); err != nil {
		return InvestmentInstrument{}, err
	}

	now := s.now().UTC()
	today, err := s.settings.OwnerTodayAt(ctx, input.OwnerUserID, now)
	if err != nil {
		return InvestmentInstrument{}, err
	}
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, today)
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
		AllowsPostings:        new(true),
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
	today, err := s.settings.OwnerTodayAt(ctx, input.OwnerUserID, s.now())
	if err != nil {
		return DividendDefault{}, err
	}
	spec, err := cleanDividendDefaultSpec(input, today)
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
	result, err := s.buy(ctx, input, nil)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	result.Transaction, err = s.transactionService.enrichOne(ctx, result.Transaction)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	return result, nil
}

// buyWithPostWrite is used by imports that must persist an idempotency marker
// inside the same transaction as the trade and its lot. Its internal receipt
// only supplies durable IDs, so optional response enrichment cannot turn an
// already committed import into a reported financial failure.
func (s *InvestmentService) buyWithPostWrite(ctx context.Context, input InvestmentTradeInput, postWrite func(*sql.Tx, int64) error) (InvestmentTradeResult, error) {
	return s.buy(ctx, input, postWrite)
}

// investmentTransactionPlan is the transaction a write path is about to create,
// separated from the writing of it. Splitting it out lets the reconciliation
// preview build exactly the same postings the real trade would, without
// persisting anything — a preview that guessed at the postings would name the
// wrong checkpoints, which is worse than naming none.
type investmentTransactionPlan struct {
	Create         CreateTransactionInput
	TradeEconomics *tradeEconomics
	// MetadataJSON is the cleaned metadata the lot and disposal records reuse.
	MetadataJSON string
	// Date is the validated transaction date. The dividend paths normalise it,
	// so callers must use this rather than the raw input.
	Date string
	// AccountRuleDependencies holds the account versions this plan's role
	// checks were decided against. The journal preparation that follows adds
	// its own reads to the same collector, and the write refuses the command
	// if any of those accounts changed after the role was decided (T-100).
	AccountRuleDependencies *accountRuleDependencies
}

func (s *InvestmentService) buyPlan(ctx context.Context, input InvestmentTradeInput) (investmentTransactionPlan, error) {
	if err := validateTradeInput(input); err != nil {
		return investmentTransactionPlan{}, err
	}
	dependencies := newAccountRuleDependencies()
	if err := s.validateTradeRoles(ctx, input, dependencies); err != nil {
		return investmentTransactionPlan{}, err
	}
	economics, err := s.calculateTradeEconomics(ctx, input, true, dependencies)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return investmentTransactionPlan{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	return investmentTransactionPlan{
		TradeEconomics:          &economics,
		AccountRuleDependencies: dependencies,
		MetadataJSON:            metadataJSON,
		Date:                    input.TransactionDate,
		Create: CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             defaultString(input.OriginType, "browser_api"),
			Operation:              defaultString(input.Operation, "investment.buy"),
			ChangeReason:           input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
			Spec: TransactionInput{
				Status:                  status,
				TransactionKind:         "investment",
				InvestmentOperationKind: "buy",
				TransactionDate:         input.TransactionDate,
				PayeeID:                 input.PayeeID,
				Description:             memo,
				ExternalRefHint:         input.ExternalRefHint,
				MetadataJSON:            metadataJSON,
				JournalEntries:          tradeJournalEntries(input, tradingAccountID, memo, economics, true),
			},
		},
	}, nil
}

func (s *InvestmentService) buy(ctx context.Context, input InvestmentTradeInput, postWrite func(*sql.Tx, int64) error) (InvestmentTradeResult, error) {
	transactionParams, lotParams, err := s.prepareBuyWrite(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	var transactionRecord db.TransactionRecord
	var lot db.InvestmentLotRecord
	// Manual (T-114) and imported (T-126) acquisitions disclose gain changes.
	transactionParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	if postWrite == nil {
		transactionRecord, lot, err = s.repository.CreateTransactionAndLot(ctx, transactionParams, lotParams)
	} else {
		transactionRecord, lot, err = s.repository.CreateTransactionAndLotWithPostWrite(ctx, transactionParams, lotParams, postWrite)
	}
	if err != nil {
		return InvestmentTradeResult{}, mapInvestmentOpeningWriteError(err, "buy")
	}

	transaction := toTransaction(transactionRecord)
	return InvestmentTradeResult{Transaction: transaction, LotID: &lot.ID}, nil
}

func mapInvestmentOpeningWriteError(err error, kind string) error {
	var dependency *db.InvestmentReplayDependencyError
	if errors.As(err, &dependency) {
		return InvestmentBuyDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	}
	if errors.Is(err, db.ErrOutOfOrderPositionEvent) || errors.Is(err, db.ErrGainImpactAcknowledgementRequired) ||
		errors.Is(err, db.ErrGainImpactAcknowledgementStale) {
		return err
	}
	return fmt.Errorf("create %s transaction and lot: %w", kind, mapTransactionDBError(err))
}

// prepareBuyWrite freezes the same exact journal and lot-opening facts for
// normal entry and a native buy replacement before either begins its write.
func (s *InvestmentService) prepareBuyWrite(ctx context.Context, input InvestmentTradeInput) (db.CreateTransactionParams, db.CreateInvestmentLotParams, error) {
	plan, err := s.buyPlan(ctx, input)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
	}
	metadataJSON := plan.MetadataJSON
	transactionParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
	}
	transactionParams.InvestmentComponents = plan.TradeEconomics.components(input.CashCommodityID)
	transactionParams.InvestmentSettlementDate = plan.TradeEconomics.SettlementDate
	if s.pricingService != nil && !plan.TradeEconomics.PriceUnavailable {
		transactionParams.TradeImpliedPrice, err = tradePriceSpec(input.CommodityID, input.CashCommodityID,
			input.TransactionDate, input.QuantityValue, input.QuantityScale,
			plan.TradeEconomics.PriceValue, plan.TradeEconomics.PriceScale, plan.TradeEconomics.PriceApproximate)
		if err != nil {
			return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
		}
	}
	now := s.now().UTC().Format(time.RFC3339)
	basis := exact.ScaledIntFromInt64(plan.TradeEconomics.ClearingValue, plan.TradeEconomics.ClearingScale).Negated()
	basisValue, err := basis.Int64()
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
	}
	lotParams := db.CreateInvestmentLotParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		OpenedOn:        input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		CostBasisValue:  basisValue,
		CostBasisScale:  basis.Scale(),
		CostCommodityID: input.CashCommodityID,
		MetadataJSON:    metadataJSON,
		CreatedAt:       now,
		CreatedByUserID: input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		OriginType:      defaultString(input.OriginType, "browser_api"),
		Operation:       "investment.lot.create",
		ChangeReason:    "created lot from buy transaction",
		EventKind:       "acquisition",
	}
	return transactionParams, lotParams, nil
}

func tradePriceSpec(baseID, quoteID int64, date string, quantity exact.Coefficient, quantityScale int, cash int64, cashScale int, approximate bool) (*db.TradeImpliedPriceSpec, error) {
	priceValue, err := scaledDivision(cash, cashScale, quantity, quantityScale, 8)
	if errors.Is(err, errScaledDivisionOverflow) {
		// The trade is exact even when its unit quote cannot fit in the
		// price table's int64 coefficient at scale eight. Keep the trade and
		// leave this observation absent, as the previous best-effort writer did.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if priceValue <= 0 {
		return nil, nil
	}
	return &db.TradeImpliedPriceSpec{
		BaseCommodityID: baseID, QuoteCommodityID: quoteID, ValuationDate: date,
		PriceValue: priceValue, PriceScale: 8, Approximate: approximate,
	}, nil
}

func (s *InvestmentService) resolveCostBasisMethod(ctx context.Context, holdingAccountID int64, txnOverride string) (string, db.DisposalDecisionSource, error) {
	if txnOverride != "" {
		return txnOverride, db.DisposalDecisionSource{ResolutionTier: "transaction"}, nil
	}
	account, err := s.accountService.Account(ctx, holdingAccountID)
	if err == nil && account.CostBasisMethod != "" {
		return account.CostBasisMethod, db.DisposalDecisionSource{
			ResolutionTier: "account", AccountVersionID: account.VersionID,
			SourceEffectiveFrom: account.EffectiveFrom, SourceRecordedAt: account.UpdatedAt,
		}, nil
	}
	profile, err := s.repository.DefaultCostBasisProfile(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "fifo", db.DisposalDecisionSource{ResolutionTier: "fallback"}, nil
		}
		return "", db.DisposalDecisionSource{}, fmt.Errorf("resolve cost basis method: %w", err)
	}
	return profile.Method, db.DisposalDecisionSource{
		ResolutionTier: "global", ProfileID: profile.ID, ProfileVersionID: profile.VersionID,
		SourceRecordedAt: profile.UpdatedAt,
	}, nil
}

// simulateDisposal runs the complete sale/write-off writer — including
// backdated replay admission, checkpoint invalidation and gain comparison —
// and rolls back, so a preview reports exactly what the commit would do
// (T-117).
func (s *InvestmentService) simulateDisposal(ctx context.Context, input InvestmentTradeInput) (db.SimulatedInvestmentWrite, []db.LotDisposalRecord, error) {
	input.ReconciliationOverride = unboundOverride
	transactionParams, disposalParams, err := s.prepareSellWrite(ctx, input)
	if err != nil {
		return db.SimulatedInvestmentWrite{}, nil, err
	}
	transactionParams.GainImpact = gainImpactPolicy("")
	simulated, disposals, _, err := s.repository.SimulateTransactionAndDisposeLots(ctx, transactionParams, disposalParams)
	if err != nil {
		return db.SimulatedInvestmentWrite{}, nil, mapDisposalWriteError(err, input.WriteOff)
	}
	return simulated, disposals, nil
}

// mapDisposalWriteError translates a sale or write-off writer failure. A
// backdated disposal that makes a later decision impossible names it.
func mapDisposalWriteError(err error, writeOff bool) error {
	var dependency *db.InvestmentReplayDependencyError
	switch {
	case errors.As(err, &dependency):
		return InvestmentSaleDependencyError{OperationID: dependency.OperationID, DecisionID: dependency.DecisionID}
	case errors.Is(err, db.ErrInsufficientLots):
		return ErrInvestmentLotsInsufficient
	case errors.Is(err, db.ErrNotFound):
		return ValidationError{Message: "selected lot is not eligible for this disposal"}
	case errors.Is(err, db.ErrInvalidDisposalParams), errors.Is(err, db.ErrInvestmentBasisRange):
		return ValidationError{Message: err.Error()}
	case errors.Is(err, db.ErrOutOfOrderPositionEvent), errors.Is(err, db.ErrGainImpactAcknowledgementRequired),
		errors.Is(err, db.ErrGainImpactAcknowledgementStale):
		return err
	}
	if writeOff {
		return fmt.Errorf("dispose write-off lots: %w", mapTransactionDBError(err))
	}
	return fmt.Errorf("dispose sell lots: %w", mapTransactionDBError(err))
}

func (s *InvestmentService) PreviewSell(ctx context.Context, input InvestmentTradeInput) (SellPreviewResult, error) {
	if err := validateTradeInput(input); err != nil {
		return SellPreviewResult{}, err
	}
	if err := s.validateTradeRoles(ctx, input, nil); err != nil {
		return SellPreviewResult{}, err
	}
	economics, err := s.calculateTradeEconomics(ctx, input, false, newAccountRuleDependencies())
	if err != nil {
		return SellPreviewResult{}, err
	}
	method, source, err := s.resolveCostBasisMethod(ctx, input.HoldingAccountID, input.CostBasisMethod)
	if err != nil {
		return SellPreviewResult{}, err
	}
	input.WriteOff = false
	_, disposals, err := s.simulateDisposal(ctx, input)
	if err != nil {
		return SellPreviewResult{}, err
	}
	// Disposals can carry different CostBasisScales (lots opened at different
	// cash scales, e.g. via import); accumulate through exact.ScaledInt rather
	// than summing raw int64s at mismatched scales (same defect class as F5,
	// ListRealizedGains). Unknown basis leaves the gain unresolved, never a
	// gain against a partial or zero basis.
	disposedBasis, known := knownDisposedBasis(disposals)
	var gainValue int64
	gain := exact.NewScaledInt()
	if known {
		gain.AddScaled(exact.ScaledIntFromInt64(economics.ClearingValue, economics.ClearingScale))
		gain.SubScaled(disposedBasis)
		gainValue, err = gain.Int64()
		if err != nil {
			return SellPreviewResult{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
		}
	}
	decision, err := previewDisposalDecision(input, method, source, disposals)
	if err != nil {
		return SellPreviewResult{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
	}
	decision.ProceedsValue = economics.ClearingValue
	decision.ProceedsScale = economics.ClearingScale
	return SellPreviewResult{
		CostBasisMethod:    method,
		DisposalDecision:   decision,
		Allocations:        toInvestmentLotDisposals(disposals),
		RealizedGain:       gainValue,
		RealizedGainScale:  gain.Scale(),
		BasisKnowledge:     basisKnowledgeCode(known),
		CashAmountValue:    input.CashAmountValue,
		CashAmountScale:    input.CashAmountScale,
		GrossAmountValue:   input.GrossAmountValue,
		GrossAmountScale:   input.GrossAmountScale,
		NetSettlementValue: economics.NetValue,
		NetSettlementScale: economics.NetScale,
		SettlementDate:     economics.SettlementDate,
		Charges:            input.Charges,
	}, nil
}

func (s *InvestmentService) Sell(ctx context.Context, input InvestmentTradeInput) (InvestmentTradeResult, error) {
	input.WriteOff = false
	result, err := s.sell(ctx, input, nil)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	result.Transaction, err = s.transactionService.enrichOne(ctx, result.Transaction)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	return result, nil
}

// writeOffTrade is the shared disposal engine a write-off runs through: the
// lots are closed exactly as a sale closes them, and the zero-proceeds
// disposal decision gives the disposed basis as a loss (T-38).
func (s *InvestmentService) writeOffTrade(ctx context.Context, input InvestmentTradeInput) (InvestmentTradeResult, error) {
	input.WriteOff = true
	input.CashAccountID = 0
	input.CashCommodityID = 0
	input.CashAmountScale = 0
	if input.Operation == "" {
		input.Operation = "investment.write_off"
	}
	if strings.TrimSpace(input.ChangeReason) == "" {
		return InvestmentTradeResult{}, ValidationError{Message: "write-off reason is required"}
	}
	return s.sell(ctx, input, nil)
}

// WriteOff records a total loss on a holding: a fund closure, a worthless
// delisting, an issuer liquidated. There is no cash account or amount —
// proceeds are zero by definition, which is what separates it from a sale
// (T-38). A dedicated input type keeps that impossible to get wrong at the
// contract level, unlike a sell whose amount was merely left at zero.
//
// Reason is required and distinct from ChangeReason: "why is this holding
// worthless" is an auditable fact about the position, not metadata about the
// edit.
func (s *InvestmentService) WriteOff(ctx context.Context, input InvestmentWriteOffInput) (InvestmentTradeResult, error) {
	reason, err := validateWriteOffInput(input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	trade := input.asTradeInput()
	trade.ChangeReason = defaultString(input.ChangeReason, "wrote off worthless holding: "+reason)
	trade.OriginType = input.OriginType
	trade.Operation = input.Operation
	trade.Memo = defaultString(input.Memo, reason)
	trade.ExternalRefHint = input.ExternalRefHint
	trade.MetadataJSON = input.MetadataJSON
	trade.PayeeID = input.PayeeID
	trade.Status = input.Status
	trade.ReconciliationOverride = input.ReconciliationOverride
	trade.GainImpactAcknowledgement = input.GainImpactAcknowledgement
	result, err := s.writeOffTrade(ctx, trade)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	result.Transaction, err = s.transactionService.enrichOne(ctx, result.Transaction)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	return result, nil
}

// PreviewWriteOff reports which lots a write-off would consume and the loss
// it would realize, without writing anything.
func (s *InvestmentService) PreviewWriteOff(ctx context.Context, input InvestmentWriteOffInput) (SellPreviewResult, error) {
	if _, err := validateWriteOffInput(input); err != nil {
		return SellPreviewResult{}, err
	}
	// The commit path reaches these through sellPlan, and a preview that
	// accepts what the commit will refuse is worse than no preview (T-98).
	if err := s.validateTradeRoles(ctx, input.asTradeInput(), nil); err != nil {
		return SellPreviewResult{}, err
	}
	method, source, err := s.resolveCostBasisMethod(ctx, input.HoldingAccountID, input.CostBasisMethod)
	if err != nil {
		return SellPreviewResult{}, err
	}
	_, disposals, err := s.simulateDisposal(ctx, input.asTradeInput())
	if err != nil {
		return SellPreviewResult{}, err
	}
	disposedBasis, known := knownDisposedBasis(disposals)
	gain := exact.NewScaledInt()
	var gainValue int64
	if known {
		gain.SubScaled(disposedBasis)
		gainValue, err = gain.Int64()
		if err != nil {
			return SellPreviewResult{}, LedgerOverflowError{CommodityID: input.CommodityID}
		}
	}
	decision, err := previewDisposalDecision(input.asTradeInput(), method, source, disposals)
	if err != nil {
		return SellPreviewResult{}, LedgerOverflowError{CommodityID: input.CommodityID}
	}
	return SellPreviewResult{
		CostBasisMethod:   method,
		DisposalDecision:  decision,
		Allocations:       toInvestmentLotDisposals(disposals),
		RealizedGain:      gainValue,
		RealizedGainScale: gain.Scale(),
		BasisKnowledge:    basisKnowledgeCode(known),
		CashAmountValue:   0,
		CashAmountScale:   0,
	}, nil
}

// WriteOffReconciliationImpact is TradeReconciliationImpact for a write-off:
// the active checkpoints it would invalidate, without persisting anything
// (T-53).
func (s *InvestmentService) WriteOffReconciliationImpact(ctx context.Context, input InvestmentWriteOffInput) (ReconciliationImpact, error) {
	if _, err := validateWriteOffInput(input); err != nil {
		return ReconciliationImpact{}, err
	}
	trade := input.asTradeInput()
	trade.ChangeReason = input.ChangeReason
	trade.OriginType = input.OriginType
	trade.Operation = input.Operation
	trade.Memo = input.Memo
	trade.ExternalRefHint = input.ExternalRefHint
	trade.MetadataJSON = input.MetadataJSON
	trade.PayeeID = input.PayeeID
	trade.Status = input.Status
	trade.WriteOff = true
	simulated, _, err := s.simulateDisposal(ctx, trade)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

// sellWithPostWrite is the import-side variant of Sell. Its callback runs
// before the transaction and lot-disposal writes commit.
func (s *InvestmentService) sellWithPostWrite(ctx context.Context, input InvestmentTradeInput, postWrite func(*sql.Tx, int64) error) (InvestmentTradeResult, error) {
	return s.sell(ctx, input, postWrite)
}

func (s *InvestmentService) sellPlan(ctx context.Context, input InvestmentTradeInput) (investmentTransactionPlan, error) {
	if err := validateTradeInput(input); err != nil {
		return investmentTransactionPlan{}, err
	}
	dependencies := newAccountRuleDependencies()
	if err := s.validateTradeRoles(ctx, input, dependencies); err != nil {
		return investmentTransactionPlan{}, err
	}
	var economics *tradeEconomics
	if !input.WriteOff {
		computed, err := s.calculateTradeEconomics(ctx, input, false, dependencies)
		if err != nil {
			return investmentTransactionPlan{}, err
		}
		economics = &computed
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return investmentTransactionPlan{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	metadataJSON, err := cleanSizedJSONObject(input.MetadataJSON, "metadata", investmentJSONMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	return investmentTransactionPlan{
		TradeEconomics:          economics,
		AccountRuleDependencies: dependencies,
		MetadataJSON:            metadataJSON,
		Date:                    input.TransactionDate,
		Create: CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             defaultString(input.OriginType, "browser_api"),
			Operation:              defaultString(input.Operation, "investment.sell"),
			ChangeReason:           input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
			Spec: TransactionInput{
				Status:                  status,
				TransactionKind:         "investment",
				InvestmentOperationKind: disposalOperationKind(input),
				TransactionDate:         input.TransactionDate,
				PayeeID:                 input.PayeeID,
				Description:             memo,
				ExternalRefHint:         input.ExternalRefHint,
				MetadataJSON:            metadataJSON,
				JournalEntries:          tradeJournalEntriesForDisposal(input, tradingAccountID, memo, economics),
			},
		},
	}, nil
}

func (s *InvestmentService) sell(ctx context.Context, input InvestmentTradeInput, postWrite func(*sql.Tx, int64) error) (InvestmentTradeResult, error) {
	transactionParams, disposalParams, err := s.prepareSellWrite(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	// A disposal dated behind a later one replays it (T-117); any change to a
	// committed gain needs the preview's acknowledgement.
	transactionParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	var transactionRecord db.TransactionRecord
	var disposals []db.LotDisposalRecord
	var decision db.DisposalDecisionRecord
	if postWrite == nil {
		transactionRecord, disposals, decision, err = s.repository.CreateTransactionAndDisposeLotsWithDecision(ctx, transactionParams, disposalParams)
	} else {
		transactionRecord, disposals, decision, err = s.repository.CreateTransactionAndDisposeLotsWithDecisionAndPostWrite(ctx, transactionParams, disposalParams, postWrite)
	}
	if err != nil {
		return InvestmentTradeResult{}, mapDisposalWriteError(err, input.WriteOff)
	}
	transaction := toTransaction(transactionRecord)
	committedDecision := toDisposalDecision(decision)
	return InvestmentTradeResult{Transaction: transaction, Allocations: toInvestmentLotDisposals(disposals), DisposalDecision: &committedDecision}, nil
}

// prepareSellWrite freezes the exact journal and disposal election before a
// write. Ordinary sale entry and native replacement must use identical source
// economics, account dependencies, fee snapshots, and cost-basis provenance.
func (s *InvestmentService) prepareSellWrite(ctx context.Context, input InvestmentTradeInput) (db.CreateTransactionParams, db.DisposeLotsParams, error) {
	plan, err := s.sellPlan(ctx, input)
	if err != nil {
		return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
	}
	metadataJSON := plan.MetadataJSON
	transactionParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
	}
	if !input.WriteOff {
		transactionParams.InvestmentComponents = plan.TradeEconomics.components(input.CashCommodityID)
		transactionParams.InvestmentSettlementDate = plan.TradeEconomics.SettlementDate
		if s.pricingService != nil && !plan.TradeEconomics.PriceUnavailable {
			transactionParams.TradeImpliedPrice, err = tradePriceSpec(input.CommodityID, input.CashCommodityID,
				input.TransactionDate, input.QuantityValue, input.QuantityScale,
				plan.TradeEconomics.PriceValue, plan.TradeEconomics.PriceScale, plan.TradeEconomics.PriceApproximate)
			if err != nil {
				return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
			}
		}
	}
	method, source, err := s.resolveCostBasisMethod(ctx, input.HoldingAccountID, input.CostBasisMethod)
	if err != nil {
		return db.CreateTransactionParams{}, db.DisposeLotsParams{}, err
	}
	allocations := make([]db.LotAllocation, 0, len(input.LotAllocations))
	for _, allocation := range input.LotAllocations {
		allocations = append(allocations, db.LotAllocation{LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	disposalParams := db.DisposeLotsParams{
		BookID:          BookID,
		AccountID:       input.HoldingAccountID,
		CommodityID:     input.CommodityID,
		CostCommodityID: input.CashCommodityID,
		ProceedsValue:   0,
		ProceedsScale:   0,
		EventDate:       input.TransactionDate,
		QuantityValue:   input.QuantityValue,
		QuantityScale:   input.QuantityScale,
		Allocations:     allocations,
		CostBasisMethod: method,
		DecisionSource:  source,
		// Sales, write-offs and cash in lieu preserve unresolved results (#169).
		AdmitUnknownBasis: true,
		CreatedAt:         s.now().UTC().Format(time.RFC3339),
		ActorUserID:       input.OwnerUserID,
		AuthSessionID:     input.AuthSessionID,
		RequestID:         input.RequestID,
		OriginType:        defaultString(input.OriginType, "browser_api"),
		Operation:         "investment.lot.dispose",
		ChangeReason:      disposalChangeReason(input.WriteOff),
		MetadataJSON:      metadataJSON,
	}
	if input.WriteOff {
		disposalParams.CostCommodityID = input.CostCommodityID
	}
	if plan.TradeEconomics != nil {
		disposalParams.ProceedsValue = plan.TradeEconomics.ClearingValue
		disposalParams.ProceedsScale = plan.TradeEconomics.ClearingScale
	}
	return transactionParams, disposalParams, nil
}

func (s *InvestmentService) Dividend(ctx context.Context, input DividendInput) (Transaction, error) {
	return s.dividend(ctx, input, nil)
}

// dividendWithPostWrite keeps an imported dividend's import identity in the
// same transaction as its ledger postings.
func (s *InvestmentService) dividendWithPostWrite(ctx context.Context, input DividendInput, postWrite func(*sql.Tx, int64) error) (Transaction, error) {
	return s.dividend(ctx, input, postWrite)
}

func (s *InvestmentService) dividendPlan(ctx context.Context, input DividendInput) (investmentTransactionPlan, error) {
	date, err := validateDividendInput(input)
	if err != nil {
		return investmentTransactionPlan{}, err
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
			return investmentTransactionPlan{}, fmt.Errorf("resolve dividend default: %w", err)
		}
	}
	incomeAccountID := int64(0)
	if input.IncomeAccountID != nil {
		incomeAccountID = *input.IncomeAccountID
	}
	if !input.WithholdingOnly && incomeAccountID <= 0 {
		return investmentTransactionPlan{}, ValidationError{Message: "dividend income account is required"}
	}
	dependencies := newAccountRuleDependencies()
	if err := s.validateDividendRoles(ctx, input, date, incomeAccountID, dependencies); err != nil {
		return investmentTransactionPlan{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	var postings []PostingInput
	operation := "investment.dividend"
	if input.WithholdingOnly {
		operation = "investment.dividend.withholding"
		postings = []PostingInput{
			{AccountID: input.CashAccountID, QuantityValue: exact.New(-input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
			{AccountID: *input.WithholdingAccountID, QuantityValue: exact.New(input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
		}
	} else {
		postings = []PostingInput{
			{AccountID: input.CashAccountID, QuantityValue: exact.New(input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
			{AccountID: incomeAccountID, QuantityValue: exact.New(-input.AmountValue), QuantityScale: input.AmountScale, CommodityID: input.CashCommodityID, Memo: memo},
		}
	}
	if input.WithholdingValue != nil && *input.WithholdingValue > 0 {
		if input.WithholdingAccountID == nil || *input.WithholdingAccountID <= 0 {
			return investmentTransactionPlan{}, ValidationError{Message: "withholding account is required"}
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
	return investmentTransactionPlan{
		AccountRuleDependencies: dependencies,
		MetadataJSON:            input.MetadataJSON,
		Date:                    date,
		Create: CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             defaultString(input.OriginType, "browser_api"),
			Operation:              defaultString(input.Operation, operation),
			ChangeReason:           input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
			Spec: TransactionInput{
				Status:                  status,
				TransactionKind:         "investment",
				InvestmentOperationKind: "dividend",
				NeedsReview:             input.NeedsReview,
				TransactionDate:         date,
				PayeeID:                 input.PayeeID,
				PayeeName:               input.PayeeName,
				TagIDs:                  input.TagIDs,
				Description:             memo,
				ExternalRefHint:         input.ExternalRefHint,
				MetadataJSON:            input.MetadataJSON,
				JournalEntries: []JournalEntryInput{{
					EntryDate: date,
					EntryKind: "investment",
					Memo:      memo,
					Postings:  postings,
				}},
			},
		},
	}, nil
}

func (s *InvestmentService) dividend(ctx context.Context, input DividendInput, postWrite func(*sql.Tx, int64) error) (Transaction, error) {
	params, err := s.prepareDividendWrite(ctx, input)
	if err != nil {
		return Transaction{}, err
	}
	record, err := s.transactionService.repository.CreateTransactionWithPostWrite(ctx, params, postWrite)
	if err != nil {
		return Transaction{}, mapTransactionDBError(err)
	}
	return s.transactionService.enrichOne(ctx, toTransaction(record))
}

// prepareDividendWrite freezes the journal and its gross and withholding
// components. Ordinary entry and replacement (T-115) share it.
func (s *InvestmentService) prepareDividendWrite(ctx context.Context, input DividendInput) (db.CreateTransactionParams, error) {
	plan, err := s.dividendPlan(ctx, input)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	createInput := plan.Create
	// A cash dividend touches cash, income and withholding only — it creates no
	// lot — so it goes through the ordinary preparation. Handing it the
	// subledger exemption just because it is an investment command would let an
	// imported dividend post to a holding account with nothing to account for
	// the shares (T-98).
	params, err := s.transactionService.prepareCreateTransactionForWriteCarrying(ctx, createInput, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, err
	}
	params.DividendCommodityID = nullableInt64(input.CommodityID)
	params.DividendWithholdingOnly = input.WithholdingOnly
	params.InvestmentComponents = []db.InvestmentComponentSpec{{
		Kind: "dividend_gross", CommodityID: input.CashCommodityID,
		AmountValue: strconv.FormatInt(input.AmountValue, 10), AmountScale: input.AmountScale,
		AmountDate: plan.Date,
	}}
	if input.WithholdingOnly {
		params.InvestmentComponents[0].Kind = "withholding"
		params.InvestmentComponents[0].AmountValue = strconv.FormatInt(-input.AmountValue, 10)
	}
	if input.WithholdingValue != nil && *input.WithholdingValue > 0 {
		scale := input.AmountScale
		if input.WithholdingScale != nil {
			scale = *input.WithholdingScale
		}
		params.InvestmentComponents = append(params.InvestmentComponents, db.InvestmentComponentSpec{
			Kind: "withholding", CommodityID: input.CashCommodityID,
			AmountValue: strconv.FormatInt(-*input.WithholdingValue, 10), AmountScale: scale,
			AmountDate: plan.Date,
		})
	}
	return params, nil
}

func (s *InvestmentService) reinvestedDividendPlan(ctx context.Context, input ReinvestedDividendInput) (investmentTransactionPlan, error) {
	return s.reinvestedDividendPlanAs(ctx, input, "investment.reinvested_dividend")
}

// reinvestedDividendPlanAs plans the journal under a command's own audit
// operation code, so a replacement records investment.reinvested_dividend.replace.
func (s *InvestmentService) reinvestedDividendPlanAs(ctx context.Context, input ReinvestedDividendInput, operationCode string) (investmentTransactionPlan, error) {
	date, err := validateReinvestedDividendInput(input)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	incomeAccountID := int64(0)
	if input.IncomeAccountID != nil {
		incomeAccountID = *input.IncomeAccountID
	} else {
		defaultRecord, err := s.repository.ResolveDividendDefault(ctx, BookID, input.CommodityID, date)
		if err == nil {
			incomeAccountID = defaultRecord.IncomeAccountID
		} else if !errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, fmt.Errorf("resolve dividend default: %w", err)
		}
	}
	if incomeAccountID <= 0 {
		return investmentTransactionPlan{}, ValidationError{Message: "dividend income account is required"}
	}
	dependencies := newAccountRuleDependencies()
	if err := s.validateReinvestedDividendRoles(ctx, input, date, incomeAccountID, dependencies); err != nil {
		return investmentTransactionPlan{}, err
	}
	tradingAccountID, err := s.repository.CommodityTradingAccountID(ctx, BookID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return investmentTransactionPlan{}, ValidationError{Message: "commodity trading system account is required"}
		}
		return investmentTransactionPlan{}, err
	}
	memo, err := cleanOptionalText(input.Memo, "memo", investmentTextMaxBytes)
	if err != nil {
		return investmentTransactionPlan{}, err
	}
	status := input.Status
	if strings.TrimSpace(status) == "" {
		status = "posted"
	}
	return investmentTransactionPlan{
		AccountRuleDependencies: dependencies,
		Date:                    date,
		Create: CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             "browser_api",
			Operation:              operationCode,
			ChangeReason:           input.ChangeReason,
			ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
			Spec: TransactionInput{
				Status:                  status,
				TransactionKind:         "investment",
				InvestmentOperationKind: "reinvested_dividend",
				TransactionDate:         date,
				PayeeID:                 input.PayeeID,
				Description:             memo,
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
		},
	}, nil
}

func (s *InvestmentService) ReinvestedDividend(ctx context.Context, input ReinvestedDividendInput) (InvestmentTradeResult, error) {
	transactionParams, lotParams, err := s.prepareReinvestmentWrite(ctx, input)
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	transactionParams.GainImpact = gainImpactPolicy(input.GainImpactAcknowledgement)
	transactionRecord, lot, err := s.repository.CreateTransactionAndLot(ctx, transactionParams, lotParams)
	if err != nil {
		return InvestmentTradeResult{}, mapInvestmentOpeningWriteError(err, "reinvested dividend")
	}
	transaction, err := s.transactionService.enrichOne(ctx, toTransaction(transactionRecord))
	if err != nil {
		return InvestmentTradeResult{}, err
	}
	return InvestmentTradeResult{Transaction: transaction, LotID: &lot.ID}, nil
}

func (s *InvestmentService) prepareReinvestmentWrite(ctx context.Context, input ReinvestedDividendInput) (db.CreateTransactionParams, db.CreateInvestmentLotParams, error) {
	return s.prepareReinvestmentWriteAs(ctx, input, "investment.reinvested_dividend")
}

// prepareReinvestmentWriteAs freezes the journal, income component, trade
// price and opening lot. Ordinary entry and replacement share it.
func (s *InvestmentService) prepareReinvestmentWriteAs(ctx context.Context, input ReinvestedDividendInput, operationCode string) (db.CreateTransactionParams, db.CreateInvestmentLotParams, error) {
	plan, err := s.reinvestedDividendPlanAs(ctx, input, operationCode)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
	}
	date := plan.Date
	transactionParams, err := s.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	if err != nil {
		return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
	}
	transactionParams.InvestmentComponents = []db.InvestmentComponentSpec{{
		Kind: "reinvested_distribution", CommodityID: input.CashCommodityID,
		AmountValue: strconv.FormatInt(input.AmountValue, 10), AmountScale: input.AmountScale,
		AmountDate: date,
	}}
	if s.pricingService != nil {
		transactionParams.TradeImpliedPrice, err = tradePriceSpec(input.CommodityID, input.CashCommodityID,
			date, input.QuantityValue, input.QuantityScale, input.AmountValue, input.AmountScale, false)
		if err != nil {
			return db.CreateTransactionParams{}, db.CreateInvestmentLotParams{}, err
		}
	}
	return transactionParams, db.CreateInvestmentLotParams{
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
	}, nil
}

func (s *InvestmentService) ListLots(ctx context.Context, accountID int64, commodityID int64) ([]InvestmentLot, error) {
	records, err := s.repository.ListLots(ctx, BookID, accountID, commodityID)
	if err != nil {
		return nil, fmt.Errorf("list investment lots: %w", err)
	}
	return toInvestmentLots(records), nil
}

func (s *InvestmentService) Positions(ctx context.Context) ([]InvestmentPosition, error) {
	boundary, err := s.repository.CurrentObservationBoundary(ctx, BookID, s.now())
	if err != nil {
		return nil, err
	}
	records, err := s.repository.Positions(ctx, BookID, boundary)
	if err != nil {
		return nil, fmt.Errorf("read investment positions: %w", err)
	}
	positions := toInvestmentPositions(records)
	defaults := make(map[int64]string)
	for index, record := range records {
		method, cached := defaults[record.AccountID]
		if !cached {
			if method, _, err = s.resolveCostBasisMethod(ctx, record.AccountID, ""); err != nil {
				return nil, err
			}
			defaults[record.AccountID] = method
		}
		positions[index].TransferBasisAllocation = db.InternalTransferAllocation(record.MethodFamily, method)
	}
	return positions, nil
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
	suggestions := toInvestmentEventSuggestions(records)
	candidates, err := s.repository.EventLinkCandidates(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list investment event link candidates: %w", err)
	}
	bySuggestion := map[int64][]InvestmentEventLinkCandidate{}
	for _, candidate := range candidates {
		normalized := candidate.Gross.Normalized()
		value, err := normalized.Coefficient()
		if err != nil {
			return nil, fmt.Errorf("link candidate gross: %w", err)
		}
		bySuggestion[candidate.SuggestionID] = append(bySuggestion[candidate.SuggestionID], InvestmentEventLinkCandidate{
			TransactionID: candidate.TransactionID, TransactionDate: candidate.TransactionDate, CashCode: candidate.CashCode,
			GrossValue: value, GrossScale: normalized.Scale(), HoldingAccountID: candidate.HoldingAccountID, HoldingAccountName: candidate.HoldingAccountName})
	}
	revisions, err := s.repository.ProviderEventRevisions(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list investment provider event revisions: %w", err)
	}
	byEvent := map[int64][]InvestmentEventRevision{}
	for _, revision := range revisions {
		byEvent[revision.ProviderEventID] = append(byEvent[revision.ProviderEventID], InvestmentEventRevision{EvidenceJSON: revision.NormalizedJSON, ObservedAt: revision.ObservedAt})
	}
	for index := range suggestions {
		suggestions[index].LinkCandidates = bySuggestion[suggestions[index].ID]
		suggestions[index].Revisions = byEvent[suggestions[index].ProviderEventID]
	}
	return suggestions, nil
}

// AcceptSuggestion posts a complete proposal unchanged; see
// AcceptSuggestionWithCompletion.
func (s *InvestmentService) AcceptSuggestion(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, suggestionID int64) (InvestmentEventSuggestion, error) {
	return s.AcceptSuggestionWithCompletion(ctx, ownerUserID, authSessionID, requestID, suggestionID, nil)
}

// AcceptSuggestionWithCompletion parses the suggestion's
// proposed_transaction_json and posts it through the native dividend command,
// recording the result on the suggestion and its provider event in the same
// DB transaction as the post (see MarkSuggestionAcceptedInTx).
//
// A complete proposal that is malformed, unsupported, or refused downstream
// moves to 'failed' with a reason and returns with a nil error — the
// suggestion's own state reflects the failure. An incomplete provider
// proposal (#111) needs the owner's completion; a missing or invalid
// completion is returned as a ValidationError and the suggestion stays
// pending so the owner can correct it. Informational events cannot be
// accepted at all. Only infra failures and non-pending suggestions are
// returned as other errors.
func (s *InvestmentService) AcceptSuggestionWithCompletion(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, suggestionID int64, completion *SuggestionCompletion) (InvestmentEventSuggestion, error) {
	if ownerUserID <= 0 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "owner user is required"}
	}
	if suggestionID <= 0 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "suggestion id is required"}
	}

	record, err := s.repository.EventSuggestionByID(ctx, BookID, suggestionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotFound
		}
		return InvestmentEventSuggestion{}, fmt.Errorf("read investment event suggestion: %w", err)
	}
	if record.Status != "suggested" {
		return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
	}

	now := s.now().UTC().Format(time.RFC3339)
	if record.EventFamily == "return_of_capital" || record.EventFamily == "cash_in_lieu" {
		return s.failSuggestion(ctx, suggestionID, "return of capital or cash in lieu requires an investment basis workflow", now, ownerUserID, authSessionID, requestID)
	}

	if pil, ok := parseProposedPaymentInLieu(record.ProposedTransactionJSON); ok {
		return s.acceptPaymentInLieuSuggestion(ctx, record, pil, completion, now, ownerUserID, authSessionID, requestID)
	}

	proposal, ownerCompleted, err := resolveProposedDividendTransaction(record.ProposedTransactionJSON, completion)
	if err != nil {
		if validationErr, ok := errors.AsType[ValidationError](err); ok {
			return InvestmentEventSuggestion{}, validationErr
		}
		return s.failSuggestion(ctx, suggestionID, err.Error(), now, ownerUserID, authSessionID, requestID)
	}
	// Only an owner who completed the proposal can have reviewed its impact.
	var override CheckpointOverride
	if ownerCompleted {
		override = completion.ReconciliationOverride
	}

	commodityID := proposal.CommodityID
	if commodityID == nil && record.InstrumentID.Valid {
		instrument, instrErr := s.Instrument(ctx, record.InstrumentID.Int64)
		if instrErr != nil {
			return s.failSuggestion(ctx, suggestionID, fmt.Sprintf("resolve instrument commodity: %v", instrErr), now, ownerUserID, authSessionID, requestID)
		}
		commodityID = &instrument.CommodityID
	}

	dividendInput := DividendInput{
		OwnerUserID:            ownerUserID,
		AuthSessionID:          authSessionID,
		RequestID:              requestID,
		TransactionDate:        proposal.TransactionDate,
		CommodityID:            commodityID,
		CashAccountID:          proposal.CashAccountID,
		CashCommodityID:        proposal.CashCommodityID,
		IncomeAccountID:        proposal.IncomeAccountID,
		AmountValue:            proposal.AmountValue,
		AmountScale:            proposal.AmountScale,
		WithholdingValue:       proposal.WithholdingValue,
		WithholdingScale:       proposal.WithholdingScale,
		WithholdingAccountID:   proposal.WithholdingAccountID,
		Memo:                   proposal.Memo,
		ExternalRefHint:        proposal.ExternalRefHint,
		PayeeID:                proposal.PayeeID,
		ReconciliationOverride: override,
		ChangeReason:           "accepted provider event suggestion",
		Operation:              "investment.event_suggestion.accept",
	}
	_, err = s.dividendWithPostWrite(ctx, dividendInput, func(tx *sql.Tx, transactionID int64) error {
		return s.repository.MarkSuggestionAcceptedInTx(ctx, tx, BookID, suggestionID, transactionID, now, ownerUserID, authSessionID, requestID)
	})
	if err != nil {
		if errors.Is(err, db.ErrEventSuggestionNotPending) {
			return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
		}
		if ownerCompleted {
			// The owner's facts were refused (or need the reconciliation
			// override); nothing posted and the suggestion stays reviewable.
			return InvestmentEventSuggestion{}, err
		}
		if validationErr, ok := errors.AsType[ValidationError](err); ok {
			return s.failSuggestion(ctx, suggestionID, validationErr.Error(), now, ownerUserID, authSessionID, requestID)
		}
		return InvestmentEventSuggestion{}, fmt.Errorf("post accepted suggestion transaction: %w", err)
	}

	accepted, err := s.repository.EventSuggestionByID(ctx, BookID, suggestionID)
	if err != nil {
		return InvestmentEventSuggestion{}, fmt.Errorf("reload accepted investment event suggestion: %w", err)
	}
	return toInvestmentEventSuggestion(accepted), nil
}

// LinkSuggestion records that an owner-recorded cash dividend, or for a
// short-only suggestion an owner-recorded payment in lieu (#258), is this
// provider dividend, so nothing posts and later fetches stay duplicates.
func (s *InvestmentService) LinkSuggestion(ctx context.Context, ownerUserID int64, authSessionID int64, requestID string, suggestionID int64, transactionID int64) (InvestmentEventSuggestion, error) {
	if ownerUserID <= 0 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "owner user is required"}
	}
	if suggestionID <= 0 || transactionID <= 0 {
		return InvestmentEventSuggestion{}, ValidationError{Message: "suggestion id and transaction id are required"}
	}
	record, err := s.repository.LinkSuggestion(ctx, db.LinkSuggestionParams{BookID: BookID, SuggestionID: suggestionID, TransactionID: transactionID,
		ActorUserID: ownerUserID, AuthSessionID: authSessionID, RequestID: requestID, RecordedAt: s.now().UTC().Format(time.RFC3339)})
	switch {
	case errors.Is(err, db.ErrNotFound):
		return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotFound
	case errors.Is(err, db.ErrEventSuggestionNotPending):
		return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
	case errors.Is(err, db.ErrEventLinkTarget):
		return InvestmentEventSuggestion{}, ValidationError{Message: "only an unlinked posted cash dividend of this instrument dated on or after the ex-date, or for a short holder an unlinked payment in lieu of this instrument, holding and ex-date, can be linked"}
	case err != nil:
		return InvestmentEventSuggestion{}, fmt.Errorf("link investment event suggestion: %w", err)
	}
	return toInvestmentEventSuggestion(record), nil
}

func (s *InvestmentService) failSuggestion(ctx context.Context, suggestionID int64, reason string, now string, ownerUserID int64, authSessionID int64, requestID string) (InvestmentEventSuggestion, error) {
	failed, err := s.repository.MarkSuggestionFailed(ctx, BookID, suggestionID, reason, now, ownerUserID, authSessionID, requestID)
	if err != nil {
		if errors.Is(err, db.ErrEventSuggestionNotPending) {
			return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
		}
		return InvestmentEventSuggestion{}, fmt.Errorf("mark investment event suggestion failed: %w", err)
	}
	return toInvestmentEventSuggestion(failed), nil
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
		if errors.Is(err, db.ErrEventSuggestionNotPending) {
			return InvestmentEventSuggestion{}, ErrInvestmentSuggestionNotPending
		}
		return InvestmentEventSuggestion{}, fmt.Errorf("update investment suggestion: %w", err)
	}
	return toInvestmentEventSuggestion(record), nil
}

// proposedTransactionKindDividendIncome is the only proposed_transaction_json
// "kind" AcceptSuggestion knows how to post. It covers the cash-income-shaped
// event families (dividend, distribution) —
// these reduce to "cash in, income account credited, optional withholding",
// exactly DividendInput's shape. Structural actions, return of capital and
// cash in lieu cannot use this dividend proposal. Native split, exchange,
// merger, spin-off and basis-action commands ship, but this suggestion path
// has no mapping to those writers. Reject unsupported proposals rather than
// treating them as income; see the R16 operation contract and
// provider-import boundaries in docs/implemented.md.
const proposedTransactionKindDividendIncome = "dividend_income"

// proposedDividendTransaction is the contract a provider-event producer writes
// into investment_event_suggestions.proposed_transaction_json. The Twelve Data
// dividend producer (#111) writes complete=false and leaves the payment facts
// to the owner's SuggestionCompletion; an "informational" kind never posts.
type proposedDividendTransaction struct {
	Kind string `json:"kind"`
	// Complete is false for a provider proposal that still needs the owner's
	// facts (#111); absent means the proposal is complete as written.
	Complete             *bool  `json:"complete,omitempty"`
	TransactionDate      string `json:"transaction_date"`
	CashAccountID        int64  `json:"cash_account_id"`
	CashCommodityID      int64  `json:"cash_commodity_id"`
	AmountValue          int64  `json:"amount_value"`
	AmountScale          int    `json:"amount_scale"`
	CommodityID          *int64 `json:"commodity_id,omitempty"`
	IncomeAccountID      *int64 `json:"income_account_id,omitempty"`
	WithholdingAccountID *int64 `json:"withholding_account_id,omitempty"`
	WithholdingValue     *int64 `json:"withholding_value,omitempty"`
	WithholdingScale     *int   `json:"withholding_scale,omitempty"`
	Memo                 string `json:"memo,omitempty"`
	ExternalRefHint      string `json:"external_ref_hint,omitempty"`
	PayeeID              *int64 `json:"payee_id,omitempty"`
}

// resolveProposedDividendTransaction merges the owner's completion into an
// incomplete provider proposal. ownerCompleted reports whether the posted
// facts came from the owner, whose refusals are returned rather than recorded
// as a failed suggestion.
func resolveProposedDividendTransaction(raw string, completion *SuggestionCompletion) (proposedDividendTransaction, bool, error) {
	var kind struct {
		Kind   string `json:"kind"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &kind); err == nil && kind.Kind == proposedTransactionKindInfoRow {
		return proposedDividendTransaction{}, false, ValidationError{Message: "this provider event is informational and cannot be posted; record it manually or ignore it"}
	}
	var proposal proposedDividendTransaction
	if err := json.Unmarshal([]byte(raw), &proposal); err == nil && proposal.Kind == proposedTransactionKindDividendIncome && proposal.Complete != nil && !*proposal.Complete {
		if completion == nil {
			return proposedDividendTransaction{}, false, ValidationError{Message: "this provider dividend is incomplete: supply the payment date, cash account and received gross amount"}
		}
		proposal.TransactionDate = completion.TransactionDate
		proposal.CashAccountID = completion.CashAccountID
		proposal.CashCommodityID = completion.CashCommodityID
		proposal.AmountValue, proposal.AmountScale = completion.AmountValue, completion.AmountScale
		proposal.IncomeAccountID = completion.IncomeAccountID
		proposal.WithholdingValue, proposal.WithholdingScale = completion.WithholdingValue, completion.WithholdingScale
		proposal.WithholdingAccountID = completion.WithholdingAccountID
		proposal.Memo = completion.Memo
		if err := validateProposedDividendTransaction(proposal); err != nil {
			return proposedDividendTransaction{}, true, ValidationError{Message: err.Error()}
		}
		return proposal, true, nil
	}
	if completion != nil {
		return proposedDividendTransaction{}, false, ValidationError{Message: "this suggestion is already complete and takes no owner completion"}
	}
	proposal, err := parseProposedDividendTransaction(raw)
	return proposal, false, err
}

func parseProposedDividendTransaction(raw string) (proposedDividendTransaction, error) {
	var proposal proposedDividendTransaction
	if err := json.Unmarshal([]byte(raw), &proposal); err != nil {
		return proposedDividendTransaction{}, fmt.Errorf("proposed transaction is not valid JSON: %w", err)
	}
	return proposal, validateProposedDividendTransaction(proposal)
}

func validateProposedDividendTransaction(proposal proposedDividendTransaction) error {
	if proposal.Kind != proposedTransactionKindDividendIncome {
		return fmt.Errorf("unsupported proposed transaction kind %q", proposal.Kind)
	}
	if strings.TrimSpace(proposal.TransactionDate) == "" {
		return fmt.Errorf("proposed transaction date is required")
	}
	if proposal.CashAccountID <= 0 {
		return fmt.Errorf("proposed cash account is required")
	}
	if proposal.CashCommodityID <= 0 {
		return fmt.Errorf("proposed cash commodity is required")
	}
	if proposal.AmountValue <= 0 {
		return fmt.Errorf("proposed amount is required")
	}
	if proposal.AmountScale < 0 {
		return fmt.Errorf("proposed amount scale is invalid")
	}
	return nil
}

func (s *InvestmentService) ListAutomationRules(ctx context.Context) ([]InvestmentAutomationRule, error) {
	records, err := s.repository.ListAutomationRules(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list automation rules: %w", err)
	}
	return toInvestmentAutomationRules(records), nil
}

// SaveAutomationRules replaces the book's full active automation-rule set
// with input.Rules: every rule given is created or updated, and any
// currently-active rule not among them is archived in the same transaction.
// An empty Rules list is a legitimate "archive everything" request, not an
// error — that's the correct reading of "replace".
func (s *InvestmentService) SaveAutomationRules(ctx context.Context, input SaveAutomationRulesInput) ([]InvestmentAutomationRule, error) {
	if input.OwnerUserID <= 0 {
		return nil, ValidationError{Message: "owner user is required"}
	}
	now := s.now().UTC()
	recordedAt := now.Format(time.RFC3339)
	today, err := s.settings.OwnerTodayAt(ctx, input.OwnerUserID, now)
	if err != nil {
		return nil, err
	}
	upserts := make([]db.AutomationRuleUpsert, 0, len(input.Rules))
	for _, ruleInput := range input.Rules {
		spec, err := cleanAutomationRuleSpec(ruleInput, today)
		if err != nil {
			return nil, err
		}
		upserts = append(upserts, db.AutomationRuleUpsert{RuleID: ruleInput.RuleID, Spec: spec})
	}
	records, err := s.repository.ReplaceAutomationRules(ctx, db.ReplaceAutomationRulesParams{
		BookID:        BookID,
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OriginType:    "browser_api",
		Operation:     "investment.automation_rules.replace",
		RecordedAt:    recordedAt,
		ChangeReason:  input.ChangeReason,
		Rules:         upserts,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrAutomationRuleNotFound
		}
		return nil, fmt.Errorf("save investment automation rules: %w", err)
	}
	return toInvestmentAutomationRules(records), nil
}

func cleanAutomationRuleSpec(input InvestmentAutomationRuleInput, today string) (db.AutomationRuleSpec, error) {
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
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, today)
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
	if !map[string]bool{"stock": true, "etf": true, "fund": true, "bond": true, "option": true, "future": true, "other": true, "crypto": true}[instrumentType] {
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
	maxScale := exact.MaxStandardScale
	if instrumentType == "crypto" {
		maxScale = exact.MaxCryptoScale
	}
	if input.QuantityScale < 0 || input.QuantityScale > maxScale {
		return db.InvestmentInstrumentSpec{}, ValidationError{Message: "quantity scale is invalid"}
	}
	if input.PriceScale < 0 || input.PriceScale > 12 {
		return db.InvestmentInstrumentSpec{}, ValidationError{Message: "price scale is invalid"}
	}
	identifiersJSON, err := cleanSizedJSONObject(input.IdentifiersJSON, "identifiers", investmentJSONMaxBytes)
	if err != nil {
		return db.InvestmentInstrumentSpec{}, err
	}
	if len(input.QuoteIdentifiers) > 0 {
		var identifiers map[string]json.RawMessage
		if err := json.Unmarshal([]byte(identifiersJSON), &identifiers); err != nil {
			return db.InvestmentInstrumentSpec{}, err
		}
		for key, value := range input.QuoteIdentifiers {
			if key != "coingecko_id" && key != "yahoo_symbol" && key != "twelve_data_symbol" {
				return db.InvestmentInstrumentSpec{}, ValidationError{Message: "unsupported quote identifier"}
			}
			text, err := cleanOptionalText(value, "quote identifier", 128)
			if err != nil {
				return db.InvestmentInstrumentSpec{}, err
			}
			if text == "" {
				delete(identifiers, key)
			} else {
				encoded, _ := json.Marshal(text)
				identifiers[key] = encoded
			}
		}
		encoded, err := json.Marshal(identifiers)
		if err != nil {
			return db.InvestmentInstrumentSpec{}, err
		}
		identifiersJSON, err = cleanSizedJSONObject(string(encoded), "identifiers", investmentJSONMaxBytes)
		if err != nil {
			return db.InvestmentInstrumentSpec{}, err
		}
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

func cleanDividendDefaultSpec(input DividendDefaultInput, today string) (db.DividendDefaultSpec, error) {
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
	effectiveFrom, err := cleanEffectiveFrom(input.EffectiveFrom, today)
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

// disposalChangeReason labels the lot event for what it was, so the audit trail
// distinguishes a sale from a position written off for nothing.
func disposalChangeReason(writeOff bool) string {
	if writeOff {
		return "disposed lots from write-off transaction"
	}
	return "disposed lots from sell transaction"
}

// writeOffPostings builds the security legs of a zero-proceeds disposal.
//
// A write-off has no cash legs at all rather than zero-valued ones. Two reasons:
// a zero posting on a cash account is noise a reconciler has to look at and
// dismiss. The disposal decision records zero proceeds explicitly.
//
// The commodity legs balance on their own, so the transaction stays balanced
// per commodity either way.
func writeOffPostings(input InvestmentTradeInput, tradingAccountID int64, memo string) []PostingInput {
	return []PostingInput{
		{AccountID: input.HoldingAccountID, QuantityValue: input.QuantityValue.Negated(), QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
		{AccountID: tradingAccountID, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale, CommodityID: input.CommodityID, Memo: memo},
	}
}

func validateTradeInput(input InvestmentTradeInput) error {
	if err := validatePostedInvestmentStatus(input.Status); err != nil {
		return err
	}
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}
	if _, err := cleanRequiredDate(input.TransactionDate, "transaction date"); err != nil {
		return err
	}
	if input.CommodityID <= 0 || input.HoldingAccountID <= 0 {
		return ValidationError{Message: "investment trade references are required"}
	}
	if !input.WriteOff && (input.CashAccountID <= 0 || input.CashCommodityID <= 0) {
		return ValidationError{Message: "investment trade references are required"}
	}
	if input.WriteOff && input.CostCommodityID < 0 {
		return ValidationError{Message: "cost commodity is invalid"}
	}
	if input.QuantityValue.Sign() <= 0 {
		return ValidationError{Message: "quantity is required"}
	}
	if input.QuantityScale < 0 || input.QuantityScale > exact.MaxCryptoScale || input.CashAmountScale < 0 || input.CashAmountScale > exact.MaxStandardScale {
		return ValidationError{Message: "quantity scale is invalid"}
	}
	switch {
	case input.WriteOff && input.CashAmountValue != 0:
		// A write-off is zero proceeds by definition. Accepting an amount here
		// would make the endpoint a second, unlabelled way to sell.
		return ValidationError{Message: "a write-off cannot carry a cash amount"}
	case input.WriteOff && (input.GrossAmountValue != nil || input.NetSettlementValue != nil || len(input.Charges) > 0 || input.SettlementDate != ""):
		return ValidationError{Message: "a write-off cannot carry trade economics"}
	case !input.WriteOff && input.GrossAmountValue == nil && input.CashAmountValue <= 0:
		return ValidationError{Message: "cash amount is required"}
	case !input.WriteOff && input.GrossAmountValue != nil && input.CashAmountValue < 0:
		return ValidationError{Message: "cash amount cannot be negative"}
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
	if err := validatePostedInvestmentStatus(input.Status); err != nil {
		return "", err
	}
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
	if input.WithholdingOnly && (input.CommodityID == nil || *input.CommodityID <= 0 ||
		input.WithholdingAccountID == nil || *input.WithholdingAccountID <= 0 ||
		input.IncomeAccountID != nil || input.WithholdingValue != nil || input.WithholdingScale != nil) {
		return "", ValidationError{Message: "separate withholding requires an instrument and tax account, with amount_value as tax and no income or combined withholding fields"}
	}
	date, err := cleanRequiredDate(input.TransactionDate, "transaction date")
	if err != nil {
		return "", err
	}
	return date, nil
}

func validateReinvestedDividendInput(input ReinvestedDividendInput) (string, error) {
	if err := validatePostedInvestmentStatus(input.Status); err != nil {
		return "", err
	}
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

func validatePostedInvestmentStatus(status string) error {
	status = strings.TrimSpace(status)
	if status != "" && status != "posted" {
		return ValidationError{Message: "investment transactions must be posted"}
	}
	return nil
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
	return CostBasisProfile{ID: record.ID, BookID: record.BookID, Name: record.Name, Method: record.Method, IsDefault: record.IsDefault, Status: record.Status, Description: record.Description, MetadataJSON: record.MetadataJSON, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, VersionID: record.VersionID, VersionSeq: record.VersionSeq}
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
		OpeningBasisKnowledge:   record.OpeningBasisKnowledge,
		CostBasisScale:          record.CostBasisScale,
		RemainingCostBasisValue: record.RemainingCostBasisValue,
		BasisKnowledge:          record.BasisKnowledge,
		RemainingCostBasisScale: record.RemainingCostBasisScale,
		CostCommodityID:         record.CostCommodityID,
		MetadataJSON:            record.MetadataJSON,
		CreatedAt:               record.CreatedAt,
		UpdatedAt:               record.UpdatedAt,
		PositionSide:            record.PositionSide,
		Origin:                  toInvestmentLotOrigin(record.Origin),
	}
}

func toInvestmentLotOrigin(record *db.InvestmentLotOriginRecord) *InvestmentLotOrigin {
	if record == nil {
		return nil
	}
	return &InvestmentLotOrigin{
		OperationID: record.OperationID, TransferKind: record.TransferKind, SourceCommodityID: record.SourceCommodityID,
		SourceAccountID: nullableSQLInt64Ptr(record.SourceAccountID), SourceLotID: nullableSQLInt64Ptr(record.SourceLotID),
		RatioNumerator: nullableSQLInt64Ptr(record.RatioNumerator), RatioDenominator: nullableSQLInt64Ptr(record.RatioDenominator),
		OriginalDateKnowledge: record.OriginalDateKnowledge, OriginalAcquiredOn: record.OriginalAcquiredOn.String,
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
			PriceStatus: record.PriceStatus, ValuationAsOf: record.ValuationAsOf, KnowledgeCutoff: record.KnowledgeCutoff,
			AccountID:               record.AccountID,
			CommodityID:             record.CommodityID,
			QuantityValue:           record.QuantityValue,
			QuantityScale:           record.QuantityScale,
			RemainingCostBasisValue: record.RemainingCostBasisValue,
			BasisKnowledge:          record.BasisKnowledge,
			RemainingCostBasisScale: record.RemainingCostBasisScale,
			CostCommodityID:         record.CostCommodityID,
			LatestPriceValue:        nullableSQLInt64Ptr(record.LatestPriceValue),
			LatestPriceScale:        priceScale,
			LatestPriceDate:         nullableString(record.LatestPriceDate),
			LatestPriceApproximate:  record.LatestPriceApproximate.Valid && record.LatestPriceApproximate.Int64 == 1,
			PositionSide:            record.PositionSide,
		})
	}
	return positions
}

func toInvestmentLotDisposals(records []db.LotDisposalRecord) []InvestmentLotDisposal {
	disposals := make([]InvestmentLotDisposal, 0, len(records))
	for _, record := range records {
		disposals = append(disposals, InvestmentLotDisposal{LotID: record.LotID, QuantityValue: record.QuantityValue, QuantityScale: record.QuantityScale, CostBasisValue: record.CostBasisValue, CostBasisScale: record.CostBasisScale, BasisKnowledge: normalizedKnowledge(record.BasisKnowledge), ProceedsValue: record.ProceedsValue, ProceedsScale: record.ProceedsScale})
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
	return InvestmentEventSuggestion{ID: record.ID, BookID: record.BookID, ProviderEventID: record.ProviderEventID, EventFamily: record.EventFamily, EventDate: record.EventDate, EventStatus: record.EventStatus, SourceCode: record.SourceCode, EventEvidenceJSON: record.EventNormalizedJSON, InstrumentID: nullableSQLInt64Ptr(record.InstrumentID), ConfidenceBPS: record.ConfidenceBPS, Status: record.Status, ProposedTransactionJSON: record.ProposedTransactionJSON, GeneratedTransactionID: nullableSQLInt64Ptr(record.GeneratedTransactionID), LinkedTransactionID: nullableSQLInt64Ptr(record.LinkedTransactionID), FailureReason: record.FailureReason, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
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

var errScaledDivisionOverflow = errors.New("scaled division overflow")

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
		n.Mul(n, exact.Pow10(exponent))
	} else {
		d.Mul(d, exact.Pow10(-exponent))
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
		return 0, errScaledDivisionOverflow
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
	// RealizedGainScale is the gain's own scale — the deeper of proceeds and
	// disposed basis, which is not always the proceeds scale (T-101).
	RealizedGainScale int
	// BasisKnowledge is unknown when the disposal consumed unknown basis;
	// disposed basis and gain are then unresolved and unused.
	BasisKnowledge string
	// PositionSide is short for a cover: DisposedBasis is then the positive
	// allocated opening proceeds, and the gain is still proceeds + basis.
	PositionSide string
}

// UnrealizedGainEntry is an open position with unrealized gain when a price is available.
type UnrealizedGainEntry struct {
	PriceStatus             string
	ValuationAsOf           string
	KnowledgeCutoff         string
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
	LatestPriceApproximate  bool
	MarketValueValue        *int64
	MarketValueScale        *int
	UnrealizedGainValue     *int64
	UnrealizedGainScale     *int
	ValuationUnavailable    string
	BasisKnowledge          string
	GainUnavailable         string
	// PositionSide is short for owed units: MarketValue is the signed
	// (negative) exposure and the gain is remaining opening proceeds less the
	// cost to cover.
	PositionSide string
}

func (s *InvestmentService) ListRealizedGains(ctx context.Context, params GainsReportParams) ([]RealizedGainEntry, error) {
	records, err := s.repository.ListRealizedGains(ctx, BookID, db.RealizedGainsParams{From: params.From, To: params.To})
	if err != nil {
		return nil, fmt.Errorf("list realized gains: %w", err)
	}
	return realizedGainEntries(records), nil
}

func realizedGainEntries(records []db.RealizedGainRecord) []RealizedGainEntry {
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
			RealizedGainScale:  r.RealizedGainScale,
			BasisKnowledge:     normalizedKnowledge(r.BasisKnowledge),
			PositionSide:       r.PositionSide,
		})
	}
	return entries
}

func (s *InvestmentService) ListUnrealizedGains(ctx context.Context) ([]UnrealizedGainEntry, error) {
	_, entries, err := s.UnrealizedGainsProjection(ctx, "")
	return entries, err
}

// GainsProfile names the read-only rules a gains projection ran under (R18,
// #230). Two figures compare only when their profiles match; see the R18
// reporting plan for each field's contract.
type GainsProfile struct {
	PoolScope              string `json:"pool_scope,omitempty"`
	JurisdictionNote       string `json:"jurisdiction_note,omitempty"`
	ReportingCurrencyID    int64  `json:"reporting_currency_id,omitempty"`
	FXMaxAgeDays           int    `json:"fx_max_age_days,omitempty"`
	StructuralActionPolicy string `json:"structural_action_policy,omitempty"`
	SpecificLotFallback    string `json:"specific_lot_fallback,omitempty"`
	TransferPolicy         string `json:"transfer_policy,omitempty"`
	From                   string `json:"from,omitempty"`
	To                     string `json:"to,omitempty"`
	Name                   string `json:"name"`
	Purpose                string `json:"purpose"`
	BasisMethod            string `json:"basis_method"`
	ValuationAsOf          string `json:"valuation_as_of"`
	KnowledgeCutoff        string `json:"knowledge_cutoff"`
	LedgerKnowledge        string `json:"ledger_knowledge"`
	QuotePolicy            string `json:"quote_policy"`
	StaleAfterDays         int    `json:"stale_after_days"`
	FXMethod               string `json:"fx_method"`
	Rounding               string `json:"rounding"`
}

// operationalGainsProfile is operational_v1 at the given boundary.
func operationalGainsProfile(boundary db.ObservationBoundary) GainsProfile {
	return GainsProfile{
		Name: "operational_v1", Purpose: "operational", BasisMethod: "operational_elections",
		ValuationAsOf: boundary.AsOf, KnowledgeCutoff: boundary.RecordedAt, LedgerKnowledge: "current_effective",
		QuotePolicy: "live_raw_v1", StaleAfterDays: 7, FXMethod: "none", Rounding: "exact_half_up",
	}
}

// UnrealizedGainsProjection values open positions under operational_v1. An
// empty valuationAsOf resolves to owner-local today. Every valuation replays
// positions to its date, excluding even already-recorded future events (#230).
// A date after owner-local today is refused: it would value positions with
// prices nobody has observed yet.
func (s *InvestmentService) UnrealizedGainsProjection(ctx context.Context, valuationAsOf string) (GainsProfile, []UnrealizedGainEntry, error) {
	boundary, err := s.repository.CurrentObservationBoundary(ctx, BookID, s.now())
	if err != nil {
		return GainsProfile{}, nil, err
	}
	if valuationAsOf != "" {
		date, dateErr := cleanRequiredDate(valuationAsOf, "valuation date")
		if dateErr != nil {
			return GainsProfile{}, nil, dateErr
		}
		if date > boundary.AsOf {
			return GainsProfile{}, nil, ValidationError{Message: "valuation date cannot be after today"}
		}
		boundary.AsOf = date
	}
	records, err := s.repository.DatedPositionsWithGains(ctx, BookID, boundary)
	if err != nil {
		return GainsProfile{}, nil, fmt.Errorf("list unrealized gains: %w", err)
	}
	return operationalGainsProfile(boundary), unrealizedGainEntries(records), nil
}

func unrealizedGainEntries(records []db.UnrealizedGainRecord) []UnrealizedGainEntry {
	entries := make([]UnrealizedGainEntry, 0, len(records))
	for _, r := range records {
		var priceScale *int
		if r.LatestPriceScale.Valid {
			s := int(r.LatestPriceScale.Int64)
			priceScale = &s
		}
		entries = append(entries, UnrealizedGainEntry{
			PriceStatus: r.PriceStatus, ValuationAsOf: r.ValuationAsOf, KnowledgeCutoff: r.KnowledgeCutoff,
			AccountID:               r.AccountID,
			CommodityID:             r.CommodityID,
			CostCommodityID:         r.CostCommodityID,
			QuantityValue:           r.QuantityValue,
			QuantityScale:           r.QuantityScale,
			RemainingCostBasisValue: r.RemainingCostBasisValue,
			BasisKnowledge:          r.BasisKnowledge,
			RemainingCostBasisScale: r.RemainingCostBasisScale,
			LatestPriceValue:        nullableSQLInt64Ptr(r.LatestPriceValue),
			LatestPriceScale:        priceScale,
			LatestPriceDate:         nullableString(r.LatestPriceDate),
			LatestPriceApproximate:  r.LatestPriceApproximate.Valid && r.LatestPriceApproximate.Int64 == 1,
			MarketValueValue:        r.MarketValueValue,
			MarketValueScale:        r.MarketValueScale,
			UnrealizedGainValue:     r.UnrealizedGainValue,
			UnrealizedGainScale:     r.UnrealizedGainScale,
			ValuationUnavailable:    r.ValuationUnavailable,
			GainUnavailable:         r.GainUnavailable,
			PositionSide:            r.PositionSide,
		})
	}
	return entries
}

// InvestmentImpactKind names which investment write path a reconciliation
// preview should plan. Buy and sell produce different postings from the same
// request shape, so the kind cannot be inferred from the input alone.
type InvestmentImpactKind string

const (
	InvestmentImpactBuy                InvestmentImpactKind = "buy"
	InvestmentImpactSell               InvestmentImpactKind = "sell"
	InvestmentImpactWriteOff           InvestmentImpactKind = "write_off"
	InvestmentImpactDividend           InvestmentImpactKind = "dividend"
	InvestmentImpactReinvestedDividend InvestmentImpactKind = "reinvested_dividend"
	InvestmentImpactShortSale          InvestmentImpactKind = "short_sale"
	InvestmentImpactShortCover         InvestmentImpactKind = "short_cover"
)

// TradeReconciliationImpact returns the active checkpoints a buy, sell, or
// write-off would invalidate, without persisting anything. It plans the trade
// through the same builder the write path uses. Buys also execute the writer
// and dependent replay in a rolled-back transaction before returning impact.
func (s *InvestmentService) TradeReconciliationImpact(ctx context.Context, kind InvestmentImpactKind, input InvestmentTradeInput) (ReconciliationImpact, error) {
	switch kind {
	case InvestmentImpactBuy:
		input.ReconciliationOverride = unboundOverride
		transactionParams, lotParams, err := s.prepareBuyWrite(ctx, input)
		if err != nil {
			return ReconciliationImpact{}, err
		}
		return s.openingReconciliationImpact(ctx, transactionParams, lotParams, "buy")
	case InvestmentImpactSell, InvestmentImpactWriteOff:
		input.WriteOff = kind == InvestmentImpactWriteOff
		simulated, _, err := s.simulateDisposal(ctx, input)
		if err != nil {
			return ReconciliationImpact{}, err
		}
		return s.simulatedReconciliationImpact(ctx, simulated)
	case InvestmentImpactShortSale:
		return s.shortSaleReconciliationImpact(ctx, input)
	case InvestmentImpactShortCover:
		return s.shortCoverReconciliationImpact(ctx, input)
	default:
		return ReconciliationImpact{}, ValidationError{Message: "investment impact kind is invalid"}
	}
}

// DividendReconciliationImpact is TradeReconciliationImpact for a cash dividend.
func (s *InvestmentService) DividendReconciliationImpact(ctx context.Context, input DividendInput) (ReconciliationImpact, error) {
	plan, err := s.dividendPlan(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.reconciliationImpactForPlan(ctx, plan)
}

// ReinvestedDividendReconciliationImpact is TradeReconciliationImpact for a
// reinvested dividend.
func (s *InvestmentService) ReinvestedDividendReconciliationImpact(ctx context.Context, input ReinvestedDividendInput) (ReconciliationImpact, error) {
	input.ReconciliationOverride = unboundOverride
	transactionParams, lotParams, err := s.prepareReinvestmentWrite(ctx, input)
	if err != nil {
		return ReconciliationImpact{}, err
	}
	return s.openingReconciliationImpact(ctx, transactionParams, lotParams, "reinvested dividend")
}

// simulatedReconciliationImpact names the checkpoints a rolled-back writer
// invalidated and carries its committed-disposal gain changes (T-126).
func (s *InvestmentService) simulatedReconciliationImpact(ctx context.Context, simulated db.SimulatedInvestmentWrite) (ReconciliationImpact, error) {
	refs := simulated.InvalidatedCheckpointRefs
	if err := s.transactionService.enrichCheckpointRefs(ctx, refs); err != nil {
		return ReconciliationImpact{}, err
	}
	return ReconciliationImpact{AffectedCheckpoints: refs, GainImpact: simulated.GainImpact}, nil
}

// gainImpactPolicy opts a command's first journal into gain disclosure.
func gainImpactPolicy(acknowledgement string) *db.GainImpactPolicy {
	return &db.GainImpactPolicy{Acknowledgement: acknowledgement}
}

func (s *InvestmentService) openingReconciliationImpact(ctx context.Context, transactionParams db.CreateTransactionParams, lotParams db.CreateInvestmentLotParams, kind string) (ReconciliationImpact, error) {
	transactionParams.GainImpact = gainImpactPolicy("")
	simulated, err := s.repository.SimulateLotOpening(ctx, transactionParams, lotParams)
	if err != nil {
		return ReconciliationImpact{}, mapInvestmentOpeningWriteError(err, kind)
	}
	return s.simulatedReconciliationImpact(ctx, simulated)
}

func (s *InvestmentService) reconciliationImpactForPlan(ctx context.Context, plan investmentTransactionPlan) (ReconciliationImpact, error) {
	return s.transactionService.investmentReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
		OwnerUserID: plan.Create.OwnerUserID,
		Spec:        plan.Create.Spec,
	})
}

// disposalOperationKind names the long disposal a trade input records.
func disposalOperationKind(input InvestmentTradeInput) string {
	switch {
	case input.WriteOff:
		return "write_off"
	case input.CashInLieu:
		return "cash_in_lieu"
	default:
		return "sell"
	}
}

// normalizedKnowledge gives every outgoing result explicit knowledge; an
// empty code comes from a known-only internal caller.
func normalizedKnowledge(knowledge string) string {
	if knowledge == "" {
		return db.InvestmentBasisKnown
	}
	return knowledge
}

func (s *InvestmentService) validateInstrumentCommodity(ctx context.Context, commodityID int64, spec db.InvestmentInstrumentSpec) error {
	kind, scale, err := s.repository.InstrumentCommodity(ctx, BookID, commodityID)
	if errors.Is(err, db.ErrNotFound) {
		return ValidationError{Message: "instrument commodity is unavailable"}
	}
	if err != nil {
		return fmt.Errorf("read instrument commodity: %w", err)
	}
	wanted := "security"
	if spec.InstrumentType == "crypto" {
		wanted = "crypto"
	}
	if kind != wanted || spec.QuantityScale > scale {
		return ValidationError{Message: "instrument type or precision does not match commodity"}
	}
	return nil
}
