package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// exchangeEntryKind marks a journal entry that moves money between two of the
// owner's accounts in different currencies (#481).
const exchangeEntryKind = "exchange"

// Conversion sides of a currency exchange (#481).
const (
	conversionSending   = "sending"
	conversionReceiving = "receiving"
)

// buildExchangeLegs completes a currency exchange: a request whose one
// journal entry has entry_kind "exchange" names the two account postings —
// what left the sending account and what reached the receiving one, as their
// statements show them — and at most one expense posting for a fee charged
// on top, in the currency of the account that paid it, and the service adds
// one commodity_trading posting per currency so each balances.
//
// Both amounts are facts; no rate is stored or used. The difference against
// any market rate stays in commodity_trading, which reports exclude, and no
// FX gain or loss is posted. The client never names a system account here,
// as it never sends the #187 transfer_clearing bridge: the server owns both
// and rebuilds them on every save.
//
// Days compose through the posted-date rule: the fee posts on the day of the
// account that paid it, and the trading legs on the day of the side that
// converted — the sending account's by default, the receiving account's
// when the receiving bank converted — so the currency in transit, the
// cashflow of the fee and the market comparison all follow from the journal.
//
// A request without an exchange entry passes through untouched.
func (s *TransactionService) buildExchangeLegs(ctx context.Context, input TransactionInput, transactionDate string) (TransactionInput, error) {
	exchange := false
	for _, entry := range input.JournalEntries {
		if strings.TrimSpace(entry.EntryKind) == exchangeEntryKind {
			exchange = true
		} else if strings.TrimSpace(entry.ConversionSide) != "" {
			return input, ValidationError{Message: "a conversion side applies only to a currency exchange"}
		}
	}
	if !exchange {
		return input, nil
	}
	if len(input.JournalEntries) != 1 {
		return input, ValidationError{Message: "a currency exchange requires exactly one journal entry"}
	}
	if input.InvestmentOperationKind != "" || input.TransactionKind == "investment" {
		return input, ValidationError{Message: "a currency exchange is not an investment transaction"}
	}
	entry := input.JournalEntries[0]
	entryDate := strings.TrimSpace(entry.EntryDate)
	if entryDate == "" {
		entryDate = transactionDate
	}

	kinds := make([]exchangeLegKind, len(entry.Postings))
	for index, posting := range entry.Postings {
		date := entryDate
		if postedOn := strings.TrimSpace(posting.PostedOn); postedOn != "" {
			date = postedOn
		}
		rule, err := s.repository.PostingAccountRule(ctx, BookID, posting.AccountID, date)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				// cleanPosting explains a missing or not-yet-open account.
				kinds[index] = exchangeAccountLeg
				continue
			}
			return input, fmt.Errorf("read posting account rule: %w", err)
		}
		switch {
		case rule.IsSystem:
			return input, ValidationError{Message: "a currency exchange names no system account: the server adds the exchange postings"}
		case isPostedOnAccount(rule):
			kinds[index] = exchangeAccountLeg
		case rule.AccountClass == "expense":
			kinds[index] = exchangeFeeLeg
		default:
			return input, ValidationError{Message: "a currency exchange takes two account postings and at most one fee"}
		}
	}

	tradingAccountID, err := s.repository.SystemAccountID(ctx, BookID, "commodity_trading")
	if err != nil {
		return input, fmt.Errorf("read commodity trading account: %w", err)
	}
	built, err := exchangeLegs(entry, entryDate, kinds, tradingAccountID)
	if err != nil {
		return input, err
	}
	input.JournalEntries = []JournalEntryInput{built}
	return input, nil
}

type exchangeLegKind int

const (
	exchangeAccountLeg exchangeLegKind = iota + 1
	exchangeFeeLeg
)

