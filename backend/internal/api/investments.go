package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"rekenraam/backend/internal/app"
	"rekenraam/backend/internal/db"
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
	ID                      int64             `json:"id"`
	BookID                  int64             `json:"book_id"`
	CommodityID             *int64            `json:"commodity_id,omitempty"`
	IncomeAccountID         int64             `json:"income_account_id"`
	WithholdingAccountID    *int64            `json:"withholding_account_id,omitempty"`
	DefaultWithholdingValue *moneyCoefficient `json:"default_withholding_value,omitempty"`
	DefaultWithholdingScale *int              `json:"default_withholding_scale,omitempty"`
	WithholdingRateBPS      *int64            `json:"withholding_rate_bps,omitempty"`
	TaxCountryCode          string            `json:"tax_country_code,omitempty"`
	TaxTreatment            string            `json:"tax_treatment,omitempty"`
	Status                  string            `json:"status"`
	EffectiveFrom           string            `json:"effective_from"`
	EffectiveTo             string            `json:"effective_to,omitempty"`
	Metadata                json.RawMessage   `json:"metadata"`
	CreatedAt               string            `json:"created_at"`
	UpdatedAt               string            `json:"updated_at"`
}

type dividendDefaultsResponse struct {
	Defaults []dividendDefaultResponse `json:"defaults"`
}

type dividendDefaultRequest struct {
	CommodityID             *int64            `json:"commodity_id"`
	IncomeAccountID         int64             `json:"income_account_id"`
	WithholdingAccountID    *int64            `json:"withholding_account_id"`
	DefaultWithholdingValue *moneyCoefficient `json:"default_withholding_value"`
	DefaultWithholdingScale *int              `json:"default_withholding_scale"`
	WithholdingRateBPS      *int64            `json:"withholding_rate_bps"`
	TaxCountryCode          string            `json:"tax_country_code"`
	TaxTreatment            string            `json:"tax_treatment"`
	Status                  string            `json:"status"`
	EffectiveFrom           string            `json:"effective_from"`
	EffectiveTo             string            `json:"effective_to"`
	Metadata                json.RawMessage   `json:"metadata"`
	ChangeReason            string            `json:"change_reason"`
}

type investmentTradeRequest struct {
	TransactionDate    string                           `json:"transaction_date"`
	CommodityID        int64                            `json:"commodity_id"`
	HoldingAccountID   int64                            `json:"holding_account_id"`
	CashAccountID      int64                            `json:"cash_account_id"`
	QuantityValue      exact.Coefficient                `json:"quantity_value"`
	QuantityScale      int                              `json:"quantity_scale"`
	CashAmountValue    moneyCoefficient                 `json:"cash_amount_value"`
	CashAmountScale    int                              `json:"cash_amount_scale"`
	CashCommodityID    int64                            `json:"cash_commodity_id"`
	GrossAmountValue   *moneyCoefficient                `json:"gross_amount_value,omitempty"`
	GrossAmountScale   int                              `json:"gross_amount_scale,omitempty"`
	NetSettlementValue *moneyCoefficient                `json:"net_settlement_value,omitempty"`
	NetSettlementScale int                              `json:"net_settlement_scale,omitempty"`
	SettlementDate     string                           `json:"settlement_date,omitempty"`
	Charges            []investmentTradeChargeRequest   `json:"charges,omitempty"`
	Memo               string                           `json:"memo"`
	PayeeID            *int64                           `json:"payee_id"`
	Status             string                           `json:"status"`
	LotAllocations     []investmentLotAllocationRequest `json:"lot_allocations"`
	ChangeReason       string                           `json:"change_reason"`
	CostBasisMethod    string                           `json:"cost_basis_method"`
	// ReconciliationOverride lets a backdated trade proceed into a reconciled
	// period, invalidating the affected checkpoints. The service has always
	// honoured it; without it on the wire a buy, sell, or write-off dated
	// before a checkpoint was simply unreachable (T-53).
	ReconciliationOverride bool `json:"reconciliation_override"`
	// GainImpactAcknowledgement echoes the preview token for the committed
	// disposal gain changes the user accepted (T-114).
	GainImpactAcknowledgement string `json:"gain_impact_acknowledgement,omitempty"`
}

type externalTransferInRequest struct {
	EffectiveOn               string            `json:"effective_on"`
	HoldingAccountID          int64             `json:"holding_account_id"`
	CommodityID               int64             `json:"commodity_id"`
	QuantityValue             exact.Coefficient `json:"quantity_value"`
	QuantityScale             int               `json:"quantity_scale"`
	CarriedBasisValue         *moneyCoefficient `json:"carried_basis_value"`
	CarriedBasisScale         int               `json:"carried_basis_scale"`
	CostCommodityID           int64             `json:"cost_commodity_id"`
	OriginalAcquiredOn        string            `json:"original_acquired_on"`
	SourceEvidence            json.RawMessage   `json:"source_evidence,omitempty"`
	Memo                      string            `json:"memo"`
	ChangeReason              string            `json:"change_reason"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

type externalTransferInResponse struct {
	Transaction transactionResponse `json:"transaction"`
	LotID       int64               `json:"lot_id"`
}

type internalTransferRequest struct {
	EffectiveOn          string                           `json:"effective_on"`
	SourceAccountID      int64                            `json:"source_account_id"`
	DestinationAccountID int64                            `json:"destination_account_id"`
	CommodityID          int64                            `json:"commodity_id"`
	CostCommodityID      int64                            `json:"cost_commodity_id"`
	LotAllocations       []investmentLotAllocationRequest `json:"lot_allocations"`
	// QuantityValue/Scale move a total out of an average-cost pool instead of
	// selected lots (T-123).
	QuantityValue             exact.Coefficient `json:"quantity_value,omitempty"`
	QuantityScale             int               `json:"quantity_scale,omitempty"`
	SourceEvidence            json.RawMessage   `json:"source_evidence,omitempty"`
	Memo                      string            `json:"memo"`
	ChangeReason              string            `json:"change_reason"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
}

type internalTransferLinkResponse struct {
	SourceLotID           int64             `json:"source_lot_id"`
	DestinationLotID      *int64            `json:"destination_lot_id"`
	QuantityValue         exact.Coefficient `json:"quantity_value"`
	QuantityScale         int               `json:"quantity_scale"`
	CarriedBasisValue     moneyCoefficient  `json:"carried_basis_value"`
	CarriedBasisScale     int               `json:"carried_basis_scale"`
	OriginalDateKnowledge string            `json:"original_date_knowledge"`
	OriginalAcquiredOn    *string           `json:"original_acquired_on"`
}

type internalTransferPlanResponse struct {
	BasisAllocation string                         `json:"basis_allocation"`
	CostBasisMethod string                         `json:"cost_basis_method"`
	ResolutionTier  string                         `json:"resolution_tier"`
	Links           []internalTransferLinkResponse `json:"links"`
}

type internalTransferPreviewResponse struct {
	Plan   internalTransferPlanResponse `json:"plan"`
	Impact reconciliationImpactResponse `json:"impact"`
}

type internalTransferResponse struct {
	Transaction       transactionResponse          `json:"transaction"`
	Plan              internalTransferPlanResponse `json:"plan"`
	DestinationLotIDs []int64                      `json:"destination_lot_ids"`
}

type investmentSaleReversalRequest struct {
	Reason                    string `json:"reason"`
	ReconciliationOverride    bool   `json:"reconciliation_override"`
	GainImpactAcknowledgement string `json:"gain_impact_acknowledgement,omitempty"`
}

