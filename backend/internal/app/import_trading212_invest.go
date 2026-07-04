package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// commitTrading212InvestmentRow attempts to route a Trading 212 order-fill or
// dividend staged row through InvestmentService.Buy/Sell/Dividend instead of
// the generic buildTransactionSpec path (see CommitImportBatch).
//
// handled=false means either this isn't a trading212 investment row, or
// investment resolution/posting failed for any reason — the caller falls
// back to the generic commit path unchanged. This is a strict superset of
// pre-Slice-4b behavior: a row that would have committed as a plain cash
// transaction before still can if anything here doesn't work out (no
// cash_account_id configured, no instrument match, insufficient lots for a
// sell, no dividend income-account default configured, ...). Nothing here
// is required for a row to commit.
//
// All instrument/holding-account resolution AND creation happens here, at
// commit time, never at fetch time — see the discard-orphan concern in
// docs/import-connection-accounts-plan.md.
func (s *ImportService) commitTrading212InvestmentRow(ctx context.Context, row db.ImportStagedRowRecord, batch db.ImportBatchRecord, input CommitImportBatchInput, nowStr string) (handled bool, transactionID int64, identityAccountID int64, err error) {
	if s.investmentService == nil || s.connectionService == nil || !batch.ConnectionID.Valid {
		return false, 0, 0, nil
	}

	var raw map[string]string
	if jsonErr := json.Unmarshal([]byte(row.RawJSON), &raw); jsonErr != nil {
		return false, 0, 0, nil
	}
	kind := raw[rawKeyKind]
	if kind != trading212RawKindOrderFill && kind != trading212RawKindDividend {
		return false, 0, 0, nil
	}

	connectionID := batch.ConnectionID.Int64
	conn, connErr := s.connectionService.GetImportConnection(ctx, connectionID)
	if connErr != nil || conn.CashAccountID == nil {
		return false, 0, 0, nil // no settlement account configured — needs_attention fallback.
	}

	var normalized struct {
		Date          string `json:"date"`
		Amount        string `json:"amount"`
		CommodityHint string `json:"commodity_hint"`
	}
	if jsonErr := json.Unmarshal([]byte(row.NormalizedJSON), &normalized); jsonErr != nil || normalized.Date == "" || normalized.CommodityHint == "" {
		return false, 0, 0, nil
	}

	cashCurrency, err := s.accountRepository.CurrentCurrencyByCode(ctx, BookID, normalized.CommodityHint)
	if err != nil {
		return false, 0, 0, nil // unknown currency — can't resolve, generic path still works.
	}

	instrumentID, commodityID, err := s.investmentService.ResolveOrCreateInstrumentForImport(
		ctx, input.OwnerUserID, input.AuthSessionID, input.RequestID, raw[rawKeyISIN], raw[rawKeyTicker], normalized.Date,
	)
	if err != nil {
		return false, 0, 0, nil
	}

	switch kind {
	case trading212RawKindOrderFill:
		holdingAccountID, err := s.resolveTrading212HoldingAccount(ctx, connectionID, instrumentID, commodityID, normalized.Date, input, nowStr)
		if err != nil {
			return false, 0, 0, nil
		}
		handled, txnID, err := s.commitTrading212OrderFill(ctx, raw, normalized.Date, commodityID, holdingAccountID, *conn.CashAccountID, cashCurrency.ID, input)
		return handled, txnID, holdingAccountID, err
	case trading212RawKindDividend:
		handled, txnID, err := s.commitTrading212Dividend(ctx, raw, normalized.Date, normalized.Amount, commodityID, *conn.CashAccountID, cashCurrency.ID, input)
		return handled, txnID, *conn.CashAccountID, err
	default:
		return false, 0, 0, nil
	}
}

// resolveTrading212HoldingAccount reuses the (connection, commodity) ->
// holding account mapping if one exists, or creates a new holding account
// and records the mapping — the "scenario 1: brand new instrument" path
// from docs/import-connection-accounts-plan.md. The "scenario 2: link to an
// existing holding account with explicit confirmation" path is not
// implemented here (tracked as a follow-up, see docs/backlog.md
// B-T212-INVST) — an ambiguous or ignorable existing match simply isn't
// looked for; this always either reuses a connection-linked account or
// creates a fresh one.
func (s *ImportService) resolveTrading212HoldingAccount(ctx context.Context, connectionID int64, instrumentID int64, commodityID int64, tradeDate string, input CommitImportBatchInput, nowStr string) (int64, error) {
	if holdingAccountID, found, err := s.connectionService.HoldingAccountForCommodity(ctx, connectionID, commodityID); err != nil {
		return 0, err
	} else if found {
		return holdingAccountID, nil
	}

	instrument, err := s.investmentService.Instrument(ctx, instrumentID)
	if err != nil {
		return 0, fmt.Errorf("load instrument for holding account: %w", err)
	}
	// OpenedOn/EffectiveFrom must be the trade's own date, not "today" (the
	// CreateAccount default) — otherwise a holding account created while
	// importing a back-dated fill would be "opened" after that fill's own
	// date and every posting to it would fail "posting account is invalid"
	// (PostingAccountRule finds no account version effective as of the
	// entry date).
	account, err := s.investmentService.CreateHoldingAccount(ctx, HoldingAccountInput{
		OwnerUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		InstrumentID:  instrumentID,
		Name:          instrument.DisplayName,
		OpenedOn:      tradeDate,
		EffectiveFrom: tradeDate,
		ChangeReason:  "created from Trading 212 import",
	})
	if err != nil {
		return 0, fmt.Errorf("create holding account for import: %w", err)
	}
	if err := s.connectionService.LinkHolding(ctx, connectionID, commodityID, account.ID, nowStr); err != nil {
		return 0, fmt.Errorf("link import connection holding: %w", err)
	}
	return account.ID, nil
}