// exchangeLegs is the exchange rule without its account lookups, shared by
// the writer and by TransferShape's rebuild of a stored journal.
func exchangeLegs(entry JournalEntryInput, entryDate string, kinds []exchangeLegKind, tradingAccountID int64) (JournalEntryInput, error) {
	var accounts []int
	fee := -1
	for index, kind := range kinds {
		switch kind {
		case exchangeAccountLeg:
			accounts = append(accounts, index)
		case exchangeFeeLeg:
			if fee >= 0 {
				return entry, ValidationError{Message: "a currency exchange takes two account postings and at most one fee"}
			}
			fee = index
		}
	}
	if len(accounts) != 2 {
		return entry, ValidationError{Message: "a currency exchange takes two account postings and at most one fee"}
	}
	first, second := entry.Postings[accounts[0]], entry.Postings[accounts[1]]
	if first.AccountID == second.AccountID {
		return entry, ValidationError{Message: "a currency exchange moves between two accounts; a multi-currency provider keeps one account per currency"}
	}
	if first.CommodityID == second.CommodityID {
		return entry, ValidationError{Message: "a currency exchange moves between two currencies; a same-currency transfer is an ordinary entry"}
	}
	if first.QuantityValue.Sign()*second.QuantityValue.Sign() >= 0 {
		return entry, ValidationError{Message: "a currency exchange takes one amount out of one account and one into the other"}
	}
	sending, receiving := accounts[0], accounts[1]
	if entry.Postings[sending].QuantityValue.Sign() > 0 {
		sending, receiving = receiving, sending
	}
	day := func(index int) string {
		if postedOn := strings.TrimSpace(entry.Postings[index].PostedOn); postedOn != "" {
			return postedOn
		}
		return entryDate
	}

	postings := append([]PostingInput{}, entry.Postings...)
	if fee >= 0 {
		feePosting := postings[fee]
		payer := -1
		for _, account := range []int{sending, receiving} {
			if entry.Postings[account].CommodityID == feePosting.CommodityID {
				payer = account
			}
		}
		if payer < 0 {
			return entry, ValidationError{Message: "an exchange fee is in the currency of the account that paid it"}
		}
		if feePosting.QuantityValue.Sign() <= 0 {
			return entry, ValidationError{Message: "an exchange fee must be positive"}
		}
		// The fee posts with the account that paid it, so cashflow sees the
		// payment and the fee together on that account's day.
		if postedOn := strings.TrimSpace(feePosting.PostedOn); postedOn != "" && postedOn != day(payer) {
			return entry, ValidationError{Message: "an exchange fee posts on the day of the account that paid it"}
		}
		postings[fee].PostedOn = day(payer)
		payerSide := conversionSending
		if payer == receiving {
			payerSide = conversionReceiving
		}
		if paidBy := strings.TrimSpace(entry.FeePaidBy); paidBy != "" && paidBy != payerSide {
			return entry, ValidationError{Message: "an exchange fee is in the currency of the account that paid it"}
		}
		entry.FeePaidBy = payerSide
	} else if strings.TrimSpace(entry.FeePaidBy) != "" {
		return entry, ValidationError{Message: "a fee payer applies to a transfer with one fee"}
	}

	// With the fee on top, each currency still moves one way: what left is
	// negative, what arrived positive. A fee larger than its side's amount
	// flips that sign and is refused rather than converted into its opposite.
	sums, commodities := sumPostingsByCommodity(entry.Postings)
	for _, account := range accounts {
		if sums[entry.Postings[account].CommodityID].Sign() != entry.Postings[account].QuantityValue.Sign() {
			return entry, ValidationError{Message: "an exchange fee exceeds the amount it is charged on"}
		}
	}

	converting, side := sending, conversionSending
	switch strings.TrimSpace(entry.ConversionSide) {
	case "", conversionSending:
	case conversionReceiving:
		converting, side = receiving, conversionReceiving
	default:
		return entry, ValidationError{Message: "conversion side must be 'sending' or 'receiving'"}
	}
	for _, commodityID := range commodities {
		value, err := sums[commodityID].Negated().Coefficient()
		if err != nil {
			return entry, ValidationError{Message: "posting amounts exceed the coefficient range"}
		}
		postings = append(postings, PostingInput{
			AccountID:     tradingAccountID,
			CommodityID:   commodityID,
			QuantityValue: value,
			QuantityScale: sums[commodityID].Scale(),
			PostedOn:      day(converting),
		})
	}
	entry.EntryKind = exchangeEntryKind
	// Stored, so the choice outlives a day on which both sides post (#481).
	entry.ConversionSide = side
	entry.Postings = postings
	return entry, nil
}