type investmentSaleReversalResponse struct {
	Transaction            transactionResponse `json:"transaction"`
	CorrectedTransactionID int64               `json:"corrected_transaction_id"`
}

type investmentSaleReplacementRequest struct {
	Reason                    string                 `json:"reason"`
	ReconciliationOverride    bool                   `json:"reconciliation_override"`
	Replacement               investmentTradeRequest `json:"replacement"`
	GainImpactAcknowledgement string                 `json:"gain_impact_acknowledgement,omitempty"`
}

type investmentSaleReplacementResponse struct {
	InverseTransaction     transactionResponse     `json:"inverse_transaction"`
	Replacement            investmentTradeResponse `json:"replacement"`
	CorrectedTransactionID int64                   `json:"corrected_transaction_id"`
}

type investmentBuyReplacementRequest struct {
	Reason                    string                 `json:"reason"`
	ReconciliationOverride    bool                   `json:"reconciliation_override"`
	Replacement               investmentTradeRequest `json:"replacement"`
	GainImpactAcknowledgement string                 `json:"gain_impact_acknowledgement,omitempty"`
}

type investmentBuyReplacementResponse struct {
	InverseTransaction     transactionResponse     `json:"inverse_transaction"`
	Replacement            investmentTradeResponse `json:"replacement"`
	CorrectedTransactionID int64                   `json:"corrected_transaction_id"`
}

type investmentCorrectionNodeResponse struct {
	OperationID             int64  `json:"operation_id"`
	TransactionID           *int64 `json:"transaction_id,omitempty"`
	OperationKind           string `json:"operation_kind"`
	EventDate               string `json:"event_date"`
	CorrectionOfOperationID *int64 `json:"correction_of_operation_id,omitempty"`
	CorrectionMode          string `json:"correction_mode,omitempty"`
	CorrectionReason        string `json:"correction_reason,omitempty"`
	CreatedAt               string `json:"created_at"`
	AuditEventID            int64  `json:"audit_event_id"`
	Imported                bool   `json:"imported"`
	Effective               bool   `json:"effective"`
}

type investmentCorrectionChainResponse struct {
	RootOperationID        int64                              `json:"root_operation_id"`
	EffectiveTransactionID *int64                             `json:"effective_transaction_id"`
	CanReverseManualSale   bool                               `json:"can_reverse_manual_sale"`
	CanReverseManualBuy    bool                               `json:"can_reverse_manual_buy"`
	CanReverseSale         bool                               `json:"can_reverse_sale"`
	CanReverseBuy          bool                               `json:"can_reverse_buy"`
	Operations             []investmentCorrectionNodeResponse `json:"operations"`
}

type investmentTradeCorrectionChargeResponse struct {
	Kind            string `json:"kind"`
	AmountValue     string `json:"amount_value"`
	AmountScale     int    `json:"amount_scale"`
	CommodityID     int64  `json:"commodity_id"`
	Treatment       string `json:"treatment"`
	ChargeAccountID *int64 `json:"charge_account_id,omitempty"`
	CashAccountID   *int64 `json:"cash_account_id,omitempty"`
	PaidOn          string `json:"paid_on"`
}

type investmentTradeCorrectionLotChoiceResponse struct {
	LotID         int64  `json:"lot_id"`
	QuantityValue string `json:"quantity_value"`
	QuantityScale int    `json:"quantity_scale"`
}

type investmentTradeCorrectionAvailableLotResponse struct {
	LotID         int64  `json:"lot_id"`
	OpenedOn      string `json:"opened_on"`
	QuantityValue string `json:"quantity_value"`
	QuantityScale int    `json:"quantity_scale"`
}

type investmentTradeCorrectionContextResponse struct {
	OperationID          int64                                           `json:"operation_id"`
	TransactionID        int64                                           `json:"transaction_id"`
	OperationKind        string                                          `json:"operation_kind"`
	EventDate            string                                          `json:"event_date"`
	HoldingAccountID     int64                                           `json:"holding_account_id"`
	CommodityID          int64                                           `json:"commodity_id"`
	CommodityCode        string                                          `json:"commodity_code"`
	CostCommodityID      int64                                           `json:"cost_commodity_id"`
	QuantityValue        string                                          `json:"quantity_value"`
	QuantityScale        int                                             `json:"quantity_scale"`
	CostBasisMethod      string                                          `json:"cost_basis_method"`
	CashAccountID        int64                                           `json:"cash_account_id"`
	NetValue             string                                          `json:"net_value"`
	NetScale             int                                             `json:"net_scale"`
	SettlementDate       string                                          `json:"settlement_date"`
	Memo                 string                                          `json:"memo"`
	PayeeID              *int64                                          `json:"payee_id,omitempty"`
	GrossValue           *string                                         `json:"gross_value,omitempty"`
	GrossScale           *int                                            `json:"gross_scale,omitempty"`
	Imported             bool                                            `json:"imported"`
	SourceIdentityID     int64                                           `json:"source_identity_id"`
	SourceKind           string                                          `json:"source_kind"`
	AlreadyCorrected     bool                                            `json:"already_corrected"`
	Charges              []investmentTradeCorrectionChargeResponse       `json:"charges"`
	ElectedLots          []investmentTradeCorrectionLotChoiceResponse    `json:"elected_lots"`
	CanReplaceSale       bool                                            `json:"can_replace_sale"`
	AvailableLots        []investmentTradeCorrectionAvailableLotResponse `json:"available_lots"`
	EffectiveElectedLots []investmentTradeCorrectionLotChoiceResponse    `json:"effective_elected_lots"`
}

func toInvestmentTradeCorrectionContextResponse(record db.InvestmentTradeCorrectionContext) investmentTradeCorrectionContextResponse {
	charges := make([]investmentTradeCorrectionChargeResponse, 0, len(record.Charges))
	for _, charge := range record.Charges {
		charges = append(charges, investmentTradeCorrectionChargeResponse{
			Kind: charge.Kind, AmountValue: charge.AmountValue, AmountScale: charge.AmountScale,
			CommodityID: charge.CommodityID, Treatment: charge.Treatment,
			ChargeAccountID: charge.ChargeAccountID, CashAccountID: charge.CashAccountID,
			PaidOn: charge.PaidOn,
		})
	}
	electedLots := make([]investmentTradeCorrectionLotChoiceResponse, 0, len(record.ElectedLots))
	for _, choice := range record.ElectedLots {
		electedLots = append(electedLots, investmentTradeCorrectionLotChoiceResponse{
			LotID: choice.LotID, QuantityValue: choice.QuantityValue, QuantityScale: choice.QuantityScale,
		})
	}
	effectiveElectedLots := make([]investmentTradeCorrectionLotChoiceResponse, 0, len(record.EffectiveElectedLots))
	for _, choice := range record.EffectiveElectedLots {
		effectiveElectedLots = append(effectiveElectedLots, investmentTradeCorrectionLotChoiceResponse{
			LotID: choice.LotID, QuantityValue: choice.QuantityValue, QuantityScale: choice.QuantityScale,
		})
	}
	availableLots := make([]investmentTradeCorrectionAvailableLotResponse, 0, len(record.AvailableLots))
	for _, lot := range record.AvailableLots {
		availableLots = append(availableLots, investmentTradeCorrectionAvailableLotResponse{
			LotID: lot.LotID, OpenedOn: lot.OpenedOn, QuantityValue: lot.QuantityValue,
			QuantityScale: lot.QuantityScale,
		})
	}
	return investmentTradeCorrectionContextResponse{
		OperationID: record.OperationID, TransactionID: record.TransactionID,
		OperationKind: record.OperationKind, EventDate: record.EventDate,
		HoldingAccountID: record.HoldingAccountID, CommodityID: record.CommodityID,
		CommodityCode: record.CommodityCode, CostCommodityID: record.CostCommodityID,
		QuantityValue: record.QuantityValue, QuantityScale: record.QuantityScale,
		CostBasisMethod: record.CostBasisMethod, CashAccountID: record.CashAccountID,
		NetValue: record.NetValue, NetScale: record.NetScale,
		SettlementDate: record.SettlementDate, GrossValue: record.GrossValue,
		GrossScale: record.GrossScale, Memo: record.Memo, PayeeID: record.PayeeID,
		Imported:         record.Imported,
		SourceIdentityID: record.SourceIdentityID, SourceKind: record.SourceKind,
		AlreadyCorrected: record.AlreadyCorrected, Charges: charges, ElectedLots: electedLots,
		CanReplaceSale: record.CanReplaceSale, AvailableLots: availableLots,
		EffectiveElectedLots: effectiveElectedLots,
	}
}

