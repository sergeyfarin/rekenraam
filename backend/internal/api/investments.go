package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/exact"
)

type investmentInstrumentResponse struct {
	ID                 int64           `json:"id"`
	BookID             int64           `json:"book_id"`
	CommodityID        int64           `json:"commodity_id"`
	CommodityCode      string          `json:"commodity_code"`
	InstrumentType     string          `json:"instrument_type"`
	DisplayName        string          `json:"display_name"`
	Symbol             string          `json:"symbol,omitempty"`
	ExchangeCode       string          `json:"exchange_code,omitempty"`
	MIC                string          `json:"mic,omitempty"`
	Issuer             string          `json:"issuer,omitempty"`
	CountryCode        string          `json:"country_code,omitempty"`
	QuoteCommodityID   *int64          `json:"quote_commodity_id,omitempty"`
	TradingCommodityID *int64          `json:"trading_commodity_id,omitempty"`
	QuantityScale      int             `json:"quantity_scale"`
	PriceScale         int             `json:"price_scale"`
	Status             string          `json:"status"`
	Identifiers        json.RawMessage `json:"identifiers"`
	Metadata           json.RawMessage `json:"metadata"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type investmentInstrumentsResponse struct {
	Instruments []investmentInstrumentResponse `json:"instruments"`
}

type investmentInstrumentRequest struct {
	CommodityID        *int64          `json:"commodity_id"`
	CommodityCode      string          `json:"commodity_code"`
	InstrumentType     string          `json:"instrument_type"`
	DisplayName        string          `json:"display_name"`
	Symbol             string          `json:"symbol"`
	ExchangeCode       string          `json:"exchange_code"`
	MIC                string          `json:"mic"`
	Issuer             string          `json:"issuer"`
	CountryCode        string          `json:"country_code"`
	QuoteCommodityID   *int64          `json:"quote_commodity_id"`
	TradingCommodityID *int64          `json:"trading_commodity_id"`
	QuantityScale      int             `json:"quantity_scale"`
	PriceScale         int             `json:"price_scale"`
	Identifiers        json.RawMessage `json:"identifiers"`
	Metadata           json.RawMessage `json:"metadata"`
	EffectiveFrom      string          `json:"effective_from"`
	ChangeReason       string          `json:"change_reason"`
}

type holdingAccountRequest struct {
	InstrumentID          int64  `json:"instrument_id"`
	Name                  string `json:"name"`
	ParentAccountID       *int64 `json:"parent_account_id"`
	InstitutionID         *int64 `json:"institution_id"`
	OpenedOn              string `json:"opened_on"`
	EffectiveFrom         string `json:"effective_from"`
	QuantityScaleOverride *int   `json:"quantity_scale_override"`
	ChangeReason          string `json:"change_reason"`
	CostBasisMethod       string `json:"cost_basis_method"`
}

type costBasisProfileResponse struct {
	ID          int64           `json:"id"`
	BookID      int64           `json:"book_id"`
	Name        string          `json:"name"`
	Method      string          `json:"method"`
	IsDefault   bool            `json:"is_default"`
	Status      string          `json:"status"`
	Description string          `json:"description"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type costBasisProfilesResponse struct {
	Profiles []costBasisProfileResponse `json:"profiles"`
}

type costBasisProfileRequest struct {
	Name         string          `json:"name"`
	Method       string          `json:"method"`
	IsDefault    bool            `json:"is_default"`
	Status       string          `json:"status"`
	Description  string          `json:"description"`
	Metadata     json.RawMessage `json:"metadata"`
	ChangeReason string          `json:"change_reason"`
}

type dividendDefaultResponse struct {
	ID                      int64           `json:"id"`
	BookID                  int64           `json:"book_id"`
	CommodityID             *int64          `json:"commodity_id,omitempty"`
	IncomeAccountID         int64           `json:"income_account_id"`
	WithholdingAccountID    *int64          `json:"withholding_account_id,omitempty"`
	DefaultWithholdingValue *int64          `json:"default_withholding_value,omitempty"`
	DefaultWithholdingScale *int            `json:"default_withholding_scale,omitempty"`
	WithholdingRateBPS      *int64          `json:"withholding_rate_bps,omitempty"`
	TaxCountryCode          string          `json:"tax_country_code,omitempty"`
	TaxTreatment            string          `json:"tax_treatment,omitempty"`
	Status                  string          `json:"status"`
	EffectiveFrom           string          `json:"effective_from"`
	EffectiveTo             string          `json:"effective_to,omitempty"`
	Metadata                json.RawMessage `json:"metadata"`
	CreatedAt               string          `json:"created_at"`
	UpdatedAt               string          `json:"updated_at"`
}

type dividendDefaultsResponse struct {
	Defaults []dividendDefaultResponse `json:"defaults"`
}

type dividendDefaultRequest struct {
	CommodityID             *int64          `json:"commodity_id"`
	IncomeAccountID         int64           `json:"income_account_id"`
	WithholdingAccountID    *int64          `json:"withholding_account_id"`
	DefaultWithholdingValue *int64          `json:"default_withholding_value"`
	DefaultWithholdingScale *int            `json:"default_withholding_scale"`
	WithholdingRateBPS      *int64          `json:"withholding_rate_bps"`
	TaxCountryCode          string          `json:"tax_country_code"`
	TaxTreatment            string          `json:"tax_treatment"`
	Status                  string          `json:"status"`
	EffectiveFrom           string          `json:"effective_from"`
	EffectiveTo             string          `json:"effective_to"`
	Metadata                json.RawMessage `json:"metadata"`
	ChangeReason            string          `json:"change_reason"`
}

type investmentTradeRequest struct {
	TransactionDate  string                           `json:"transaction_date"`
	CommodityID      int64                            `json:"commodity_id"`
	HoldingAccountID int64                            `json:"holding_account_id"`
	CashAccountID    int64                            `json:"cash_account_id"`
	QuantityValue    exact.Coefficient                `json:"quantity_value"`
	QuantityScale    int                              `json:"quantity_scale"`
	CashAmountValue  int64                            `json:"cash_amount_value"`
	CashAmountScale  int                              `json:"cash_amount_scale"`
	CashCommodityID  int64                            `json:"cash_commodity_id"`
	Memo             string                           `json:"memo"`
	PayeeID          *int64                           `json:"payee_id"`
	Status           string                           `json:"status"`
	LotAllocations   []investmentLotAllocationRequest `json:"lot_allocations"`
	ChangeReason     string                           `json:"change_reason"`
	CostBasisMethod  string                           `json:"cost_basis_method"`
	// ReconciliationOverride lets a backdated trade proceed into a reconciled
	// period, invalidating the affected checkpoints. The service has always
	// honoured it; without it on the wire a buy, sell, or write-off dated
	// before a checkpoint was simply unreachable (T-47).
	ReconciliationOverride bool `json:"reconciliation_override"`
}

type sellPreviewResponse struct {
	CostBasisMethod   string                          `json:"cost_basis_method"`
	Allocations       []investmentLotDisposalResponse `json:"allocations"`
	RealizedGain      int64                           `json:"realized_gain"`
	RealizedGainScale int                             `json:"realized_gain_scale"`
	CashAmountValue   int64                           `json:"cash_amount_value"`
	CashAmountScale   int                             `json:"cash_amount_scale"`
}

type investmentLotAllocationRequest struct {
	LotID         int64             `json:"lot_id"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
}

type investmentTradeResponse struct {
	Transaction transactionResponse             `json:"transaction"`
	LotID       *int64                          `json:"lot_id,omitempty"`
	Allocations []investmentLotDisposalResponse `json:"allocations"`
}

type investmentLotDisposalResponse struct {
	LotID          int64             `json:"lot_id"`
	QuantityValue  exact.Coefficient `json:"quantity_value"`
	QuantityScale  int               `json:"quantity_scale"`
	CostBasisValue int64             `json:"cost_basis_value"`
	CostBasisScale int               `json:"cost_basis_scale"`
}

type dividendRequest struct {
	TransactionDate        string `json:"transaction_date"`
	CommodityID            *int64 `json:"commodity_id"`
	CashAccountID          int64  `json:"cash_account_id"`
	CashCommodityID        int64  `json:"cash_commodity_id"`
	IncomeAccountID        *int64 `json:"income_account_id"`
	AmountValue            int64  `json:"amount_value"`
	AmountScale            int    `json:"amount_scale"`
	WithholdingValue       *int64 `json:"withholding_value"`
	WithholdingScale       *int   `json:"withholding_scale"`
	WithholdingAccountID   *int64 `json:"withholding_account_id"`
	Memo                   string `json:"memo"`
	PayeeID                *int64 `json:"payee_id"`
	Status                 string `json:"status"`
	ChangeReason           string `json:"change_reason"`
	ReconciliationOverride bool   `json:"reconciliation_override"`
}

type reinvestedDividendRequest struct {
	TransactionDate        string            `json:"transaction_date"`
	CommodityID            int64             `json:"commodity_id"`
	HoldingAccountID       int64             `json:"holding_account_id"`
	IncomeAccountID        *int64            `json:"income_account_id"`
	QuantityValue          exact.Coefficient `json:"quantity_value"`
	QuantityScale          int               `json:"quantity_scale"`
	AmountValue            int64             `json:"amount_value"`
	AmountScale            int               `json:"amount_scale"`
	CashCommodityID        int64             `json:"cash_commodity_id"`
	Memo                   string            `json:"memo"`
	PayeeID                *int64            `json:"payee_id"`
	Status                 string            `json:"status"`
	ChangeReason           string            `json:"change_reason"`
	ReconciliationOverride bool              `json:"reconciliation_override"`
}

type investmentLotResponse struct {
	ID                      int64             `json:"id"`
	BookID                  int64             `json:"book_id"`
	AccountID               int64             `json:"account_id"`
	CommodityID             int64             `json:"commodity_id"`
	OpenedOn                string            `json:"opened_on"`
	SourceTransactionID     *int64            `json:"source_transaction_id,omitempty"`
	Status                  string            `json:"status"`
	QuantityValue           exact.Coefficient `json:"quantity_value"`
	QuantityScale           int               `json:"quantity_scale"`
	RemainingQuantityValue  exact.Coefficient `json:"remaining_quantity_value"`
	RemainingQuantityScale  int               `json:"remaining_quantity_scale"`
	CostBasisValue          int64             `json:"cost_basis_value"`
	CostBasisScale          int               `json:"cost_basis_scale"`
	RemainingCostBasisValue int64             `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale int               `json:"remaining_cost_basis_scale"`
	CostCommodityID         int64             `json:"cost_commodity_id"`
	Metadata                json.RawMessage   `json:"metadata"`
	CreatedAt               string            `json:"created_at"`
	UpdatedAt               string            `json:"updated_at"`
}

type investmentLotsResponse struct {
	Lots []investmentLotResponse `json:"lots"`
}

type investmentPositionResponse struct {
	AccountID               int64             `json:"account_id"`
	CommodityID             int64             `json:"commodity_id"`
	QuantityValue           exact.Coefficient `json:"quantity_value"`
	QuantityScale           int               `json:"quantity_scale"`
	RemainingCostBasisValue int64             `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale int               `json:"remaining_cost_basis_scale"`
	CostCommodityID         int64             `json:"cost_commodity_id"`
	LatestPriceValue        *int64            `json:"latest_price_value,omitempty"`
	LatestPriceScale        *int              `json:"latest_price_scale,omitempty"`
	LatestPriceDate         string            `json:"latest_price_date,omitempty"`
}

type investmentPositionsResponse struct {
	Positions []investmentPositionResponse `json:"positions"`
}

type investmentProviderEventResponse struct {
	ID              int64           `json:"id"`
	BookID          int64           `json:"book_id"`
	SourceID        int64           `json:"source_id"`
	ProviderEventID string          `json:"provider_event_id,omitempty"`
	InstrumentID    *int64          `json:"instrument_id,omitempty"`
	EventFamily     string          `json:"event_family"`
	EventDate       string          `json:"event_date"`
	Status          string          `json:"status"`
	Normalized      json.RawMessage `json:"normalized"`
	Raw             json.RawMessage `json:"raw"`
	CreatedAt       string          `json:"created_at"`
}

type investmentProviderEventsResponse struct {
	Events []investmentProviderEventResponse `json:"events"`
}

type investmentEventSuggestionResponse struct {
	ID                     int64           `json:"id"`
	BookID                 int64           `json:"book_id"`
	ProviderEventID        int64           `json:"provider_event_id"`
	InstrumentID           *int64          `json:"instrument_id,omitempty"`
	ConfidenceBPS          int             `json:"confidence_bps"`
	Status                 string          `json:"status"`
	ProposedTransaction    json.RawMessage `json:"proposed_transaction"`
	GeneratedTransactionID *int64          `json:"generated_transaction_id,omitempty"`
	FailureReason          string          `json:"failure_reason,omitempty"`
	CreatedAt              string          `json:"created_at"`
	UpdatedAt              string          `json:"updated_at"`
}

type investmentEventSuggestionsResponse struct {
	Suggestions []investmentEventSuggestionResponse `json:"suggestions"`
}

type investmentAutomationRuleResponse struct {
	ID                     int64           `json:"id"`
	BookID                 int64           `json:"book_id"`
	SourceID               *int64          `json:"source_id,omitempty"`
	InstrumentID           *int64          `json:"instrument_id,omitempty"`
	EventFamily            string          `json:"event_family"`
	Mode                   string          `json:"mode"`
	ConfidenceThresholdBPS int             `json:"confidence_threshold_bps"`
	RequiredAccounts       json.RawMessage `json:"required_accounts"`
	Status                 string          `json:"status"`
	EffectiveFrom          string          `json:"effective_from"`
	EffectiveTo            string          `json:"effective_to,omitempty"`
	CreatedAt              string          `json:"created_at"`
	UpdatedAt              string          `json:"updated_at"`
}

type investmentAutomationRulesResponse struct {
	Rules []investmentAutomationRuleResponse `json:"rules"`
}

type investmentAutomationRuleRequest struct {
	ID                     int64           `json:"id"`
	SourceID               *int64          `json:"source_id"`
	InstrumentID           *int64          `json:"instrument_id"`
	EventFamily            string          `json:"event_family"`
	Mode                   string          `json:"mode"`
	ConfidenceThresholdBPS int             `json:"confidence_threshold_bps"`
	RequiredAccounts       json.RawMessage `json:"required_accounts"`
	Status                 string          `json:"status"`
	EffectiveFrom          string          `json:"effective_from"`
	EffectiveTo            string          `json:"effective_to"`
}

type investmentAutomationRulesRequest struct {
	Rules        []investmentAutomationRuleRequest `json:"rules"`
	ChangeReason string                            `json:"change_reason"`
}

func searchInvestments(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		instruments, err := investmentService.Search(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "search investments", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentInstrumentsResponse{Instruments: toInvestmentInstrumentResponses(instruments)})
	}
}

func listInvestmentInstruments(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		instruments, err := investmentService.ListInstruments(r.Context())
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list investment instruments", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentInstrumentsResponse{Instruments: toInvestmentInstrumentResponses(instruments)})
	}
}

func createInvestmentInstrument(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request investmentInstrumentRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		instrument, err := investmentService.CreateInstrument(r.Context(), toInvestmentInstrumentInput(owner, r, request, 0))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "create investment instrument", err)
			return
		}
		writeJSON(w, http.StatusCreated, toInvestmentInstrumentResponse(instrument))
	}))
}

func readInvestmentInstrument(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		instrumentID, ok := readPathInt64(w, r, "instrument_id", "instrument id")
		if !ok {
			return
		}
		instrument, err := investmentService.Instrument(r.Context(), instrumentID)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read investment instrument", err)
			return
		}
		writeJSON(w, http.StatusOK, toInvestmentInstrumentResponse(instrument))
	}
}

func updateInvestmentInstrument(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		instrumentID, ok := readPathInt64(w, r, "instrument_id", "instrument id")
		if !ok {
			return
		}
		var request investmentInstrumentRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		instrument, err := investmentService.UpdateInstrument(r.Context(), toInvestmentInstrumentInput(owner, r, request, instrumentID))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "update investment instrument", err)
			return
		}
		writeJSON(w, http.StatusOK, toInvestmentInstrumentResponse(instrument))
	}))
}

func createHoldingAccount(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request holdingAccountRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		account, err := investmentService.CreateHoldingAccount(r.Context(), app.HoldingAccountInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			InstrumentID: request.InstrumentID, Name: request.Name, ParentAccountID: request.ParentAccountID,
			InstitutionID: request.InstitutionID, OpenedOn: request.OpenedOn, EffectiveFrom: request.EffectiveFrom,
			QuantityScale: request.QuantityScaleOverride, ChangeReason: request.ChangeReason,
			CostBasisMethod: request.CostBasisMethod,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "create holding account", err)
			return
		}
		writeJSON(w, http.StatusCreated, toAccountResponse(account))
	}))
}

func listCostBasisProfiles(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		profiles, err := investmentService.ListCostBasisProfiles(r.Context(), owner.ID, authenticatedSessionID(r), RequestIDFromContext(r.Context()))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list cost basis profiles", err)
			return
		}
		writeJSON(w, http.StatusOK, costBasisProfilesResponse{Profiles: toCostBasisProfileResponses(profiles)})
	}
}

func saveCostBasisProfile(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions, profileIDFromPath bool) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		profileID := int64(0)
		if profileIDFromPath {
			var parsedOK bool
			profileID, parsedOK = readPathInt64(w, r, "profile_id", "profile id")
			if !parsedOK {
				return
			}
		}
		var request costBasisProfileRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		profile, err := investmentService.SaveCostBasisProfile(r.Context(), app.CostBasisProfileInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			ProfileID: profileID, Name: request.Name, Method: request.Method, IsDefault: request.IsDefault,
			Status: request.Status, Description: request.Description, MetadataJSON: rawJSONText(request.Metadata), ChangeReason: request.ChangeReason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "save cost basis profile", err)
			return
		}
		status := http.StatusOK
		if !profileIDFromPath {
			status = http.StatusCreated
		}
		writeJSON(w, status, toCostBasisProfileResponse(profile))
	}))
}

func listDividendDefaults(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		commodityID, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("commodity_id"), "commodity id")
		if !ok {
			return
		}
		defaults, err := investmentService.ListDividendDefaults(r.Context(), commodityID)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list dividend defaults", err)
			return
		}
		writeJSON(w, http.StatusOK, dividendDefaultsResponse{Defaults: toDividendDefaultResponses(defaults)})
	}
}

func saveDividendDefault(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions, defaultIDFromPath bool) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		defaultID := int64(0)
		if defaultIDFromPath {
			var parsedOK bool
			defaultID, parsedOK = readPathInt64(w, r, "default_id", "dividend default id")
			if !parsedOK {
				return
			}
		}
		var request dividendDefaultRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		dividendDefault, err := investmentService.SaveDividendDefault(r.Context(), app.DividendDefaultInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			DefaultID: defaultID, CommodityID: request.CommodityID, IncomeAccountID: request.IncomeAccountID,
			WithholdingAccountID: request.WithholdingAccountID, DefaultWithholdingValue: request.DefaultWithholdingValue,
			DefaultWithholdingScale: request.DefaultWithholdingScale, WithholdingRateBPS: request.WithholdingRateBPS,
			TaxCountryCode: request.TaxCountryCode, TaxTreatment: request.TaxTreatment, Status: request.Status,
			EffectiveFrom: request.EffectiveFrom, EffectiveTo: request.EffectiveTo, MetadataJSON: rawJSONText(request.Metadata),
			ChangeReason: request.ChangeReason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "save dividend default", err)
			return
		}
		status := http.StatusOK
		if !defaultIDFromPath {
			status = http.StatusCreated
		}
		writeJSON(w, status, toDividendDefaultResponse(dividendDefault))
	}))
}

func buyInvestment(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return investmentTradeMutation(logger, authService, investmentService, options, "buy")
}

func sellInvestment(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return investmentTradeMutation(logger, authService, investmentService, options, "sell")
}

// writeOffInvestment records a disposal at zero proceeds. It is its own route
// rather than a flag on sell so that "no proceeds" is always a stated intent —
// an empty cash amount must never quietly retire a position (T-38).
func writeOffInvestment(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return investmentTradeMutation(logger, authService, investmentService, options, "write-off")
}

func sellPreviewInvestment(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request investmentTradeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input := toInvestmentTradeInput(owner, r, request)
		preview, err := investmentService.PreviewSell(r.Context(), input)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview sell", err)
			return
		}
		writeJSON(w, http.StatusOK, sellPreviewResponse{
			CostBasisMethod:   preview.CostBasisMethod,
			Allocations:       toInvestmentLotDisposalResponses(preview.Allocations),
			RealizedGain:      preview.RealizedGain,
			RealizedGainScale: preview.RealizedGainScale,
			CashAmountValue:   preview.CashAmountValue,
			CashAmountScale:   preview.CashAmountScale,
		})
	}
}

// investmentTradeReconciliationImpact previews which checkpoints a trade would
// invalidate. It is a read-only preview like sell/preview, so it takes no CSRF
// token and persists nothing; the UI calls it to name the checkpoints in its
// confirmation before retrying with reconciliation_override (T-47).
func investmentTradeReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, kind app.InvestmentImpactKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request investmentTradeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.TradeReconciliationImpact(r.Context(), kind, toInvestmentTradeInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "investment reconciliation impact", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationImpactResponse(impact))
	}
}

func dividendReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request dividendRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.DividendReconciliationImpact(r.Context(), toDividendInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "dividend reconciliation impact", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationImpactResponse(impact))
	}
}

func reinvestedDividendReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request reinvestedDividendRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.ReinvestedDividendReconciliationImpact(r.Context(), toReinvestedDividendInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reinvested dividend reconciliation impact", err)
			return
		}
		writeJSON(w, http.StatusOK, toReconciliationImpactResponse(impact))
	}
}

func investmentTradeMutation(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions, action string) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request investmentTradeRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input := toInvestmentTradeInput(owner, r, request)
		var result app.InvestmentTradeResult
		var err error
		switch action {
		case "buy":
			result, err = investmentService.Buy(r.Context(), input)
		case "write-off":
			result, err = investmentService.WriteOff(r.Context(), input)
		default:
			result, err = investmentService.Sell(r.Context(), input)
		}
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "investment "+action, err)
			return
		}
		writeJSON(w, http.StatusCreated, toInvestmentTradeResponse(result))
	}))
}

func createDividend(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request dividendRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := investmentService.Dividend(r.Context(), toDividendInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "create dividend", err)
			return
		}
		writeJSON(w, http.StatusCreated, toTransactionResponse(transaction))
	}))
}

func createReinvestedDividend(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request reinvestedDividendRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ReinvestedDividend(r.Context(), toReinvestedDividendInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "create reinvested dividend", err)
			return
		}
		writeJSON(w, http.StatusCreated, toInvestmentTradeResponse(result))
	}))
}

func listInvestmentLots(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		accountID, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("account_id"), "account id")
		if !ok {
			return
		}
		commodityID, ok := parseOptionalPositiveInt64(w, r.URL.Query().Get("commodity_id"), "commodity id")
		if !ok {
			return
		}
		lots, err := investmentService.ListLots(r.Context(), accountID, commodityID)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list investment lots", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentLotsResponse{Lots: toInvestmentLotResponses(lots)})
	}
}

func listInvestmentPositions(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		positions, err := investmentService.Positions(r.Context())
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list investment positions", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentPositionsResponse{Positions: toInvestmentPositionResponses(positions)})
	}
}

func listInvestmentEvents(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		events, err := investmentService.ListProviderEvents(r.Context())
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list investment events", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentProviderEventsResponse{Events: toInvestmentProviderEventResponses(events)})
	}
}

func listInvestmentEventSuggestions(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		suggestions, err := investmentService.ListEventSuggestions(r.Context())
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list investment event suggestions", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentEventSuggestionsResponse{Suggestions: toInvestmentEventSuggestionResponses(suggestions)})
	}
}

func acceptInvestmentEventSuggestion(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return eventSuggestionStatusMutation(logger, authService, investmentService, options, "accept")
}

func ignoreInvestmentEventSuggestion(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return eventSuggestionStatusMutation(logger, authService, investmentService, options, "ignore")
}

func eventSuggestionStatusMutation(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions, action string) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		suggestionID, ok := readPathInt64(w, r, "suggestion_id", "suggestion id")
		if !ok {
			return
		}
		var suggestion app.InvestmentEventSuggestion
		var err error
		if action == "accept" {
			suggestion, err = investmentService.AcceptSuggestion(r.Context(), owner.ID, authenticatedSessionID(r), RequestIDFromContext(r.Context()), suggestionID)
		} else {
			suggestion, err = investmentService.IgnoreSuggestion(r.Context(), owner.ID, authenticatedSessionID(r), RequestIDFromContext(r.Context()), suggestionID)
		}
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "update investment event suggestion", err)
			return
		}
		writeJSON(w, http.StatusOK, toInvestmentEventSuggestionResponse(suggestion))
	}))
}

func listInvestmentAutomationRules(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		rules, err := investmentService.ListAutomationRules(r.Context())
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list investment automation rules", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentAutomationRulesResponse{Rules: toInvestmentAutomationRuleResponses(rules)})
	}
}

func saveInvestmentAutomationRules(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request investmentAutomationRulesRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		rules := make([]app.InvestmentAutomationRuleInput, 0, len(request.Rules))
		for _, rule := range request.Rules {
			rules = append(rules, app.InvestmentAutomationRuleInput{
				RuleID:                 rule.ID,
				SourceID:               rule.SourceID,
				InstrumentID:           rule.InstrumentID,
				EventFamily:            rule.EventFamily,
				Mode:                   rule.Mode,
				ConfidenceThresholdBPS: rule.ConfidenceThresholdBPS,
				RequiredAccountsJSON:   rawJSONText(rule.RequiredAccounts),
				Status:                 rule.Status,
				EffectiveFrom:          rule.EffectiveFrom,
				EffectiveTo:            rule.EffectiveTo,
			})
		}
		savedRules, err := investmentService.SaveAutomationRules(r.Context(), app.SaveAutomationRulesInput{
			OwnerUserID:   owner.ID,
			AuthSessionID: authenticatedSessionID(r),
			RequestID:     RequestIDFromContext(r.Context()),
			ChangeReason:  request.ChangeReason,
			Rules:         rules,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "save investment automation rules", err)
			return
		}
		writeJSON(w, http.StatusOK, investmentAutomationRulesResponse{Rules: toInvestmentAutomationRuleResponses(savedRules)})
	}))
}

func writeInvestmentServiceError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, action string, err error) {
	var validationError app.ValidationError
	var overflowError app.LedgerOverflowError
	switch {
	case errors.As(err, &validationError):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationError.Error())
	case errors.As(err, &overflowError):
		writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", overflowError.Error())
	case errors.Is(err, app.ErrInvestmentInstrumentNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment instrument not found")
	case errors.Is(err, app.ErrInvestmentInstrumentExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "investment instrument already exists")
	case errors.Is(err, app.ErrCostBasisProfileNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "cost basis profile not found")
	case errors.Is(err, app.ErrCostBasisProfileExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "cost basis profile already exists")
	case errors.Is(err, app.ErrDividendDefaultNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "dividend default not found")
	case errors.Is(err, app.ErrInvestmentLotsInsufficient):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "insufficient investment lots")
	// Every investment trade goes through the transaction write guard, so a
	// backdated trade into a reconciled period raises this. It was unmapped and
	// surfaced as a 500 "internal server error", which told the user nothing and
	// looked like a fault rather than the deliberate refusal it is.
	case errors.Is(err, app.ErrReconciliationOverrideRequired):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "reconciliation override is required")
	case errors.Is(err, app.ErrInvestmentSuggestionNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment event suggestion not found")
	case errors.Is(err, app.ErrInvestmentSuggestionNotPending):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "investment event suggestion is not pending")
	case errors.Is(err, app.ErrAutomationRuleNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment automation rule not found")
	default:
		writeServiceInternalError(w, r, logger, action, err)
	}
}

func readPathInt64(w http.ResponseWriter, r *http.Request, key string, field string) (int64, bool) {
	value, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || value <= 0 {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", field+" is invalid")
		return 0, false
	}
	return value, true
}

func toInvestmentInstrumentInput(owner app.Owner, r *http.Request, request investmentInstrumentRequest, instrumentID int64) app.InvestmentInstrumentInput {
	return app.InvestmentInstrumentInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		OriginType: "browser_api", Operation: "", InstrumentID: instrumentID, CommodityID: request.CommodityID,
		CommodityCode: request.CommodityCode, InstrumentType: request.InstrumentType, DisplayName: request.DisplayName,
		Symbol: request.Symbol, ExchangeCode: request.ExchangeCode, MIC: request.MIC, Issuer: request.Issuer,
		CountryCode: request.CountryCode, QuoteCommodityID: request.QuoteCommodityID, TradingCommodityID: request.TradingCommodityID,
		QuantityScale: request.QuantityScale, PriceScale: request.PriceScale, IdentifiersJSON: rawJSONText(request.Identifiers),
		MetadataJSON: rawJSONText(request.Metadata), EffectiveFrom: request.EffectiveFrom, ChangeReason: request.ChangeReason,
	}
}

func toInvestmentTradeInput(owner app.Owner, r *http.Request, request investmentTradeRequest) app.InvestmentTradeInput {
	allocations := make([]app.InvestmentLotAllocationInput, 0, len(request.LotAllocations))
	for _, allocation := range request.LotAllocations {
		allocations = append(allocations, app.InvestmentLotAllocationInput{LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	return app.InvestmentTradeInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionDate: request.TransactionDate, CommodityID: request.CommodityID, HoldingAccountID: request.HoldingAccountID,
		CashAccountID: request.CashAccountID, QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		CashAmountValue: request.CashAmountValue, CashAmountScale: request.CashAmountScale, CashCommodityID: request.CashCommodityID,
		Memo: request.Memo, PayeeID: request.PayeeID, Status: request.Status, LotAllocations: allocations,
		ChangeReason: request.ChangeReason, CostBasisMethod: request.CostBasisMethod,
		ReconciliationOverride: request.ReconciliationOverride,
	}
}

func toDividendInput(owner app.Owner, r *http.Request, request dividendRequest) app.DividendInput {
	return app.DividendInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionDate: request.TransactionDate, CommodityID: request.CommodityID, CashAccountID: request.CashAccountID,
		CashCommodityID: request.CashCommodityID, IncomeAccountID: request.IncomeAccountID, AmountValue: request.AmountValue,
		AmountScale: request.AmountScale, WithholdingValue: request.WithholdingValue, WithholdingScale: request.WithholdingScale,
		WithholdingAccountID: request.WithholdingAccountID, Memo: request.Memo, PayeeID: request.PayeeID,
		Status: request.Status, ChangeReason: request.ChangeReason,
		ReconciliationOverride: request.ReconciliationOverride,
	}
}

func toReinvestedDividendInput(owner app.Owner, r *http.Request, request reinvestedDividendRequest) app.ReinvestedDividendInput {
	return app.ReinvestedDividendInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionDate: request.TransactionDate, CommodityID: request.CommodityID, HoldingAccountID: request.HoldingAccountID,
		IncomeAccountID: request.IncomeAccountID, QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		AmountValue: request.AmountValue, AmountScale: request.AmountScale, CashCommodityID: request.CashCommodityID,
		Memo: request.Memo, PayeeID: request.PayeeID, Status: request.Status, ChangeReason: request.ChangeReason,
		ReconciliationOverride: request.ReconciliationOverride,
	}
}

func toInvestmentInstrumentResponse(instrument app.InvestmentInstrument) investmentInstrumentResponse {
	return investmentInstrumentResponse{ID: instrument.ID, BookID: instrument.BookID, CommodityID: instrument.CommodityID, CommodityCode: instrument.CommodityCode, InstrumentType: instrument.InstrumentType, DisplayName: instrument.DisplayName, Symbol: instrument.Symbol, ExchangeCode: instrument.ExchangeCode, MIC: instrument.MIC, Issuer: instrument.Issuer, CountryCode: instrument.CountryCode, QuoteCommodityID: instrument.QuoteCommodityID, TradingCommodityID: instrument.TradingCommodityID, QuantityScale: instrument.QuantityScale, PriceScale: instrument.PriceScale, Status: instrument.Status, Identifiers: json.RawMessage(instrument.IdentifiersJSON), Metadata: json.RawMessage(instrument.MetadataJSON), CreatedAt: instrument.CreatedAt, UpdatedAt: instrument.UpdatedAt}
}

func toInvestmentInstrumentResponses(instruments []app.InvestmentInstrument) []investmentInstrumentResponse {
	responses := make([]investmentInstrumentResponse, 0, len(instruments))
	for _, instrument := range instruments {
		responses = append(responses, toInvestmentInstrumentResponse(instrument))
	}
	return responses
}

func toCostBasisProfileResponse(profile app.CostBasisProfile) costBasisProfileResponse {
	return costBasisProfileResponse{ID: profile.ID, BookID: profile.BookID, Name: profile.Name, Method: profile.Method, IsDefault: profile.IsDefault, Status: profile.Status, Description: profile.Description, Metadata: json.RawMessage(profile.MetadataJSON), CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt}
}

func toCostBasisProfileResponses(profiles []app.CostBasisProfile) []costBasisProfileResponse {
	responses := make([]costBasisProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		responses = append(responses, toCostBasisProfileResponse(profile))
	}
	return responses
}

func toDividendDefaultResponse(dividendDefault app.DividendDefault) dividendDefaultResponse {
	return dividendDefaultResponse{ID: dividendDefault.ID, BookID: dividendDefault.BookID, CommodityID: dividendDefault.CommodityID, IncomeAccountID: dividendDefault.IncomeAccountID, WithholdingAccountID: dividendDefault.WithholdingAccountID, DefaultWithholdingValue: dividendDefault.DefaultWithholdingValue, DefaultWithholdingScale: dividendDefault.DefaultWithholdingScale, WithholdingRateBPS: dividendDefault.WithholdingRateBPS, TaxCountryCode: dividendDefault.TaxCountryCode, TaxTreatment: dividendDefault.TaxTreatment, Status: dividendDefault.Status, EffectiveFrom: dividendDefault.EffectiveFrom, EffectiveTo: dividendDefault.EffectiveTo, Metadata: json.RawMessage(dividendDefault.MetadataJSON), CreatedAt: dividendDefault.CreatedAt, UpdatedAt: dividendDefault.UpdatedAt}
}

func toDividendDefaultResponses(defaults []app.DividendDefault) []dividendDefaultResponse {
	responses := make([]dividendDefaultResponse, 0, len(defaults))
	for _, dividendDefault := range defaults {
		responses = append(responses, toDividendDefaultResponse(dividendDefault))
	}
	return responses
}

func toInvestmentTradeResponse(result app.InvestmentTradeResult) investmentTradeResponse {
	return investmentTradeResponse{Transaction: toTransactionResponse(result.Transaction), LotID: result.LotID, Allocations: toInvestmentLotDisposalResponses(result.Allocations)}
}

func toInvestmentLotDisposalResponses(disposals []app.InvestmentLotDisposal) []investmentLotDisposalResponse {
	responses := make([]investmentLotDisposalResponse, 0, len(disposals))
	for _, disposal := range disposals {
		responses = append(responses, investmentLotDisposalResponse{LotID: disposal.LotID, QuantityValue: disposal.QuantityValue, QuantityScale: disposal.QuantityScale, CostBasisValue: disposal.CostBasisValue, CostBasisScale: disposal.CostBasisScale})
	}
	return responses
}

func toInvestmentLotResponse(lot app.InvestmentLot) investmentLotResponse {
	return investmentLotResponse{ID: lot.ID, BookID: lot.BookID, AccountID: lot.AccountID, CommodityID: lot.CommodityID, OpenedOn: lot.OpenedOn, SourceTransactionID: lot.SourceTransactionID, Status: lot.Status, QuantityValue: lot.QuantityValue, QuantityScale: lot.QuantityScale, RemainingQuantityValue: lot.RemainingQuantityValue, RemainingQuantityScale: lot.RemainingQuantityScale, CostBasisValue: lot.CostBasisValue, CostBasisScale: lot.CostBasisScale, RemainingCostBasisValue: lot.RemainingCostBasisValue, RemainingCostBasisScale: lot.RemainingCostBasisScale, CostCommodityID: lot.CostCommodityID, Metadata: json.RawMessage(lot.MetadataJSON), CreatedAt: lot.CreatedAt, UpdatedAt: lot.UpdatedAt}
}

func toInvestmentLotResponses(lots []app.InvestmentLot) []investmentLotResponse {
	responses := make([]investmentLotResponse, 0, len(lots))
	for _, lot := range lots {
		responses = append(responses, toInvestmentLotResponse(lot))
	}
	return responses
}

func toInvestmentPositionResponses(positions []app.InvestmentPosition) []investmentPositionResponse {
	responses := make([]investmentPositionResponse, 0, len(positions))
	for _, position := range positions {
		responses = append(responses, investmentPositionResponse{AccountID: position.AccountID, CommodityID: position.CommodityID, QuantityValue: position.QuantityValue, QuantityScale: position.QuantityScale, RemainingCostBasisValue: position.RemainingCostBasisValue, RemainingCostBasisScale: position.RemainingCostBasisScale, CostCommodityID: position.CostCommodityID, LatestPriceValue: position.LatestPriceValue, LatestPriceScale: position.LatestPriceScale, LatestPriceDate: position.LatestPriceDate})
	}
	return responses
}

func toInvestmentProviderEventResponses(events []app.InvestmentProviderEvent) []investmentProviderEventResponse {
	responses := make([]investmentProviderEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, investmentProviderEventResponse{ID: event.ID, BookID: event.BookID, SourceID: event.SourceID, ProviderEventID: event.ProviderEventID, InstrumentID: event.InstrumentID, EventFamily: event.EventFamily, EventDate: event.EventDate, Status: event.Status, Normalized: json.RawMessage(event.NormalizedJSON), Raw: json.RawMessage(event.RawJSON), CreatedAt: event.CreatedAt})
	}
	return responses
}

func toInvestmentEventSuggestionResponse(suggestion app.InvestmentEventSuggestion) investmentEventSuggestionResponse {
	return investmentEventSuggestionResponse{ID: suggestion.ID, BookID: suggestion.BookID, ProviderEventID: suggestion.ProviderEventID, InstrumentID: suggestion.InstrumentID, ConfidenceBPS: suggestion.ConfidenceBPS, Status: suggestion.Status, ProposedTransaction: json.RawMessage(suggestion.ProposedTransactionJSON), GeneratedTransactionID: suggestion.GeneratedTransactionID, FailureReason: suggestion.FailureReason, CreatedAt: suggestion.CreatedAt, UpdatedAt: suggestion.UpdatedAt}
}

func toInvestmentEventSuggestionResponses(suggestions []app.InvestmentEventSuggestion) []investmentEventSuggestionResponse {
	responses := make([]investmentEventSuggestionResponse, 0, len(suggestions))
	for _, suggestion := range suggestions {
		responses = append(responses, toInvestmentEventSuggestionResponse(suggestion))
	}
	return responses
}

func toInvestmentAutomationRuleResponse(rule app.InvestmentAutomationRule) investmentAutomationRuleResponse {
	return investmentAutomationRuleResponse{
		ID:                     rule.ID,
		BookID:                 rule.BookID,
		SourceID:               rule.SourceID,
		InstrumentID:           rule.InstrumentID,
		EventFamily:            rule.EventFamily,
		Mode:                   rule.Mode,
		ConfidenceThresholdBPS: rule.ConfidenceThresholdBPS,
		RequiredAccounts:       json.RawMessage(rule.RequiredAccountsJSON),
		Status:                 rule.Status,
		EffectiveFrom:          rule.EffectiveFrom,
		EffectiveTo:            rule.EffectiveTo,
		CreatedAt:              rule.CreatedAt,
		UpdatedAt:              rule.UpdatedAt,
	}
}

func toInvestmentAutomationRuleResponses(rules []app.InvestmentAutomationRule) []investmentAutomationRuleResponse {
	responses := make([]investmentAutomationRuleResponse, 0, len(rules))
	for _, rule := range rules {
		responses = append(responses, toInvestmentAutomationRuleResponse(rule))
	}
	return responses
}

type realizedGainResponse struct {
	AccountID          int64             `json:"account_id"`
	CommodityID        int64             `json:"commodity_id"`
	CostCommodityID    int64             `json:"cost_commodity_id"`
	DisposalDate       string            `json:"disposal_date"`
	TransactionID      *int64            `json:"transaction_id,omitempty"`
	QuantityValue      exact.Coefficient `json:"quantity_value"`
	QuantityScale      int               `json:"quantity_scale"`
	DisposedBasisValue int64             `json:"disposed_basis_value"`
	DisposedBasisScale int               `json:"disposed_basis_scale"`
	ProceedsValue      int64             `json:"proceeds_value"`
	ProceedsScale      int               `json:"proceeds_scale"`
	RealizedGainValue  int64             `json:"realized_gain_value"`
}

type unrealizedGainResponse struct {
	AccountID               int64             `json:"account_id"`
	CommodityID             int64             `json:"commodity_id"`
	CostCommodityID         int64             `json:"cost_commodity_id"`
	QuantityValue           exact.Coefficient `json:"quantity_value"`
	QuantityScale           int               `json:"quantity_scale"`
	RemainingCostBasisValue int64             `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale int               `json:"remaining_cost_basis_scale"`
	LatestPriceValue        *int64            `json:"latest_price_value,omitempty"`
	LatestPriceScale        *int              `json:"latest_price_scale,omitempty"`
	LatestPriceDate         string            `json:"latest_price_date,omitempty"`
	MarketValueValue        *int64            `json:"market_value_value,omitempty"`
	MarketValueScale        *int              `json:"market_value_scale,omitempty"`
	UnrealizedGainValue     *int64            `json:"unrealized_gain_value,omitempty"`
	UnrealizedGainScale     *int              `json:"unrealized_gain_scale,omitempty"`
}

type realizedGainTotalResponse struct {
	CostCommodityID int64 `json:"cost_commodity_id"`
	TotalGainValue  int64 `json:"total_gain_value"`
	TotalGainScale  int   `json:"total_gain_scale"`
}

type investmentGainsResponse struct {
	Realized       []realizedGainResponse      `json:"realized"`
	Unrealized     []unrealizedGainResponse    `json:"unrealized"`
	RealizedTotals []realizedGainTotalResponse `json:"realized_totals"`
}

func listInvestmentGains(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		params := app.GainsReportParams{
			From: r.URL.Query().Get("from"),
			To:   r.URL.Query().Get("to"),
		}
		realized, err := investmentService.ListRealizedGains(r.Context(), params)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list realized gains", err)
			return
		}
		unrealized, err := investmentService.ListUnrealizedGains(r.Context())
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "list unrealized gains", err)
			return
		}
		realizedResponses := make([]realizedGainResponse, 0, len(realized))
		for _, e := range realized {
			realizedResponses = append(realizedResponses, realizedGainResponse{
				AccountID:          e.AccountID,
				CommodityID:        e.CommodityID,
				CostCommodityID:    e.CostCommodityID,
				DisposalDate:       e.DisposalDate,
				TransactionID:      e.TransactionID,
				QuantityValue:      e.QuantityValue,
				QuantityScale:      e.QuantityScale,
				DisposedBasisValue: e.DisposedBasisValue,
				DisposedBasisScale: e.DisposedBasisScale,
				ProceedsValue:      e.ProceedsValue,
				ProceedsScale:      e.ProceedsScale,
				RealizedGainValue:  e.RealizedGainValue,
			})
		}
		unrealizedResponses := make([]unrealizedGainResponse, 0, len(unrealized))
		for _, e := range unrealized {
			unrealizedResponses = append(unrealizedResponses, unrealizedGainResponse{
				AccountID:               e.AccountID,
				CommodityID:             e.CommodityID,
				CostCommodityID:         e.CostCommodityID,
				QuantityValue:           e.QuantityValue,
				QuantityScale:           e.QuantityScale,
				RemainingCostBasisValue: e.RemainingCostBasisValue,
				RemainingCostBasisScale: e.RemainingCostBasisScale,
				LatestPriceValue:        e.LatestPriceValue,
				LatestPriceScale:        e.LatestPriceScale,
				LatestPriceDate:         e.LatestPriceDate,
				MarketValueValue:        e.MarketValueValue,
				MarketValueScale:        e.MarketValueScale,
				UnrealizedGainValue:     e.UnrealizedGainValue,
				UnrealizedGainScale:     e.UnrealizedGainScale,
			})
		}

		// Compute realized totals grouped by (cost_commodity_id, proceeds_scale).
		// All entries sharing the same currency and scale can be summed directly.
		type totalKey struct {
			costCommodityID int64
			scale           int
		}
		totalsMap := map[totalKey]int64{}
		var totalsOrder []totalKey
		for _, e := range realized {
			k := totalKey{e.CostCommodityID, e.ProceedsScale}
			if _, exists := totalsMap[k]; !exists {
				totalsOrder = append(totalsOrder, k)
			}
			totalsMap[k] += e.RealizedGainValue
		}
		realizedTotals := make([]realizedGainTotalResponse, 0, len(totalsOrder))
		for _, k := range totalsOrder {
			realizedTotals = append(realizedTotals, realizedGainTotalResponse{
				CostCommodityID: k.costCommodityID,
				TotalGainValue:  totalsMap[k],
				TotalGainScale:  k.scale,
			})
		}
		writeJSON(w, http.StatusOK, investmentGainsResponse{
			Realized:       realizedResponses,
			Unrealized:     unrealizedResponses,
			RealizedTotals: realizedTotals,
		})
	}
}