// isExchangeTradingLeg reports whether a stored posting is one of an
// exchange's server-built commodity_trading legs, which a reader presenting
// the exchange as one editable entry leaves out.
func isExchangeTradingLeg(entry JournalEntry, posting Posting) bool {
	return entry.EntryKind == exchangeEntryKind && posting.AccountSystemRole != nil && *posting.AccountSystemRole == "commodity_trading"
}

// exchangeRateScale is the precision of the rates an exchange's detail
// shows. Rates are display-only (#481), so this is a reading aid, never an
// input to a stored amount.
const exchangeRateScale = 6

// ExchangeQuantity is one exact amount of one commodity.
type ExchangeQuantity struct {
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
}

// ExchangeConversion explains a currency exchange against the market (#481).
//
// Sent and Received are the amounts converted, read from the exchange's own
// commodity_trading legs (a fee charged on top is outside them). The implied
// rate is Received per one unit of Sent. Market is the reporting rate on the
// conversion day, selected exactly as reports select it (the nearest earlier
// observation within the staleness window, else none); MarketGap names why
// there is none. Everything is computed on read from stored observations and
// nothing is written: when a rate arrives or is corrected later, the next
// read shows it.
type ExchangeConversion struct {
	ConversionDate string
	Sent           ExchangeQuantity
	Received       ExchangeQuantity
	ImpliedRate    ExchangeQuantity // CommodityID is the received commodity
	Market         *ExchangeMarket
	MarketGap      string
}

// ExchangeMarket is the exchange measured at the market rate. Cost is Sent
// minus MarketValue, in the sent currency: positive when the conversion
// delivered less than the market rate would have.
type ExchangeMarket struct {
	Rate            ExchangeQuantity // received commodity per one sent
	ObservationDate string
	Stale           bool
	MarketValue     ExchangeQuantity // Received valued in the sent currency
	Cost            ExchangeQuantity
	// CostPercent is Cost as a percentage of Sent, at two places.
	CostPercent exact.Coefficient
}

// ExchangeConversion returns the market comparison for a currency exchange,
// or nil for any other transaction. Without pricing data it reports only the
// implied rate.
func (s *TransactionService) ExchangeConversion(ctx context.Context, transaction Transaction) (*ExchangeConversion, error) {
	var sent, received *ExchangeQuantity
	conversionDate := ""
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if !isExchangeTradingLeg(entry, posting) {
				continue
			}
			// The trading legs post on the day the conversion happened.
			conversionDate = entry.EntryDate
			// The trading leg mirrors the account: positive where money left.
			leg := &ExchangeQuantity{CommodityID: posting.CommodityID, QuantityValue: posting.QuantityValue, QuantityScale: posting.QuantityScale}
			if posting.QuantityValue.Sign() > 0 {
				sent = leg
			} else {
				leg.QuantityValue = leg.QuantityValue.Negated()
				received = leg
			}
		}
	}
	if sent == nil || received == nil {
		return nil, nil
	}
	implied, err := exact.MulDivRound(received.QuantityValue.BigInt(), received.QuantityScale, big.NewInt(1), 0, sent.QuantityValue.BigInt(), sent.QuantityScale, exchangeRateScale)
	if err != nil {
		return nil, fmt.Errorf("exchange implied rate: %w", err)
	}
	impliedValue, err := exact.FromBig(implied)
	if err != nil {
		return nil, fmt.Errorf("exchange implied rate: %w", err)
	}
	conversion := &ExchangeConversion{
		ConversionDate: conversionDate,
		Sent:           *sent,
		Received:       *received,
		ImpliedRate:    ExchangeQuantity{CommodityID: received.CommodityID, QuantityValue: impliedValue, QuantityScale: exchangeRateScale},
	}
	if s.pricingRepository == nil {
		conversion.MarketGap = RateMissingNoObservation
		return conversion, nil
	}

	exactValue, scale, use, gap, err := s.exchangeMarketValue(ctx, *sent, *received, conversion.ConversionDate)
	if err != nil {
		return nil, err
	}
	if exactValue == nil {
		conversion.MarketGap = gap
		return conversion, nil
	}
	// One rounding each, from the exact market value: the value at the sent
	// currency's scale, the rate at six places.
	marketValue, err := exact.MulDivRound(exactValue.Num(), 0, big.NewInt(1), 0, exactValue.Denom(), 0, scale)
	if err != nil {
		return nil, fmt.Errorf("exchange market value: %w", err)
	}
	marketRate, err := exact.MulDivRound(new(big.Int).Mul(received.QuantityValue.BigInt(), exactValue.Denom()), received.QuantityScale, big.NewInt(1), 0, exactValue.Num(), 0, exchangeRateScale)
	if err != nil {
		return nil, fmt.Errorf("exchange market rate: %w", err)
	}
	cost := exact.ScaledIntFromCoefficient(sent.QuantityValue, sent.QuantityScale)
	cost.SubScaled(exact.ScaledIntFromBig(marketValue, scale))
	costValue, err := cost.Coefficient()
	if err != nil {
		return nil, fmt.Errorf("exchange cost: %w", err)
	}
	percent, err := exact.MulDivRound(cost.BigInt(), cost.Scale(), big.NewInt(100), 0, sent.QuantityValue.BigInt(), sent.QuantityScale, 2)
	if err != nil {
		return nil, fmt.Errorf("exchange cost percent: %w", err)
	}
	coefficients := make([]exact.Coefficient, 3)
	for index, value := range []*big.Int{marketRate, marketValue, percent} {
		if coefficients[index], err = exact.FromBig(value); err != nil {
			return nil, fmt.Errorf("exchange market comparison: %w", err)
		}
	}
	conversion.Market = &ExchangeMarket{
		Rate:            ExchangeQuantity{CommodityID: received.CommodityID, QuantityValue: coefficients[0], QuantityScale: exchangeRateScale},
		ObservationDate: use.ObservationDate,
		Stale:           use.Stale,
		MarketValue:     ExchangeQuantity{CommodityID: sent.CommodityID, QuantityValue: coefficients[1], QuantityScale: scale},
		Cost:            ExchangeQuantity{CommodityID: sent.CommodityID, QuantityValue: costValue, QuantityScale: cost.Scale()},
		CostPercent:     coefficients[2],
	}
	return conversion, nil
}