type investmentTradeChargeRequest struct {
	Kind            string           `json:"kind"`
	AmountValue     moneyCoefficient `json:"amount_value"`
	AmountScale     int              `json:"amount_scale"`
	CommodityID     int64            `json:"commodity_id"`
	Treatment       string           `json:"treatment,omitempty"`
	ChargeAccountID *int64           `json:"charge_account_id,omitempty"`
	CashAccountID   *int64           `json:"cash_account_id,omitempty"`
	PaidOn          string           `json:"paid_on,omitempty"`
	SourceEvidence  json.RawMessage  `json:"source_evidence,omitempty"`
}

// investmentWriteOffRequest has no cash account, commodity or amount: a
// write-off's proceeds are zero by definition (T-38).
type investmentWriteOffRequest struct {
	TransactionDate  string                           `json:"transaction_date"`
	CommodityID      int64                            `json:"commodity_id"`
	HoldingAccountID int64                            `json:"holding_account_id"`
	QuantityValue    exact.Coefficient                `json:"quantity_value"`
	QuantityScale    int                              `json:"quantity_scale"`
	Reason           string                           `json:"reason"`
	Memo             string                           `json:"memo"`
	PayeeID          *int64                           `json:"payee_id"`
	Status           string                           `json:"status"`
	LotAllocations   []investmentLotAllocationRequest `json:"lot_allocations"`
	ChangeReason     string                           `json:"change_reason"`
	CostBasisMethod  string                           `json:"cost_basis_method"`
	// ReconciliationOverride lets a backdated write-off proceed into a
	// reconciled period, invalidating the affected checkpoints (T-53).
	ReconciliationOverride bool `json:"reconciliation_override"`
	// GainImpactAcknowledgement echoes the preview token when a backdated
	// write-off revises committed gains (T-117).
	GainImpactAcknowledgement string `json:"gain_impact_acknowledgement,omitempty"`
}

type sellPreviewResponse struct {
	CostBasisMethod    string                          `json:"cost_basis_method"`
	DisposalDecision   disposalDecisionResponse        `json:"disposal_decision"`
	Allocations        []investmentLotDisposalResponse `json:"allocations"`
	RealizedGain       moneyCoefficient                `json:"realized_gain"`
	RealizedGainScale  int                             `json:"realized_gain_scale"`
	CashAmountValue    moneyCoefficient                `json:"cash_amount_value"`
	CashAmountScale    int                             `json:"cash_amount_scale"`
	GrossAmountValue   *moneyCoefficient               `json:"gross_amount_value,omitempty"`
	GrossAmountScale   int                             `json:"gross_amount_scale"`
	NetSettlementValue moneyCoefficient                `json:"net_settlement_value"`
	NetSettlementScale int                             `json:"net_settlement_scale"`
	SettlementDate     string                          `json:"settlement_date"`
	Charges            []investmentTradeChargeRequest  `json:"charges"`
}

type investmentLotAllocationRequest struct {
	LotID         int64             `json:"lot_id"`
	QuantityValue exact.Coefficient `json:"quantity_value"`
	QuantityScale int               `json:"quantity_scale"`
}

type investmentTradeResponse struct {
	Transaction      transactionResponse             `json:"transaction"`
	LotID            *int64                          `json:"lot_id,omitempty"`
	Allocations      []investmentLotDisposalResponse `json:"allocations"`
	DisposalDecision *disposalDecisionResponse       `json:"disposal_decision,omitempty"`
}

type disposalDecisionResponse struct {
	ID                   *int64                          `json:"id,omitempty"`
	TransactionID        *int64                          `json:"transaction_id,omitempty"`
	TransactionVersionID *int64                          `json:"transaction_version_id,omitempty"`
	CostBasisMethod      string                          `json:"cost_basis_method"`
	ResolutionTier       string                          `json:"resolution_tier"`
	AccountVersionID     *int64                          `json:"account_version_id,omitempty"`
	ProfileID            *int64                          `json:"profile_id,omitempty"`
	ProfileVersionID     *int64                          `json:"profile_version_id,omitempty"`
	SourceEffectiveFrom  string                          `json:"source_effective_from,omitempty"`
	SourceRecordedAt     string                          `json:"source_recorded_at,omitempty"`
	QuantityValue        exact.Coefficient               `json:"quantity_value"`
	QuantityScale        int                             `json:"quantity_scale"`
	DisposedBasisValue   exact.Coefficient               `json:"disposed_basis_value"`
	DisposedBasisScale   int                             `json:"disposed_basis_scale"`
	ProceedsValue        moneyCoefficient                `json:"proceeds_value"`
	ProceedsScale        int                             `json:"proceeds_scale"`
	CostCommodityID      int64                           `json:"cost_commodity_id"`
	AuditEventID         *int64                          `json:"audit_event_id,omitempty"`
	Allocations          []investmentLotDisposalResponse `json:"allocations"`
}

type investmentLotDisposalResponse struct {
	LotID          int64             `json:"lot_id"`
	QuantityValue  exact.Coefficient `json:"quantity_value"`
	QuantityScale  int               `json:"quantity_scale"`
	CostBasisValue moneyCoefficient  `json:"cost_basis_value"`
	CostBasisScale int               `json:"cost_basis_scale"`
	ProceedsValue  moneyCoefficient  `json:"proceeds_value"`
	ProceedsScale  int               `json:"proceeds_scale"`
}

type dividendRequest struct {
	TransactionDate        string            `json:"transaction_date"`
	CommodityID            *int64            `json:"commodity_id"`
	CashAccountID          int64             `json:"cash_account_id"`
	CashCommodityID        int64             `json:"cash_commodity_id"`
	IncomeAccountID        *int64            `json:"income_account_id"`
	AmountValue            moneyCoefficient  `json:"amount_value"`
	AmountScale            int               `json:"amount_scale"`
	WithholdingValue       *moneyCoefficient `json:"withholding_value"`
	WithholdingScale       *int              `json:"withholding_scale"`
	WithholdingAccountID   *int64            `json:"withholding_account_id"`
	Memo                   string            `json:"memo"`
	PayeeID                *int64            `json:"payee_id"`
	Status                 string            `json:"status"`
	ChangeReason           string            `json:"change_reason"`
	ReconciliationOverride bool              `json:"reconciliation_override"`
}

