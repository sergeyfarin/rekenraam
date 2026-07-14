package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestCleanInvestmentInstrumentSpecDefaultsAndNormalizesFields(t *testing.T) {
	quoteCommodityID := int64(10)
	tradingCommodityID := int64(20)
	spec, err := cleanInvestmentInstrumentSpec(InvestmentInstrumentInput{
		CommodityID:        ptrInt64(30),
		CommodityCode:      " VWRL ",
		DisplayName:        " Vanguard FTSE All-World UCITS ETF ",
		Symbol:             " vwrl ",
		ExchangeCode:       " ams ",
		MIC:                " xams ",
		Issuer:             " Vanguard ",
		CountryCode:        " nl ",
		QuoteCommodityID:   &quoteCommodityID,
		TradingCommodityID: &tradingCommodityID,
		QuantityScale:      6,
		PriceScale:         4,
		IdentifiersJSON:    `{" isin" : "IE00B3RBWM25" }`,
		MetadataJSON:       ``,
	})
	require.NoError(t, err)

	require.True(t, spec.CommodityID.Valid)
	assert.Equal(t, int64(30), spec.CommodityID.Int64)
	assert.Equal(t, "VWRL", spec.CommodityCode)
	assert.Equal(t, "stock", spec.InstrumentType)
	assert.Equal(t, "Vanguard FTSE All-World UCITS ETF", spec.DisplayName)
	assert.Equal(t, "VWRL", spec.Symbol)
	assert.Equal(t, "AMS", spec.ExchangeCode)
	assert.Equal(t, "XAMS", spec.MIC)
	assert.Equal(t, "NL", spec.CountryCode)
	assert.Equal(t, sql.NullInt64{Int64: quoteCommodityID, Valid: true}, spec.QuoteCommodityID)
	assert.Equal(t, sql.NullInt64{Int64: tradingCommodityID, Valid: true}, spec.TradingCommodityID)
	assert.Equal(t, `{" isin":"IE00B3RBWM25"}`, spec.IdentifiersJSON)
	assert.Equal(t, `{}`, spec.MetadataJSON)
}

func TestCleanInvestmentInstrumentSpecRejectsInvalidInputs(t *testing.T) {
	valid := InvestmentInstrumentInput{
		CommodityCode:   "VWRL",
		InstrumentType:  "etf",
		DisplayName:     "Vanguard FTSE All-World UCITS ETF",
		Symbol:          "VWRL",
		CountryCode:     "NL",
		QuantityScale:   6,
		PriceScale:      4,
		IdentifiersJSON: `{}`,
		MetadataJSON:    `{}`,
	}
	tests := []struct {
		name   string
		mutate func(*InvestmentInstrumentInput)
	}{
		{name: "invalid instrument type", mutate: func(input *InvestmentInstrumentInput) { input.InstrumentType = "crypto_derivative" }},
		{name: "missing display name", mutate: func(input *InvestmentInstrumentInput) { input.DisplayName = " " }},
		{name: "display name too long", mutate: func(input *InvestmentInstrumentInput) { input.DisplayName = strings.Repeat("a", 201) }},
		{name: "country code too short", mutate: func(input *InvestmentInstrumentInput) { input.CountryCode = "N" }},
		{name: "country code has digit", mutate: func(input *InvestmentInstrumentInput) { input.CountryCode = "N1" }},
		{name: "quantity scale too high", mutate: func(input *InvestmentInstrumentInput) { input.QuantityScale = 13 }},
		{name: "price scale negative", mutate: func(input *InvestmentInstrumentInput) { input.PriceScale = -1 }},
		{name: "identifiers must be object", mutate: func(input *InvestmentInstrumentInput) { input.IdentifiersJSON = `[]` }},
		{name: "metadata invalid json", mutate: func(input *InvestmentInstrumentInput) { input.MetadataJSON = `{` }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			_, err := cleanInvestmentInstrumentSpec(input)
			require.Error(t, err)
		})
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}