// exchangeMarketValue values the received amount in the sent currency at the
// market rate of the conversion day, exactly, with the reporting rate
// selection, and returns the sent currency's scale. A book may hold the pair
// either way round: the received currency priced in the sent one is used
// first, then the sent currency priced in the received one (received × sent
// ÷ the sent amount's value). Neither found returns nil and the first gap's
// reason.
func (s *TransactionService) exchangeMarketValue(ctx context.Context, sent, received ExchangeQuantity, date string) (*big.Rat, int, RateUse, string, error) {
	gap := RateMissingNoObservation
	direct, err := s.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: sent.CommodityID}, []int64{received.CommodityID}, date, date)
	if err != nil {
		return nil, 0, RateUse{}, "", fmt.Errorf("exchange market rate: %w", err)
	}
	scale := direct.Scale()
	if value, ok := direct.ConvertExact(received.QuantityValue, received.QuantityScale, received.CommodityID, date); ok && value.Sign() > 0 {
		return value, scale, firstRateUse(direct.Coverage("")), "", nil
	}
	if coverage := direct.Coverage(""); len(coverage.Gaps) > 0 {
		gap = coverage.Gaps[0].Reason
	}

	reverse, err := s.NewRateTable(ctx, ReportingCurrencyInput{CommodityID: received.CommodityID}, []int64{sent.CommodityID}, date, date)
	if err != nil {
		return nil, 0, RateUse{}, "", fmt.Errorf("exchange market rate: %w", err)
	}
	sentValue, ok := reverse.ConvertExact(sent.QuantityValue, sent.QuantityScale, sent.CommodityID, date)
	if !ok || sentValue.Sign() <= 0 {
		return nil, 0, RateUse{}, gap, nil
	}
	value := new(big.Rat).SetFrac(received.QuantityValue.BigInt(), exact.Pow10(received.QuantityScale))
	value.Mul(value, new(big.Rat).SetFrac(sent.QuantityValue.BigInt(), exact.Pow10(sent.QuantityScale)))
	value.Quo(value, sentValue)
	return value, scale, firstRateUse(reverse.Coverage("")), "", nil
}

func firstRateUse(coverage ValuationCoverage) RateUse {
	if len(coverage.Used) == 0 {
		return RateUse{}
	}
	return coverage.Used[0]
}