type reinvestedDividendRequest struct {
	TransactionDate           string            `json:"transaction_date"`
	CommodityID               int64             `json:"commodity_id"`
	HoldingAccountID          int64             `json:"holding_account_id"`
	IncomeAccountID           *int64            `json:"income_account_id"`
	QuantityValue             exact.Coefficient `json:"quantity_value"`
	QuantityScale             int               `json:"quantity_scale"`
	AmountValue               moneyCoefficient  `json:"amount_value"`
	AmountScale               int               `json:"amount_scale"`
	CashCommodityID           int64             `json:"cash_commodity_id"`
	Memo                      string            `json:"memo"`
	PayeeID                   *int64            `json:"payee_id"`
	Status                    string            `json:"status"`
	ChangeReason              string            `json:"change_reason"`
	ReconciliationOverride    bool              `json:"reconciliation_override"`
	GainImpactAcknowledgement string            `json:"gain_impact_acknowledgement,omitempty"`
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
	CostBasisValue          moneyCoefficient  `json:"cost_basis_value"`
	CostBasisScale          int               `json:"cost_basis_scale"`
	RemainingCostBasisValue *moneyCoefficient `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale *int              `json:"remaining_cost_basis_scale"`
	BasisKnowledge          string            `json:"basis_knowledge"`
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
	RemainingCostBasisValue *moneyCoefficient `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale *int              `json:"remaining_cost_basis_scale"`
	BasisKnowledge          string            `json:"basis_knowledge"`
	CostCommodityID         int64             `json:"cost_commodity_id"`
	LatestPriceValue        *moneyCoefficient `json:"latest_price_value,omitempty"`
	LatestPriceScale        *int              `json:"latest_price_scale,omitempty"`
	LatestPriceDate         string            `json:"latest_price_date,omitempty"`
	LatestPriceApproximate  bool              `json:"latest_price_approximate"`
	TransferBasisAllocation string            `json:"transfer_basis_allocation"`
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
			WithholdingAccountID: request.WithholdingAccountID, DefaultWithholdingValue: moneyInt64Pointer(request.DefaultWithholdingValue),
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

func externalTransferInInput(owner app.Owner, r *http.Request, request externalTransferInRequest) (app.ExternalTransferInInput, error) {
	if request.CarriedBasisValue == nil {
		return app.ExternalTransferInInput{}, app.ValidationError{Message: "known carried basis is required"}
	}
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.ExternalTransferInInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		EffectiveOn: request.EffectiveOn, HoldingAccountID: request.HoldingAccountID,
		CommodityID: request.CommodityID, QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		CarriedBasisValue: int64(*request.CarriedBasisValue), CarriedBasisScale: request.CarriedBasisScale,
		CostCommodityID: request.CostCommodityID, OriginalAcquiredOn: request.OriginalAcquiredOn,
		SourceEvidenceJSON: evidence, Memo: request.Memo,
		ChangeReason: request.ChangeReason, ReconciliationOverride: request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}, nil
}

func externalTransferIn(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request externalTransferInRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input, err := externalTransferInInput(owner, r, request)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "external investment transfer", err)
			return
		}
		result, err := investmentService.ExternalTransferIn(r.Context(), input)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "external investment transfer", err)
			return
		}
		writeJSON(w, http.StatusCreated, externalTransferInResponse{Transaction: toTransactionResponse(result.Transaction), LotID: *result.LotID})
	}))
}

func externalTransferInReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request externalTransferInRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		input, err := externalTransferInInput(owner, r, request)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "external investment transfer impact", err)
			return
		}
		impact, err := investmentService.PreviewExternalTransferInReconciliationImpact(r.Context(), input)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "external investment transfer impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func internalTransferInput(owner app.Owner, r *http.Request, request internalTransferRequest) app.InternalTransferInput {
	allocations := make([]app.InvestmentLotAllocationInput, 0, len(request.LotAllocations))
	for _, allocation := range request.LotAllocations {
		allocations = append(allocations, app.InvestmentLotAllocationInput{
			LotID: allocation.LotID, QuantityValue: allocation.QuantityValue,
			QuantityScale: allocation.QuantityScale,
		})
	}
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.InternalTransferInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
		RequestID: RequestIDFromContext(r.Context()), EffectiveOn: request.EffectiveOn,
		SourceAccountID: request.SourceAccountID, DestinationAccountID: request.DestinationAccountID,
		CommodityID: request.CommodityID, CostCommodityID: request.CostCommodityID,
		Allocations: allocations, QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		SourceEvidenceJSON: evidence, Memo: request.Memo,
		ChangeReason: request.ChangeReason, ReconciliationOverride: request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toInternalTransferPlanResponse(plan app.InternalTransferPlan) internalTransferPlanResponse {
	out := internalTransferPlanResponse{BasisAllocation: plan.BasisAllocation, CostBasisMethod: plan.CostBasisMethod,
		ResolutionTier: plan.ResolutionTier, Links: make([]internalTransferLinkResponse, 0, len(plan.Links))}
	for _, link := range plan.Links {
		response := internalTransferLinkResponse{SourceLotID: link.SourceLotID,
			QuantityValue: link.QuantityValue, QuantityScale: link.QuantityScale,
			CarriedBasisValue: moneyCoefficient(link.CarriedBasisValue), CarriedBasisScale: link.CarriedBasisScale,
			OriginalDateKnowledge: link.OriginalDateKnowledge}
		if link.DestinationLotID > 0 {
			id := link.DestinationLotID
			response.DestinationLotID = &id
		}
		if link.OriginalAcquiredOn != "" {
			date := link.OriginalAcquiredOn
			response.OriginalAcquiredOn = &date
		}
		out.Links = append(out.Links, response)
	}
	return out
}

func internalTransfer(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request internalTransferRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.InternalTransfer(r.Context(), internalTransferInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "internal investment transfer", err)
			return
		}
		writeJSON(w, http.StatusCreated, internalTransferResponse{
			Transaction: toTransactionResponse(result.Transaction), Plan: toInternalTransferPlanResponse(result.Plan),
			DestinationLotIDs: result.DestinationLotIDs,
		})
	}))
}

func internalTransferPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request internalTransferRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewInternalTransfer(r.Context(), internalTransferInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "internal investment transfer preview", err)
			return
		}
		impact := toReconciliationImpactResponse(preview.Impact)
		gainImpact, err := toGainImpactResponse(preview.Impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		impact.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, internalTransferPreviewResponse{
			Plan: toInternalTransferPlanResponse(preview.Plan), Impact: impact,
		})
	}
}

func internalTransferReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request internalTransferRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.PreviewInternalTransferReconciliationImpact(r.Context(), internalTransferInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "internal investment transfer impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func investmentCorrectionChain(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		chain, err := investmentService.CorrectionChain(r.Context(), owner.ID, transactionID)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read investment correction chain", err)
			return
		}
		operations := make([]investmentCorrectionNodeResponse, 0, len(chain.Operations))
		for _, node := range chain.Operations {
			operations = append(operations, investmentCorrectionNodeResponse{
				OperationID: node.OperationID, TransactionID: node.TransactionID,
				OperationKind: node.OperationKind, EventDate: node.EventDate,
				CorrectionOfOperationID: node.CorrectionOfOperationID, CorrectionMode: node.CorrectionMode,
				CorrectionReason: node.CorrectionReason, CreatedAt: node.CreatedAt,
				AuditEventID: node.AuditEventID, Imported: node.Imported, Effective: node.Effective,
			})
		}
		writeJSON(w, http.StatusOK, investmentCorrectionChainResponse{
			RootOperationID: chain.RootOperationID, EffectiveTransactionID: chain.EffectiveTransactionID,
			CanReverseManualSale: chain.CanReverseManualSale, CanReverseManualBuy: chain.CanReverseManualBuy,
			CanReverseSale: chain.CanReverseSale, CanReverseBuy: chain.CanReverseBuy,
			Operations: operations,
		})
	}
}