func TestPreviewSellRealizedGainAlignsMismatchedCostBasisScales(t *testing.T) {
	// Two buys create lots with different CostBasisScale (a lot's cost scale
	// is the cash scale of the buy that created it, which varies per trade —
	// realistic via import, see F1/F4 in the 2026-07-13 audit). Before the
	// fix, PreviewSell summed CostBasisValue across disposals as raw int64s
	// regardless of scale and exposed realized_gain with no scale field at
	// all, so the result was both numerically wrong and unrenderable.
	ctx := context.Background()
	database := openConnectionTestDatabase(t)

	res, err := database.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, 'EUR', 'currency', 1, '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	eurCommodityID, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', 'EUR', '€', 'Euro', 2, 6)
	`, eurCommodityID)
	require.NoError(t, err)

	res, err = database.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, 'TEST', 'security', 0, '2026-01-01T00:00:00Z', 1)
	`, BookID)
	require.NoError(t, err)
	stockCommodityID, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', 'TEST', 'TEST', 'Test Security', 0, 6)
	`, stockCommodityID)
	require.NoError(t, err)

	cashAccountID := seedTestAccount(t, database, "active", true)
	holdingAccountID := seedTestAccount(t, database, "active", true)
	seedCommodityTradingAccount(t, database)

	accountRepository := db.NewAccountRepository(database)
	accountService := NewAccountService(accountRepository, db.NewInstitutionRepository(database), nil)
	transactionService := NewTransactionService(db.NewTransactionRepository(database), db.NewPayeeRepository(database), accountRepository, db.NewCommodityRepository(database))
	investmentService := NewInvestmentService(db.NewInvestmentRepository(database), accountService, transactionService, nil)

	// Lot A: 100.000000 shares (scale 6) bought for 2500.00 EUR (scale 2).
	_, err = investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: 1, TransactionDate: "2026-01-01",
		CommodityID: stockCommodityID, HoldingAccountID: holdingAccountID, CashAccountID: cashAccountID,
		QuantityValue: exact.New(100000000), QuantityScale: 6,
		CashAmountValue: 250000, CashAmountScale: 2, CashCommodityID: eurCommodityID,
	})
	require.NoError(t, err)
	// Lot B: 20 shares (scale 0) bought for 40.000000 EUR (scale 6).
	_, err = investmentService.Buy(ctx, InvestmentTradeInput{
		OwnerUserID: 1, TransactionDate: "2026-02-01",
		CommodityID: stockCommodityID, HoldingAccountID: holdingAccountID, CashAccountID: cashAccountID,
		QuantityValue: exact.New(20), QuantityScale: 0,
		CashAmountValue: 40000000, CashAmountScale: 6, CashCommodityID: eurCommodityID,
	})
	require.NoError(t, err)

	// Sell all 120 shares (fifo disposes lot A then lot B) for 3000.00 EUR
	// (scale 2) proceeds.
	preview, err := investmentService.PreviewSell(ctx, InvestmentTradeInput{
		OwnerUserID: 1, TransactionDate: "2026-03-01",
		CommodityID: stockCommodityID, HoldingAccountID: holdingAccountID, CashAccountID: cashAccountID,
		QuantityValue: exact.New(120), QuantityScale: 0,
		CashAmountValue: 300000, CashAmountScale: 2, CashCommodityID: eurCommodityID,
	})
	require.NoError(t, err)

	// Disposed basis aligned at scale 6: 2500.00 -> 2,500,000,000 plus
	// 40,000,000 = 2,540,000,000 (2540.000000). Proceeds aligned at scale 6:
	// 3000.00 -> 3,000,000,000. Gain = 460,000,000 at scale 6 (460.000000).
	// The pre-fix code summed the raw CostBasisValue int64s (250000 +
	// 40000000 = 40,250,000) against the unaligned cash value, producing a
	// nonsensical negative gain.
	assert.Equal(t, int64(460000000), preview.RealizedGain)
	assert.Equal(t, 6, preview.RealizedGainScale)
}
