package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

type resolvedTradeCharge struct {
	Input           InvestmentTradeChargeInput
	Date            string
	Treatment       string
	AccountID       int64
	ResolutionTier  string
	PolicyVersionID int64
	EvidenceJSON    string
	SeparatelyPaid  bool
}

type tradeEconomics struct {
	TradeDate            string
	PrimaryCashAccountID int64
	GrossKnown           bool
	GrossValue           int64
	GrossScale           int
	NetValue             int64
	NetScale             int
	ClearingValue        int64
	ClearingScale        int
	MainClearingValue    int64
	MainClearingScale    int
	SettlementDate       string
	Charges              []resolvedTradeCharge
	PriceValue           int64
	PriceScale           int
	PriceApproximate     bool
	PriceUnavailable     bool
}

func (s *InvestmentService) calculateTradeEconomics(ctx context.Context, input InvestmentTradeInput, isBuy bool, dependencies *accountRuleDependencies) (tradeEconomics, error) {
	if input.WriteOff {
		return tradeEconomics{}, nil
	}
	settlementDate := input.TransactionDate
	if strings.TrimSpace(input.SettlementDate) != "" {
		var err error
		settlementDate, err = cleanRequiredDate(input.SettlementDate, "settlement date")
		if err != nil {
			return tradeEconomics{}, err
		}
	}
	if _, err := s.accountInRole(ctx, input.CashAccountID, settlementDate, settlementRole, dependencies); err != nil {
		return tradeEconomics{}, err
	}
	economics := tradeEconomics{TradeDate: input.TransactionDate, SettlementDate: settlementDate, PrimaryCashAccountID: input.CashAccountID}
	if input.GrossAmountValue == nil {
		if input.NetSettlementValue != nil || len(input.Charges) > 0 {
			return tradeEconomics{}, ValidationError{Message: "gross amount is required when net settlement or charges are specified"}
		}
		if input.CashAmountValue <= 0 {
			return tradeEconomics{}, ValidationError{Message: "cash amount is required"}
		}
		economics.NetValue = input.CashAmountValue
		if isBuy {
			economics.NetValue = -economics.NetValue
		}
		economics.NetScale = input.CashAmountScale
		economics.ClearingValue = economics.NetValue
		economics.ClearingScale = economics.NetScale
		economics.MainClearingValue = economics.ClearingValue
		economics.MainClearingScale = economics.ClearingScale
		economics.PriceValue = input.CashAmountValue
		economics.PriceScale = input.CashAmountScale
		economics.PriceApproximate = true
		return economics, nil
	}
	if input.NetSettlementValue == nil {
		return tradeEconomics{}, ValidationError{Message: "net settlement is required with gross amount"}
	}
	if input.GrossAmountScale < 0 || input.GrossAmountScale > exact.MaxStandardScale ||
		input.NetSettlementScale < 0 || input.NetSettlementScale > exact.MaxStandardScale {
		return tradeEconomics{}, ValidationError{Message: "trade money scale is invalid"}
	}
	gross := *input.GrossAmountValue
	if isBuy && gross >= 0 || !isBuy && gross <= 0 {
		return tradeEconomics{}, ValidationError{Message: "gross amount has the wrong sign for this trade"}
	}
	net := exact.ScaledIntFromInt64(gross, input.GrossAmountScale)
	clearing := exact.ScaledIntFromInt64(gross, input.GrossAmountScale)
	for _, chargeInput := range input.Charges {
		charge, err := s.resolveTradeCharge(ctx, input, chargeInput, dependencies)
		if err != nil {
			return tradeEconomics{}, err
		}
		economics.Charges = append(economics.Charges, charge)
		if chargeInput.CommodityID == input.CashCommodityID {
			if !charge.SeparatelyPaid {
				net.AddInt64(chargeInput.AmountValue, chargeInput.AmountScale)
			}
			if charge.Treatment == "clearing_included" {
				clearing.AddInt64(chargeInput.AmountValue, chargeInput.AmountScale)
			}
		}
	}
	if net.Cmp(exact.ScaledIntFromInt64(*input.NetSettlementValue, input.NetSettlementScale)) != 0 {
		return tradeEconomics{}, ValidationError{Message: "gross plus charges does not equal net settlement"}
	}
	if isBuy && clearing.Sign() >= 0 {
		return tradeEconomics{}, ValidationError{Message: "buy charges and rebates leave no positive cost basis"}
	}
	clearingValue, err := clearing.Int64()
	if err != nil {
		return tradeEconomics{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
	}
	if input.CashAmountValue > 0 {
		legacy := exact.ScaledIntFromInt64(input.CashAmountValue, input.CashAmountScale)
		legacyNet := exact.ScaledIntFromInt64(*input.NetSettlementValue, input.NetSettlementScale)
		if isBuy {
			legacyNet = legacyNet.Negated()
		}
		if legacy.Cmp(legacyNet) != 0 {
			return tradeEconomics{}, ValidationError{Message: "cash amount conflicts with net settlement"}
		}
	}
	economics.GrossKnown = true
	economics.GrossValue = gross
	economics.GrossScale = input.GrossAmountScale
	economics.NetValue = *input.NetSettlementValue
	economics.NetScale = input.NetSettlementScale
	economics.ClearingValue = clearingValue
	economics.ClearingScale = clearing.Scale()
	mainClearing := exact.ScaledIntFromInt64(gross, input.GrossAmountScale)
	for _, charge := range economics.Charges {
		if charge.Input.CommodityID == input.CashCommodityID && !charge.SeparatelyPaid && charge.Treatment == "clearing_included" {
			mainClearing.AddInt64(charge.Input.AmountValue, charge.Input.AmountScale)
		}
	}
	economics.MainClearingValue, err = mainClearing.Int64()
	if err != nil {
		return tradeEconomics{}, LedgerOverflowError{CommodityID: input.CashCommodityID}
	}
	economics.MainClearingScale = mainClearing.Scale()
	economics.PriceValue = gross
	if isBuy && gross == math.MinInt64 {
		economics.PriceUnavailable = true
	} else if isBuy {
		economics.PriceValue = -gross
	}
	economics.PriceScale = input.GrossAmountScale
	return economics, nil
}

func (s *InvestmentService) resolveTradeCharge(ctx context.Context, trade InvestmentTradeInput, charge InvestmentTradeChargeInput, dependencies *accountRuleDependencies) (resolvedTradeCharge, error) {
	if charge.AmountValue == 0 || charge.AmountScale < 0 || charge.AmountScale > exact.MaxStandardScale {
		return resolvedTradeCharge{}, ValidationError{Message: "trade charge amount is invalid"}
	}
	kind := strings.TrimSpace(charge.Kind)
	if kind != charge.Kind {
		return resolvedTradeCharge{}, ValidationError{Message: "trade charge kind has surrounding whitespace"}
	}
	switch kind {
	case "commission", "transaction_tax", "other_fee":
		if charge.AmountValue >= 0 {
			return resolvedTradeCharge{}, ValidationError{Message: "trade fee or tax must be negative"}
		}
	case "rebate":
		if charge.AmountValue <= 0 {
			return resolvedTradeCharge{}, ValidationError{Message: "trade rebate must be positive"}
		}
	default:
		return resolvedTradeCharge{}, ValidationError{Message: "trade charge kind is invalid"}
	}
	if charge.CommodityID <= 0 {
		return resolvedTradeCharge{}, ValidationError{Message: "trade charge currency is required"}
	}
	date := trade.SettlementDate
	if date == "" {
		date = trade.TransactionDate
	}
	if charge.PaidOn != "" {
		date = charge.PaidOn
	}
	var err error
	date, err = cleanRequiredDate(date, "charge payment date")
	if err != nil {
		return resolvedTradeCharge{}, err
	}
	evidence, err := cleanSizedJSONObject(charge.SourceEvidenceJSON, "charge evidence", investmentJSONMaxBytes)
	if err != nil {
		return resolvedTradeCharge{}, err
	}
	resolved := resolvedTradeCharge{Input: charge, Date: date, EvidenceJSON: evidence}
	resolved.SeparatelyPaid = charge.CommodityID != trade.CashCommodityID || date != defaultString(trade.SettlementDate, trade.TransactionDate) ||
		(charge.CashAccountID != nil && *charge.CashAccountID != trade.CashAccountID)
	if charge.Treatment != "" {
		resolved.Treatment = charge.Treatment
		resolved.ResolutionTier = "transaction"
		if charge.ChargeAccountID != nil {
			resolved.AccountID = *charge.ChargeAccountID
		}
	} else {
		policy, found, err := s.repository.ResolveInvestmentFeePolicy(ctx, BookID, trade.HoldingAccountID, kind, trade.TransactionDate)
		if err != nil {
			return resolvedTradeCharge{}, err
		}
		if found && (charge.CommodityID == trade.CashCommodityID || policy.Treatment == "separately_expensed") {
			resolved.Treatment = policy.Treatment
			resolved.ResolutionTier = policy.ResolutionTier
			resolved.PolicyVersionID = policy.VersionID
			if policy.ChargeAccountID.Valid {
				resolved.AccountID = policy.ChargeAccountID.Int64
			}
		} else if kind == "commission" && charge.CommodityID == trade.CashCommodityID {
			resolved.Treatment = "clearing_included"
			resolved.ResolutionTier = "fallback"
		} else if charge.CommodityID != trade.CashCommodityID && charge.ChargeAccountID != nil {
			resolved.Treatment = "separately_expensed"
			resolved.ResolutionTier = "fallback"
			resolved.AccountID = *charge.ChargeAccountID
		} else {
			return resolvedTradeCharge{}, ValidationError{Message: "trade charge treatment is required"}
		}
	}
	if resolved.Treatment != "clearing_included" && resolved.Treatment != "separately_expensed" {
		return resolvedTradeCharge{}, ValidationError{Message: "trade charge treatment is invalid"}
	}
	if charge.Treatment == "" && charge.ChargeAccountID != nil && resolved.AccountID != *charge.ChargeAccountID {
		return resolvedTradeCharge{}, ValidationError{Message: "charge account conflicts with resolved fee policy; select a transaction treatment to override it"}
	}
	if resolved.SeparatelyPaid && (charge.CashAccountID == nil || *charge.CashAccountID <= 0) {
		return resolvedTradeCharge{}, ValidationError{Message: "separately paid charge requires its cash account"}
	}
	if charge.CommodityID != trade.CashCommodityID {
		if resolved.Treatment != "separately_expensed" || charge.CashAccountID == nil || *charge.CashAccountID <= 0 {
			return resolvedTradeCharge{}, ValidationError{Message: "foreign-currency charge requires a cash account and separate expense treatment"}
		}
		if _, err := s.accountInRole(ctx, *charge.CashAccountID, date, settlementRole, dependencies); err != nil {
			return resolvedTradeCharge{}, err
		}
		if err := s.requireCommodityKind(ctx, charge.CommodityID, date, "charge currency", true); err != nil {
			return resolvedTradeCharge{}, err
		}
	} else if resolved.SeparatelyPaid {
		if _, err := s.accountInRole(ctx, *charge.CashAccountID, date, settlementRole, dependencies); err != nil {
			return resolvedTradeCharge{}, err
		}
	}
	if resolved.Treatment == "separately_expensed" {
		if resolved.AccountID <= 0 {
			return resolvedTradeCharge{}, ValidationError{Message: "separate trade charge account is required"}
		}
		role := accountRoleForInvestment{label: "trade fee account", allowedClasses: []string{"expense"}}
		if charge.AmountValue > 0 {
			role = accountRoleForInvestment{label: "trade rebate account", allowedClasses: []string{"income"}}
		}
		if _, err := s.accountInRole(ctx, resolved.AccountID, date, role, dependencies); err != nil {
			return resolvedTradeCharge{}, err
		}
	} else if resolved.AccountID != 0 {
		return resolvedTradeCharge{}, ValidationError{Message: "clearing-included charge cannot name an expense account"}
	}
	if resolved.Input.CashAccountID == nil {
		accountID := trade.CashAccountID
		resolved.Input.CashAccountID = &accountID
	}
	return resolved, nil
}

// tradeJournalEntries posts the security on trade date, settlement in the
// quote currency on settlement date, and independently paid foreign charges
// on their payment dates. Every dated entry balances by commodity.
func tradeJournalEntries(input InvestmentTradeInput, tradingAccountID int64, memo string, economics tradeEconomics, isBuy bool) []JournalEntryInput {
	postingsByDate := map[string][]PostingInput{}
	add := func(date string, accountID, commodityID int64, value int64, scale int) {
		if value == 0 {
			return
		}
		postingsByDate[date] = append(postingsByDate[date], PostingInput{
			AccountID: accountID, CommodityID: commodityID, QuantityValue: exact.New(value), QuantityScale: scale, Memo: memo,
		})
	}
	security := input.QuantityValue
	if !isBuy {
		security = security.Negated()
	}
	postingsByDate[input.TransactionDate] = append(postingsByDate[input.TransactionDate],
		PostingInput{AccountID: input.HoldingAccountID, CommodityID: input.CommodityID, QuantityValue: security, QuantityScale: input.QuantityScale, Memo: memo},
		PostingInput{AccountID: tradingAccountID, CommodityID: input.CommodityID, QuantityValue: security.Negated(), QuantityScale: input.QuantityScale, Memo: memo},
	)
	add(economics.SettlementDate, input.CashAccountID, input.CashCommodityID, economics.NetValue, economics.NetScale)
	if economics.MainClearingValue != 0 {
		postingsByDate[economics.SettlementDate] = append(postingsByDate[economics.SettlementDate], PostingInput{
			AccountID: tradingAccountID, CommodityID: input.CashCommodityID,
			QuantityValue: exact.New(economics.MainClearingValue).Negated(), QuantityScale: economics.MainClearingScale, Memo: memo,
		})
	}
	for _, charge := range economics.Charges {
		if !charge.SeparatelyPaid {
			if charge.Treatment == "separately_expensed" {
				postingsByDate[economics.SettlementDate] = append(postingsByDate[economics.SettlementDate], PostingInput{
					AccountID: charge.AccountID, CommodityID: input.CashCommodityID,
					QuantityValue: exact.New(charge.Input.AmountValue).Negated(), QuantityScale: charge.Input.AmountScale, Memo: memo,
				})
			}
			continue
		}
		add(charge.Date, *charge.Input.CashAccountID, charge.Input.CommodityID, charge.Input.AmountValue, charge.Input.AmountScale)
		chargeAccountID := charge.AccountID
		if charge.Treatment == "clearing_included" {
			chargeAccountID = tradingAccountID
		}
		postingsByDate[charge.Date] = append(postingsByDate[charge.Date], PostingInput{
			AccountID: chargeAccountID, CommodityID: charge.Input.CommodityID,
			QuantityValue: exact.New(charge.Input.AmountValue).Negated(), QuantityScale: charge.Input.AmountScale, Memo: memo,
		})
	}
	dates := make([]string, 0, len(postingsByDate))
	for date := range postingsByDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	entries := make([]JournalEntryInput, 0, len(dates))
	for _, date := range dates {
		entries = append(entries, JournalEntryInput{EntryDate: date, EntryKind: "investment", Memo: memo, Postings: postingsByDate[date]})
	}
	return entries
}

func tradeJournalEntriesForDisposal(input InvestmentTradeInput, tradingAccountID int64, memo string, economics *tradeEconomics) []JournalEntryInput {
	if input.WriteOff {
		return []JournalEntryInput{{EntryDate: input.TransactionDate, EntryKind: "investment", Memo: memo,
			Postings: writeOffPostings(input, tradingAccountID, memo)}}
	}
	return tradeJournalEntries(input, tradingAccountID, memo, *economics, false)
}

func (e tradeEconomics) components(commodityID int64) []db.InvestmentComponentSpec {
	components := make([]db.InvestmentComponentSpec, 0, 2+len(e.Charges))
	if e.GrossKnown {
		components = append(components, db.InvestmentComponentSpec{
			Kind: "gross_consideration", CommodityID: commodityID,
			AmountValue: fmt.Sprint(e.GrossValue), AmountScale: e.GrossScale, AmountDate: e.TradeDate,
		})
	}
	components = append(components, db.InvestmentComponentSpec{
		Kind: "net_settlement", CommodityID: commodityID,
		AmountValue: fmt.Sprint(e.NetValue), AmountScale: e.NetScale, AmountDate: e.SettlementDate,
		GrossUnknown: !e.GrossKnown, CashAccountID: e.PrimaryCashAccountID,
	})
	for _, charge := range e.Charges {
		if charge.SeparatelyPaid {
			components = append(components, db.InvestmentComponentSpec{
				Kind: "net_settlement", CommodityID: charge.Input.CommodityID,
				AmountValue: fmt.Sprint(charge.Input.AmountValue), AmountScale: charge.Input.AmountScale,
				AmountDate: charge.Date, CashAccountID: *charge.Input.CashAccountID,
			})
		}
		components = append(components, db.InvestmentComponentSpec{
			Kind: "charge", ChargeKind: charge.Input.Kind, CommodityID: charge.Input.CommodityID,
			AmountValue: fmt.Sprint(charge.Input.AmountValue), AmountScale: charge.Input.AmountScale,
			AmountDate: charge.Date, ChargeTreatment: charge.Treatment,
			ChargeAccountID: charge.AccountID, ResolutionTier: charge.ResolutionTier,
			CashAccountID:      tradeChargeCashAccountID(charge.Input.CashAccountID),
			FeePolicyVersionID: charge.PolicyVersionID, SourceEvidenceJSON: charge.EvidenceJSON,
		})
	}
	return components
}

func tradeChargeCashAccountID(accountID *int64) int64 {
	if accountID == nil {
		return 0
	}
	return *accountID
}