func investmentTradeCorrectionContext(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		context, err := investmentService.TradeCorrectionContext(r.Context(), owner.ID, transactionID)
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "read investment trade correction context", err)
			return
		}
		writeJSON(w, http.StatusOK, toInvestmentTradeCorrectionContextResponse(context))
	}
}

func reverseInvestmentSale(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := investmentService.ReverseSale(r.Context(), app.ReverseInvestmentSaleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			OriginType: "browser_api", TransactionID: transactionID, Reason: request.Reason,
			ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse investment sale", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseInvestmentSaleReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.ReverseSaleReconciliationImpact(r.Context(), app.ReverseInvestmentSaleInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview sale reversal reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func reverseInvestmentBuy(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		transaction, err := investmentService.ReverseBuy(r.Context(), app.ReverseInvestmentBuyInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
			TransactionID: transactionID, Reason: request.Reason,
			ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "reverse investment buy", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReversalResponse{Transaction: toTransactionResponse(transaction), CorrectedTransactionID: transactionID})
	}))
}

func reverseInvestmentBuyReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReversalRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.ReverseBuyReconciliationImpact(r.Context(), app.ReverseInvestmentBuyInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview buy reversal reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func replaceInvestmentSale(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ReplaceSale(r.Context(), app.ReplaceInvestmentSaleInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), TransactionID: transactionID,
			Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			Replacement:               toInvestmentTradeInput(owner, r, request.Replacement),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace investment sale", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSaleReplacementResponse{
			InverseTransaction:     toTransactionResponse(result.Inverse),
			Replacement:            toInvestmentTradeResponse(result.Replacement),
			CorrectedTransactionID: transactionID,
		})
	}))
}

func replaceInvestmentSaleReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentSaleReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.ReplaceSaleReconciliationImpact(r.Context(), app.ReplaceInvestmentSaleInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
			Replacement: toInvestmentTradeInput(owner, r, request.Replacement),
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview sale replacement reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
}

func replaceInvestmentBuy(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentBuyReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.ReplaceBuy(r.Context(), app.ReplaceInvestmentBuyInput{
			OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
			RequestID: RequestIDFromContext(r.Context()), TransactionID: transactionID,
			Reason: request.Reason, ReconciliationOverride: request.ReconciliationOverride,
			Replacement:               toInvestmentTradeInput(owner, r, request.Replacement),
			GainImpactAcknowledgement: request.GainImpactAcknowledgement,
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "replace investment buy", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentBuyReplacementResponse{
			InverseTransaction:     toTransactionResponse(result.Inverse),
			Replacement:            toInvestmentTradeResponse(result.Replacement),
			CorrectedTransactionID: transactionID,
		})
	}))
}

func replaceInvestmentBuyReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		transactionID, ok := readPathInt64(w, r, "transaction_id", "transaction id")
		if !ok {
			return
		}
		var request investmentBuyReplacementRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.ReplaceBuyReconciliationImpact(r.Context(), app.ReplaceInvestmentBuyInput{
			OwnerUserID: owner.ID, TransactionID: transactionID, Reason: request.Reason,
			Replacement: toInvestmentTradeInput(owner, r, request.Replacement),
		})
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview buy replacement reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
	}
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
			DisposalDecision:  toDisposalDecisionResponse(preview.DisposalDecision),
			Allocations:       toInvestmentLotDisposalResponses(preview.Allocations),
			RealizedGain:      moneyCoefficient(preview.RealizedGain),
			RealizedGainScale: preview.RealizedGainScale,
			CashAmountValue:   moneyCoefficient(preview.CashAmountValue),
			CashAmountScale:   preview.CashAmountScale,
			GrossAmountValue:  moneyCoefficientPointer(preview.GrossAmountValue), GrossAmountScale: preview.GrossAmountScale,
			NetSettlementValue: moneyCoefficient(preview.NetSettlementValue), NetSettlementScale: preview.NetSettlementScale,
			SettlementDate: preview.SettlementDate, Charges: toInvestmentTradeChargeRequests(preview.Charges),
		})
	}
}

// investmentTradeReconciliationImpact previews which checkpoints a trade would
// invalidate. It is a read-only preview like sell/preview, so it takes no CSRF
// token and persists nothing; the UI calls it to name the checkpoints in its
// confirmation before retrying with reconciliation_override (T-53).
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
		writeReconciliationImpact(w, impact)
	}
}

func writeOffInvestment(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request investmentWriteOffRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.WriteOff(r.Context(), toInvestmentWriteOffInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "investment write-off", err)
			return
		}
		writeJSON(w, http.StatusCreated, toInvestmentTradeResponse(result))
	}))
}

func writeOffPreviewInvestment(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request investmentWriteOffRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewWriteOff(r.Context(), toInvestmentWriteOffInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "preview write-off", err)
			return
		}
		writeJSON(w, http.StatusOK, sellPreviewResponse{
			CostBasisMethod:   preview.CostBasisMethod,
			DisposalDecision:  toDisposalDecisionResponse(preview.DisposalDecision),
			Allocations:       toInvestmentLotDisposalResponses(preview.Allocations),
			RealizedGain:      moneyCoefficient(preview.RealizedGain),
			RealizedGainScale: preview.RealizedGainScale,
			CashAmountValue:   moneyCoefficient(preview.CashAmountValue),
			CashAmountScale:   preview.CashAmountScale,
		})
	}
}