func (s *ImportService) commitTrading212OrderFill(ctx context.Context, raw map[string]string, date string, commodityID int64, holdingAccountID int64, cashAccountID int64, cashCommodityID int64, input CommitImportBatchInput) (bool, int64, error) {
	side := strings.ToUpper(strings.TrimSpace(raw[rawKeySide]))
	if side != "BUY" && side != "SELL" {
		return false, 0, nil
	}
	quantity, quantityScale, err := parsePositiveCoefficient(raw[rawKeyQuantity])
	if err != nil {
		return false, 0, nil
	}
	cashValue, cashScale, err := parsePositiveDecimalAmount(raw["net_value"])
	if err != nil {
		return false, 0, nil
	}

	tradeInput := InvestmentTradeInput{
		OwnerUserID:      input.OwnerUserID,
		AuthSessionID:    input.AuthSessionID,
		RequestID:        input.RequestID,
		TransactionDate:  date,
		CommodityID:      commodityID,
		HoldingAccountID: holdingAccountID,
		CashAccountID:    cashAccountID,
		QuantityValue:    quantity,
		QuantityScale:    quantityScale,
		CashAmountValue:  cashValue,
		CashAmountScale:  cashScale,
		CashCommodityID:  cashCommodityID,
		Memo:             strings.TrimSpace(side + " " + raw[rawKeyTicker]),
		ChangeReason:     "imported from trading212",
		OriginType:       "import",
		Operation:        "investment." + strings.ToLower(side) + ".import",
	}

	var (
		transaction Transaction
		tradeErr    error
	)
	if side == "BUY" {
		result, err := s.investmentService.Buy(ctx, tradeInput)
		transaction, tradeErr = result.Transaction, err
	} else {
		result, err := s.investmentService.Sell(ctx, tradeInput)
		transaction, tradeErr = result.Transaction, err
	}
	if tradeErr != nil {
		// Insufficient lots (partial history fetched, or an unhandled
		// corporate action) and any other posting error both fall back to
		// the generic path rather than failing the whole batch.
		return false, 0, nil
	}
	return true, transaction.ID, nil
}

func (s *ImportService) commitTrading212Dividend(ctx context.Context, raw map[string]string, date string, rawAmount string, commodityID int64, cashAccountID int64, cashCommodityID int64, input CommitImportBatchInput) (bool, int64, error) {
	amountValue, amountScale, err := parsePositiveDecimalAmount(rawAmount)
	if err != nil {
		return false, 0, nil
	}

	transaction, err := s.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID:     input.OwnerUserID,
		AuthSessionID:   input.AuthSessionID,
		RequestID:       input.RequestID,
		TransactionDate: date,
		CommodityID:     &commodityID,
		CashAccountID:   cashAccountID,
		CashCommodityID: cashCommodityID,
		AmountValue:     amountValue,
		AmountScale:     amountScale,
		Memo:            "Dividend: " + raw[rawKeyTicker],
		ChangeReason:    "imported from trading212",
		OriginType:      "import",
		Operation:       "investment.dividend.import",
	})
	if err != nil {
		// No dividend default configured for this instrument yet (income
		// account unresolved) and any other posting error both fall back to
		// the generic path rather than failing the whole batch.
		return false, 0, nil
	}
	return true, transaction.ID, nil
}

// parsePositiveCoefficient parses a raw decimal string into an
// always-positive exact.Coefficient + scale — Buy/Sell expect a positive
// QuantityValue and apply the correct sign internally per posting
// direction/side.
func parsePositiveCoefficient(raw string) (exact.Coefficient, int, error) {
	coeff, scale, err := parseDecimalAmount(raw)
	if err != nil {
		return "", 0, err
	}
	if coeff.Sign() < 0 {
		coeff = coeff.Negated()
	}
	return coeff, scale, nil
}

// parsePositiveDecimalAmount parses a raw decimal string (as produced by the
// Trading 212 adapter) into an always-positive magnitude + scale —
// InvestmentService.Buy/Sell/Dividend all expect a positive CashAmountValue/
// AmountValue and apply the correct sign internally per posting direction,
// unlike the generic cash path which posts the raw signed amount as-is.
func parsePositiveDecimalAmount(raw string) (int64, int, error) {
	coeff, scale, err := parsePositiveCoefficient(raw)
	if err != nil {
		return 0, 0, err
	}
	value, ok := new(big.Int).SetString(coeff.String(), 10)
	if !ok {
		return 0, 0, fmt.Errorf("invalid amount: %s", coeff)
	}
	if !value.IsInt64() {
		return 0, 0, fmt.Errorf("amount out of int64 range: %s", coeff)
	}
	return value.Int64(), scale, nil
}