// writeOffReconciliationImpact is investmentTradeReconciliationImpact for a
// write-off: it takes investmentWriteOffRequest (no cash fields) rather than
// the shared trade request, since a write-off is its own contract shape (T-38,
// T-53).
func writeOffReconciliationImpact(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request investmentWriteOffRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		impact, err := investmentService.WriteOffReconciliationImpact(r.Context(), toInvestmentWriteOffInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "write-off reconciliation impact", err)
			return
		}
		writeReconciliationImpact(w, impact)
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
		writeReconciliationImpact(w, impact)
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
		writeReconciliationImpact(w, impact)
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
	case errors.Is(err, db.ErrAverageCostTransferRequiresPoolAllocation):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_TRANSFER_POOL_REQUIRED", err.Error())
	case errors.Is(err, db.ErrPooledTransferRequiresAverageCost):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_TRANSFER_POOL_UNAVAILABLE", err.Error())
	case errors.Is(err, app.ErrInvestmentSaleNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment sale operation not found")
	case errors.Is(err, app.ErrInvestmentOperationNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment operation not found")
	case errors.Is(err, app.ErrInvestmentSaleAlreadyCorrected):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_SALE_ALREADY_CORRECTED", err.Error())
	case errors.Is(err, app.ErrInvestmentImportedSale):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_IMPORTED_SALE", err.Error())
	case errors.Is(err, app.ErrInvestmentSaleChanged):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_SALE_CHANGED", err.Error())
	case errors.Is(err, app.ErrInvestmentSaleDependency):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_SALE_DEPENDENCY", err.Error())
	case errors.Is(err, app.ErrInvestmentBuyNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "investment buy operation not found")
	case errors.Is(err, app.ErrInvestmentBuyAlreadyCorrected):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_BUY_ALREADY_CORRECTED", err.Error())
	case errors.Is(err, app.ErrInvestmentImportedBuy):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_IMPORTED_BUY", err.Error())
	case errors.Is(err, app.ErrInvestmentBuyChanged):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_BUY_CHANGED", err.Error())
	case errors.Is(err, app.ErrInvestmentBuyDependency):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_BUY_DEPENDENCY", err.Error())
	case errors.Is(err, app.ErrGainImpactAcknowledgementRequired):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_REQUIRED", err.Error())
	case errors.Is(err, app.ErrGainImpactAcknowledgementStale):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE", err.Error())
	case errors.Is(err, app.ErrInvestmentSplitNoHoldings):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_SPLIT_NO_HOLDINGS", err.Error())
	case errors.Is(err, app.ErrInvestmentSplitFraction):
		writeAPIError(w, http.StatusUnprocessableEntity, "INVESTMENT_SPLIT_FRACTION_UNREPRESENTABLE", err.Error())
	case errors.Is(err, app.ErrInvestmentSplitChanged):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_SPLIT_CHANGED", err.Error())
	case errors.Is(err, app.ErrInvestmentSplitDependency):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_SPLIT_DEPENDENCY", err.Error())
	case errors.Is(err, app.ErrInvestmentEventOutOfOrder):
		writeAPIError(w, http.StatusConflict, "INVESTMENT_EVENT_OUT_OF_ORDER", err.Error())
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
	charges := make([]app.InvestmentTradeChargeInput, 0, len(request.Charges))
	for _, charge := range request.Charges {
		charges = append(charges, app.InvestmentTradeChargeInput{Kind: charge.Kind, AmountValue: int64(charge.AmountValue),
			AmountScale: charge.AmountScale, CommodityID: charge.CommodityID, Treatment: charge.Treatment,
			ChargeAccountID: charge.ChargeAccountID, CashAccountID: charge.CashAccountID, PaidOn: charge.PaidOn,
			SourceEvidenceJSON: rawJSONText(charge.SourceEvidence)})
	}
	return app.InvestmentTradeInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionDate: request.TransactionDate, CommodityID: request.CommodityID, HoldingAccountID: request.HoldingAccountID,
		CashAccountID: request.CashAccountID, QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale,
		CashAmountValue: int64(request.CashAmountValue), CashAmountScale: request.CashAmountScale, CashCommodityID: request.CashCommodityID,
		GrossAmountValue: moneyInt64Pointer(request.GrossAmountValue), GrossAmountScale: request.GrossAmountScale,
		NetSettlementValue: moneyInt64Pointer(request.NetSettlementValue), NetSettlementScale: request.NetSettlementScale,
		SettlementDate: request.SettlementDate, Charges: charges,
		Memo: request.Memo, PayeeID: request.PayeeID, Status: request.Status, LotAllocations: allocations,
		ChangeReason: request.ChangeReason, CostBasisMethod: request.CostBasisMethod,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toInvestmentTradeChargeRequests(charges []app.InvestmentTradeChargeInput) []investmentTradeChargeRequest {
	result := make([]investmentTradeChargeRequest, 0, len(charges))
	for _, charge := range charges {
		var evidence json.RawMessage
		if charge.SourceEvidenceJSON != "" {
			evidence = json.RawMessage(charge.SourceEvidenceJSON)
		}
		result = append(result, investmentTradeChargeRequest{Kind: charge.Kind, AmountValue: moneyCoefficient(charge.AmountValue),
			AmountScale: charge.AmountScale, CommodityID: charge.CommodityID, Treatment: charge.Treatment,
			ChargeAccountID: charge.ChargeAccountID, CashAccountID: charge.CashAccountID, PaidOn: charge.PaidOn,
			SourceEvidence: evidence})
	}
	return result
}

func toDividendInput(owner app.Owner, r *http.Request, request dividendRequest) app.DividendInput {
	return app.DividendInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionDate: request.TransactionDate, CommodityID: request.CommodityID, CashAccountID: request.CashAccountID,
		CashCommodityID: request.CashCommodityID, IncomeAccountID: request.IncomeAccountID, AmountValue: int64(request.AmountValue),
		AmountScale: request.AmountScale, WithholdingValue: moneyInt64Pointer(request.WithholdingValue), WithholdingScale: request.WithholdingScale,
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
		AmountValue: int64(request.AmountValue), AmountScale: request.AmountScale, CashCommodityID: request.CashCommodityID,
		Memo: request.Memo, PayeeID: request.PayeeID, Status: request.Status, ChangeReason: request.ChangeReason,
		ReconciliationOverride: request.ReconciliationOverride, GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toInvestmentWriteOffInput(owner app.Owner, r *http.Request, request investmentWriteOffRequest) app.InvestmentWriteOffInput {
	allocations := make([]app.InvestmentLotAllocationInput, 0, len(request.LotAllocations))
	for _, allocation := range request.LotAllocations {
		allocations = append(allocations, app.InvestmentLotAllocationInput{LotID: allocation.LotID, QuantityValue: allocation.QuantityValue, QuantityScale: allocation.QuantityScale})
	}
	return app.InvestmentWriteOffInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r), RequestID: RequestIDFromContext(r.Context()),
		TransactionDate: request.TransactionDate, CommodityID: request.CommodityID, HoldingAccountID: request.HoldingAccountID,
		QuantityValue: request.QuantityValue, QuantityScale: request.QuantityScale, Reason: request.Reason,
		Memo: request.Memo, PayeeID: request.PayeeID, Status: request.Status, LotAllocations: allocations,
		ChangeReason: request.ChangeReason, CostBasisMethod: request.CostBasisMethod,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
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
	return dividendDefaultResponse{ID: dividendDefault.ID, BookID: dividendDefault.BookID, CommodityID: dividendDefault.CommodityID, IncomeAccountID: dividendDefault.IncomeAccountID, WithholdingAccountID: dividendDefault.WithholdingAccountID, DefaultWithholdingValue: moneyCoefficientPointer(dividendDefault.DefaultWithholdingValue), DefaultWithholdingScale: dividendDefault.DefaultWithholdingScale, WithholdingRateBPS: dividendDefault.WithholdingRateBPS, TaxCountryCode: dividendDefault.TaxCountryCode, TaxTreatment: dividendDefault.TaxTreatment, Status: dividendDefault.Status, EffectiveFrom: dividendDefault.EffectiveFrom, EffectiveTo: dividendDefault.EffectiveTo, Metadata: json.RawMessage(dividendDefault.MetadataJSON), CreatedAt: dividendDefault.CreatedAt, UpdatedAt: dividendDefault.UpdatedAt}
}

func toDividendDefaultResponses(defaults []app.DividendDefault) []dividendDefaultResponse {
	responses := make([]dividendDefaultResponse, 0, len(defaults))
	for _, dividendDefault := range defaults {
		responses = append(responses, toDividendDefaultResponse(dividendDefault))
	}
	return responses
}

func toInvestmentTradeResponse(result app.InvestmentTradeResult) investmentTradeResponse {
	response := investmentTradeResponse{Transaction: toTransactionResponse(result.Transaction), LotID: result.LotID, Allocations: toInvestmentLotDisposalResponses(result.Allocations)}
	if result.DisposalDecision != nil {
		decision := toDisposalDecisionResponse(*result.DisposalDecision)
		response.DisposalDecision = &decision
	}
	return response
}

func toDisposalDecisionResponse(decision app.DisposalDecision) disposalDecisionResponse {
	return disposalDecisionResponse{
		ID: decision.ID, TransactionID: decision.TransactionID, TransactionVersionID: decision.TransactionVersionID,
		CostBasisMethod: decision.CostBasisMethod, ResolutionTier: decision.ResolutionTier,
		AccountVersionID: decision.AccountVersionID, ProfileID: decision.ProfileID, ProfileVersionID: decision.ProfileVersionID,
		SourceEffectiveFrom: decision.SourceEffectiveFrom, SourceRecordedAt: decision.SourceRecordedAt,
		QuantityValue: decision.QuantityValue, QuantityScale: decision.QuantityScale,
		DisposedBasisValue: decision.DisposedBasisValue, DisposedBasisScale: decision.DisposedBasisScale,
		ProceedsValue: moneyCoefficient(decision.ProceedsValue), ProceedsScale: decision.ProceedsScale,
		CostCommodityID: decision.CostCommodityID, AuditEventID: decision.AuditEventID,
		Allocations: toInvestmentLotDisposalResponses(decision.Allocations),
	}
}

func toInvestmentLotDisposalResponses(disposals []app.InvestmentLotDisposal) []investmentLotDisposalResponse {
	responses := make([]investmentLotDisposalResponse, 0, len(disposals))
	for _, disposal := range disposals {
		responses = append(responses, investmentLotDisposalResponse{LotID: disposal.LotID, QuantityValue: disposal.QuantityValue, QuantityScale: disposal.QuantityScale, CostBasisValue: moneyCoefficient(disposal.CostBasisValue), CostBasisScale: disposal.CostBasisScale, ProceedsValue: moneyCoefficient(disposal.ProceedsValue), ProceedsScale: disposal.ProceedsScale})
	}
	return responses
}

func toInvestmentLotResponse(lot app.InvestmentLot) investmentLotResponse {
	return investmentLotResponse{ID: lot.ID, BookID: lot.BookID, AccountID: lot.AccountID, CommodityID: lot.CommodityID, OpenedOn: lot.OpenedOn, SourceTransactionID: lot.SourceTransactionID, Status: lot.Status, QuantityValue: lot.QuantityValue, QuantityScale: lot.QuantityScale, RemainingQuantityValue: lot.RemainingQuantityValue, RemainingQuantityScale: lot.RemainingQuantityScale, CostBasisValue: moneyCoefficient(lot.CostBasisValue), CostBasisScale: lot.CostBasisScale, RemainingCostBasisValue: projectedBasisValue(lot.RemainingCostBasisValue, lot.BasisKnowledge), RemainingCostBasisScale: projectedBasisScale(lot.RemainingCostBasisScale, lot.BasisKnowledge), BasisKnowledge: lot.BasisKnowledge, CostCommodityID: lot.CostCommodityID, Metadata: json.RawMessage(lot.MetadataJSON), CreatedAt: lot.CreatedAt, UpdatedAt: lot.UpdatedAt}
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
		responses = append(responses, investmentPositionResponse{AccountID: position.AccountID, CommodityID: position.CommodityID, QuantityValue: position.QuantityValue, QuantityScale: position.QuantityScale, RemainingCostBasisValue: projectedBasisValue(position.RemainingCostBasisValue, position.BasisKnowledge), RemainingCostBasisScale: projectedBasisScale(position.RemainingCostBasisScale, position.BasisKnowledge), BasisKnowledge: position.BasisKnowledge, CostCommodityID: position.CostCommodityID, LatestPriceValue: moneyCoefficientPointer(position.LatestPriceValue), LatestPriceScale: position.LatestPriceScale, LatestPriceDate: position.LatestPriceDate, LatestPriceApproximate: position.LatestPriceApproximate, TransferBasisAllocation: position.TransferBasisAllocation})
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
	DisposedBasisValue moneyCoefficient  `json:"disposed_basis_value"`
	DisposedBasisScale int               `json:"disposed_basis_scale"`
	ProceedsValue      moneyCoefficient  `json:"proceeds_value"`
	ProceedsScale      int               `json:"proceeds_scale"`
	RealizedGainValue  moneyCoefficient  `json:"realized_gain_value"`
	RealizedGainScale  int               `json:"realized_gain_scale"`
}

type unrealizedGainResponse struct {
	AccountID               int64             `json:"account_id"`
	CommodityID             int64             `json:"commodity_id"`
	CostCommodityID         int64             `json:"cost_commodity_id"`
	QuantityValue           exact.Coefficient `json:"quantity_value"`
	QuantityScale           int               `json:"quantity_scale"`
	RemainingCostBasisValue *moneyCoefficient `json:"remaining_cost_basis_value"`
	RemainingCostBasisScale *int              `json:"remaining_cost_basis_scale"`
	BasisKnowledge          string            `json:"basis_knowledge"`
	LatestPriceValue        *moneyCoefficient `json:"latest_price_value,omitempty"`
	LatestPriceScale        *int              `json:"latest_price_scale,omitempty"`
	LatestPriceDate         string            `json:"latest_price_date,omitempty"`
	LatestPriceApproximate  bool              `json:"latest_price_approximate"`
	MarketValueValue        *moneyCoefficient `json:"market_value_value,omitempty"`
	MarketValueScale        *int              `json:"market_value_scale,omitempty"`
	UnrealizedGainValue     *moneyCoefficient `json:"unrealized_gain_value,omitempty"`
	UnrealizedGainScale     *int              `json:"unrealized_gain_scale,omitempty"`
	ValuationUnavailable    string            `json:"valuation_unavailable,omitempty"`
	GainUnavailable         string            `json:"gain_unavailable,omitempty"`
}

type realizedGainTotalResponse struct {
	CostCommodityID int64            `json:"cost_commodity_id"`
	TotalGainValue  moneyCoefficient `json:"total_gain_value"`
	TotalGainScale  int              `json:"total_gain_scale"`
}

type investmentGainsResponse struct {
	Currencies     []currencyResponse          `json:"currencies"`
	Realized       []realizedGainResponse      `json:"realized"`
	Unrealized     []unrealizedGainResponse    `json:"unrealized"`
	RealizedTotals []realizedGainTotalResponse `json:"realized_totals"`
}

func listInvestmentGains(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, currencyService *app.CurrencyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}
		currencies, err := currencyService.ListCurrencies(r.Context())
		if err != nil {
			writeCurrencyServiceError(w, r, logger, "list gains currencies", err)
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
				DisposedBasisValue: moneyCoefficient(e.DisposedBasisValue),
				DisposedBasisScale: e.DisposedBasisScale,
				ProceedsValue:      moneyCoefficient(e.ProceedsValue),
				ProceedsScale:      e.ProceedsScale,
				RealizedGainValue:  moneyCoefficient(e.RealizedGainValue),
				RealizedGainScale:  e.RealizedGainScale,
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
				RemainingCostBasisValue: projectedBasisValue(e.RemainingCostBasisValue, e.BasisKnowledge),
				RemainingCostBasisScale: projectedBasisScale(e.RemainingCostBasisScale, e.BasisKnowledge), BasisKnowledge: e.BasisKnowledge,
				LatestPriceValue:       moneyCoefficientPointer(e.LatestPriceValue),
				LatestPriceScale:       e.LatestPriceScale,
				LatestPriceDate:        e.LatestPriceDate,
				LatestPriceApproximate: e.LatestPriceApproximate,
				MarketValueValue:       moneyCoefficientPointer(e.MarketValueValue),
				MarketValueScale:       e.MarketValueScale,
				UnrealizedGainValue:    moneyCoefficientPointer(e.UnrealizedGainValue),
				UnrealizedGainScale:    e.UnrealizedGainScale,
				ValuationUnavailable:   e.ValuationUnavailable,
				GainUnavailable:        e.GainUnavailable,
			})
		}

		// Realized totals are grouped by cost commodity alone and summed
		// exactly. Grouping by scale as well used to stand in for "can be added
		// without loss", but a gain now carries whatever scale its own
		// subtraction needed (T-101), so two gains in the same currency can
		// legitimately differ in scale — splitting them into separate totals
		// would report one currency twice and make neither row the total.
		// exact.ScaledInt deepens as it adds, so one row per currency holds the
		// sum without rounding.
		totalsMap := map[int64]*exact.ScaledInt{}
		var totalsOrder []int64
		for _, e := range realized {
			if totalsMap[e.CostCommodityID] == nil {
				totalsMap[e.CostCommodityID] = exact.NewScaledInt()
				totalsOrder = append(totalsOrder, e.CostCommodityID)
			}
			totalsMap[e.CostCommodityID].AddInt64(e.RealizedGainValue, e.RealizedGainScale)
		}
		realizedTotals := make([]realizedGainTotalResponse, 0, len(totalsOrder))
		for _, commodityID := range totalsOrder {
			total := totalsMap[commodityID]
			totalValue, err := total.Int64()
			if err != nil {
				writeServiceInternalError(w, r, logger, "sum realized gains", err)
				return
			}
			realizedTotals = append(realizedTotals, realizedGainTotalResponse{
				CostCommodityID: commodityID,
				TotalGainValue:  moneyCoefficient(totalValue),
				TotalGainScale:  total.Scale(),
			})
		}
		writeJSON(w, http.StatusOK, investmentGainsResponse{
			Currencies:     toCurrencyResponses(currencies),
			Realized:       realizedResponses,
			Unrealized:     unrealizedResponses,
			RealizedTotals: realizedTotals,
		})
	}
}

func projectedBasisValue(value int64, knowledge string) *moneyCoefficient {
	if knowledge == "unknown" {
		return nil
	}
	result := moneyCoefficient(value)
	return &result
}
func projectedBasisScale(scale int, knowledge string) *int {
	if knowledge == "unknown" {
		return nil
	}
	return &scale
}

type investmentSplitRequest struct {
	EffectiveOn               string          `json:"effective_on"`
	HoldingAccountID          int64           `json:"holding_account_id"`
	CommodityID               int64           `json:"commodity_id"`
	RatioNumerator            int64           `json:"ratio_numerator"`
	RatioDenominator          int64           `json:"ratio_denominator"`
	SourceEvidence            json.RawMessage `json:"source_evidence,omitempty"`
	Memo                      string          `json:"memo"`
	ChangeReason              string          `json:"change_reason"`
	ReconciliationOverride    bool            `json:"reconciliation_override"`
	GainImpactAcknowledgement string          `json:"gain_impact_acknowledgement,omitempty"`
}

type investmentSplitLotEffectResponse struct {
	LotID               int64             `json:"lot_id"`
	CostCommodityID     int64             `json:"cost_commodity_id"`
	QuantityBeforeValue exact.Coefficient `json:"quantity_before_value"`
	QuantityBeforeScale int               `json:"quantity_before_scale"`
	QuantityAfterValue  exact.Coefficient `json:"quantity_after_value"`
	QuantityAfterScale  int               `json:"quantity_after_scale"`
}

type investmentSplitPlanResponse struct {
	RatioNumerator     int64                              `json:"ratio_numerator"`
	RatioDenominator   int64                              `json:"ratio_denominator"`
	Effects            []investmentSplitLotEffectResponse `json:"effects"`
	QuantityDeltaValue exact.Coefficient                  `json:"quantity_delta_value"`
	QuantityDeltaScale int                                `json:"quantity_delta_scale"`
	Replayed           bool                               `json:"replayed"`
}

type investmentSplitPreviewResponse struct {
	Plan   investmentSplitPlanResponse  `json:"plan"`
	Impact reconciliationImpactResponse `json:"impact"`
}

type investmentSplitResponse struct {
	Transaction transactionResponse         `json:"transaction"`
	Plan        investmentSplitPlanResponse `json:"plan"`
}

func investmentSplitInput(owner app.Owner, r *http.Request, request investmentSplitRequest) app.InvestmentSplitInput {
	evidence := rawJSONText(request.SourceEvidence)
	if evidence == "null" {
		evidence = ""
	}
	return app.InvestmentSplitInput{
		OwnerUserID: owner.ID, AuthSessionID: authenticatedSessionID(r),
		RequestID: RequestIDFromContext(r.Context()), EffectiveOn: request.EffectiveOn,
		HoldingAccountID: request.HoldingAccountID, CommodityID: request.CommodityID,
		RatioNumerator: request.RatioNumerator, RatioDenominator: request.RatioDenominator,
		SourceEvidenceJSON: evidence, Memo: request.Memo, ChangeReason: request.ChangeReason,
		ReconciliationOverride:    request.ReconciliationOverride,
		GainImpactAcknowledgement: request.GainImpactAcknowledgement,
	}
}

func toInvestmentSplitPlanResponse(plan app.InvestmentSplitPlan) investmentSplitPlanResponse {
	out := investmentSplitPlanResponse{RatioNumerator: plan.RatioNumerator, RatioDenominator: plan.RatioDenominator,
		QuantityDeltaValue: plan.DeltaValue, QuantityDeltaScale: plan.DeltaScale, Replayed: plan.Replayed,
		Effects: make([]investmentSplitLotEffectResponse, 0, len(plan.Effects))}
	for _, effect := range plan.Effects {
		out.Effects = append(out.Effects, investmentSplitLotEffectResponse{
			LotID: effect.LotID, CostCommodityID: effect.CostCommodityID,
			QuantityBeforeValue: effect.BeforeValue, QuantityBeforeScale: effect.BeforeScale,
			QuantityAfterValue: effect.AfterValue, QuantityAfterScale: effect.AfterScale,
		})
	}
	return out
}

func investmentSplit(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService, options HandlerOptions) http.HandlerFunc {
	return requireAuthenticatedMutation(logger, authService, options, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedMutationOwner(w, r)
		if !ok {
			return
		}
		var request investmentSplitRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		result, err := investmentService.Split(r.Context(), investmentSplitInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "investment split", err)
			return
		}
		writeJSON(w, http.StatusCreated, investmentSplitResponse{
			Transaction: toTransactionResponse(result.Transaction), Plan: toInvestmentSplitPlanResponse(result.Plan),
		})
	}))
}

func investmentSplitPreview(logger *slog.Logger, authService *app.AuthService, investmentService *app.InvestmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, ok := authenticatedOwner(w, r, logger, authService)
		if !ok {
			return
		}
		var request investmentSplitRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		preview, err := investmentService.PreviewSplit(r.Context(), investmentSplitInput(owner, r, request))
		if err != nil {
			writeInvestmentServiceError(w, r, logger, "investment split preview", err)
			return
		}
		impact := toReconciliationImpactResponse(preview.Impact)
		gainImpact, err := toGainImpactResponse(preview.Impact.GainImpact)
		if err != nil {
			writeAPIError(w, http.StatusUnprocessableEntity, "LEDGER_OVERFLOW", "gain impact value exceeds the coefficient range")
			return
		}
		impact.GainImpact = gainImpact
		writeJSON(w, http.StatusOK, investmentSplitPreviewResponse{
			Plan: toInvestmentSplitPlanResponse(preview.Plan), Impact: impact,
		})
	}
}
