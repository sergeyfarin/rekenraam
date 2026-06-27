package db

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

func TestInvestmentInstrumentCreatesSecurityCommodityAndVersionHistory(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)

	repository := NewInvestmentRepository(database)
	instrument, err := repository.CreateInstrument(ctx, CreateInvestmentInstrumentParams{
		BookID:          1,
		CreatedByUserID: ownerID,
		RequestID:       "request-create-instrument",
		OriginType:      "browser_api",
		Operation:       "investment.instrument.create",
		CreatedAt:       "2026-06-12T10:00:00Z",
		EffectiveFrom:   "2026-06-12",
		ChangeReason:    "created test ETF",
		Spec: InvestmentInstrumentSpec{
			CommodityCode:      "VWRL",
			InstrumentType:     "etf",
			DisplayName:        "Vanguard FTSE All-World UCITS ETF",
			Symbol:             "VWRL",
			ExchangeCode:       "AMS",
			MIC:                "XAMS",
			CountryCode:        "NL",
			QuoteCommodityID:   sql.NullInt64{Int64: currencyID, Valid: true},
			TradingCommodityID: sql.NullInt64{Int64: currencyID, Valid: true},
			QuantityScale:      6,
			PriceScale:         6,
			IdentifiersJSON:    `{"isin":"IE00B3RBWM25"}`,
			MetadataJSON:       `{"provider":"manual"}`,
		},
	})
	require.NoError(t, err)

	assert.NotZero(t, instrument.ID)
	assert.NotZero(t, instrument.CommodityID)
	assert.Equal(t, "security", readCommodityKind(t, database, instrument.CommodityID))
	assert.Equal(t, int64(1), instrument.VersionSeq)
	assert.Equal(t, "VWRL", instrument.Symbol.String)
	assert.Equal(t, currencyID, instrument.QuoteCommodityID.Int64)

	updated, err := repository.UpdateInstrument(ctx, UpdateInvestmentInstrumentParams{
		BookID:          1,
		InstrumentID:    instrument.ID,
		ChangedByUserID: ownerID,
		RequestID:       "request-update-instrument",
		OriginType:      "browser_api",
		Operation:       "investment.instrument.update",
		RecordedAt:      "2026-06-13T10:00:00Z",
		EffectiveFrom:   "2026-06-12",
		ChangeReason:    "ticker changed",
		Spec: InvestmentInstrumentSpec{
			CommodityID:        sql.NullInt64{Int64: instrument.CommodityID, Valid: true},
			InstrumentType:     "etf",
			DisplayName:        "Vanguard FTSE All-World UCITS ETF",
			Symbol:             "VWCE",
			ExchangeCode:       "AMS",
			MIC:                "XAMS",
			CountryCode:        "NL",
			QuoteCommodityID:   sql.NullInt64{Int64: currencyID, Valid: true},
			TradingCommodityID: sql.NullInt64{Int64: currencyID, Valid: true},
			QuantityScale:      6,
			PriceScale:         6,
			IdentifiersJSON:    `{"isin":"IE00BK5BQT80"}`,
			MetadataJSON:       `{"provider":"manual"}`,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, int64(2), updated.VersionSeq)
	assert.Equal(t, "VWCE", updated.Symbol.String)
	assert.Equal(t, 2, countRows(t, database, "investment_instrument_versions"))
}

func TestInvestmentLotsDisposeFIFOAndPreserveRemainingBasis(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewInvestmentRepository(database)

	firstLot, err := repository.CreateLot(ctx, CreateInvestmentLotParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		OpenedOn:        "2026-01-01",
		QuantityValue:   exact.New(100000000),
		QuantityScale:   6,
		CostBasisValue:  250000,
		CostBasisScale:  2,
		CostCommodityID: currencyID,
		MetadataJSON:    `{}`,
		CreatedAt:       "2026-01-01T09:00:00Z",
		CreatedByUserID: ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.acquire",
		ChangeReason:    "first lot",
		EventKind:       "acquisition",
	})
	require.NoError(t, err)
	_, err = repository.CreateLot(ctx, CreateInvestmentLotParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		OpenedOn:        "2026-02-01",
		QuantityValue:   exact.New(50000000),
		QuantityScale:   6,
		CostBasisValue:  150000,
		CostBasisScale:  2,
		CostCommodityID: currencyID,
		MetadataJSON:    `{}`,
		CreatedAt:       "2026-02-01T09:00:00Z",
		CreatedByUserID: ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.acquire",
		ChangeReason:    "second lot",
		EventKind:       "acquisition",
	})
	require.NoError(t, err)

	disposals, err := repository.DisposeLots(ctx, DisposeLotsParams{
		BookID:        1,
		AccountID:     accountID,
		CommodityID:   instrument.CommodityID,
		TransactionID: 0,
		EventDate:     "2026-03-01",
		QuantityValue: exact.New(125000000),
		QuantityScale: 6,
		CreatedAt:     "2026-03-01T09:00:00Z",
		ActorUserID:   ownerID,
		OriginType:    "browser_api",
		Operation:     "investment.lot.dispose",
		ChangeReason:  "fifo sale",
	})
	require.NoError(t, err)

	require.Len(t, disposals, 2)
	assert.Equal(t, firstLot.ID, disposals[0].LotID)
	assert.Equal(t, exact.New(100000000), disposals[0].QuantityValue)
	assert.Equal(t, int64(250000), disposals[0].CostBasisValue)
	assert.Equal(t, exact.New(25000000), disposals[1].QuantityValue)
	assert.Equal(t, int64(75000), disposals[1].CostBasisValue)

	lots, err := repository.ListLots(ctx, 1, accountID, instrument.CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 2)
	assert.Equal(t, "closed", lots[0].Status)
	assert.Equal(t, exact.New(0), lots[0].RemainingQuantityValue)
	assert.Equal(t, "open", lots[1].Status)
	assert.Equal(t, exact.New(25000000), lots[1].RemainingQuantityValue)
	assert.Equal(t, int64(75000), lots[1].RemainingCostBasisValue)
}

func TestCreateTransactionAndDisposeLotsRollsBackTransactionOnInsufficientLots(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewInvestmentRepository(database)
	before := countRows(t, database, "transactions")

	_, _, err := repository.CreateTransactionAndDisposeLots(ctx, CreateTransactionParams{
		BookID:      1,
		ActorUserID: ownerID,
		RequestID:   "atomic-sell",
		OriginType:  "browser_api",
		Operation:   "investment.sell",
		Spec: TransactionSpec{
			Status:          "posted",
			TransactionKind: "investment",
			TransactionDate: "2026-03-01",
			Description:     "sell without lots",
			MetadataJSON:    "{}",
		},
		CreatedAt:    "2026-03-01T09:00:00Z",
		ChangeReason: "test atomic sell",
	}, DisposeLotsParams{
		BookID:        1,
		AccountID:     accountID,
		CommodityID:   instrument.CommodityID,
		EventDate:     "2026-03-01",
		QuantityValue: exact.New(1),
		QuantityScale: 0,
		CreatedAt:     "2026-03-01T09:00:00Z",
		ActorUserID:   ownerID,
		RequestID:     "atomic-sell",
		OriginType:    "browser_api",
		Operation:     "investment.lot.dispose",
		ChangeReason:  "test atomic sell",
	})
	require.ErrorIs(t, err, ErrInsufficientLots)
	assert.Equal(t, before, countRows(t, database, "transactions"))
}

func TestInvestmentLotsRejectInsufficientFIFOAndRollBack(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewInvestmentRepository(database)
	_, err := repository.CreateLot(ctx, CreateInvestmentLotParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		OpenedOn:        "2026-01-01",
		QuantityValue:   exact.New(10000000),
		QuantityScale:   6,
		CostBasisValue:  25000,
		CostBasisScale:  2,
		CostCommodityID: currencyID,
		MetadataJSON:    `{}`,
		CreatedAt:       "2026-01-01T09:00:00Z",
		CreatedByUserID: ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.acquire",
		ChangeReason:    "first lot",
		EventKind:       "acquisition",
	})
	require.NoError(t, err)

	_, err = repository.DisposeLots(ctx, DisposeLotsParams{
		BookID:        1,
		AccountID:     accountID,
		CommodityID:   instrument.CommodityID,
		EventDate:     "2026-03-01",
		QuantityValue: exact.New(15000000),
		QuantityScale: 6,
		CreatedAt:     "2026-03-01T09:00:00Z",
		ActorUserID:   ownerID,
		OriginType:    "browser_api",
		Operation:     "investment.lot.dispose",
		ChangeReason:  "oversell",
	})
	require.ErrorIs(t, err, ErrInsufficientLots)

	lots, err := repository.ListLots(ctx, 1, accountID, instrument.CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "open", lots[0].Status)
	assert.Equal(t, exact.New(10000000), lots[0].RemainingQuantityValue)
	assert.Equal(t, int64(25000), lots[0].RemainingCostBasisValue)
	assert.Equal(t, 1, countRows(t, database, "investment_lot_events"))
}

func TestInvestmentLotsProrateRoundingIntoFinalDisposal(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewInvestmentRepository(database)
	lot, err := repository.CreateLot(ctx, CreateInvestmentLotParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		OpenedOn:        "2026-01-01",
		QuantityValue:   exact.New(3),
		QuantityScale:   0,
		CostBasisValue:  100,
		CostBasisScale:  2,
		CostCommodityID: currencyID,
		MetadataJSON:    `{}`,
		CreatedAt:       "2026-01-01T09:00:00Z",
		CreatedByUserID: ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.acquire",
		ChangeReason:    "fractional rounding test",
		EventKind:       "acquisition",
	})
	require.NoError(t, err)

	first, err := repository.DisposeLots(ctx, DisposeLotsParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		EventDate:       "2026-03-01",
		QuantityValue:   exact.New(1),
		QuantityScale:   0,
		CostBasisMethod: "specific_lot",
		Allocations:     []LotAllocation{{LotID: lot.ID, QuantityValue: exact.New(1), QuantityScale: 0}},
		CreatedAt:       "2026-03-01T09:00:00Z",
		ActorUserID:     ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.dispose",
		ChangeReason:    "first sale",
	})
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, int64(33), first[0].CostBasisValue)

	second, err := repository.DisposeLots(ctx, DisposeLotsParams{
		BookID:          1,
		AccountID:       accountID,
		CommodityID:     instrument.CommodityID,
		EventDate:       "2026-04-01",
		QuantityValue:   exact.New(2),
		QuantityScale:   0,
		CostBasisMethod: "specific_lot",
		Allocations:     []LotAllocation{{LotID: lot.ID, QuantityValue: exact.New(2), QuantityScale: 0}},
		CreatedAt:       "2026-04-01T09:00:00Z",
		ActorUserID:     ownerID,
		OriginType:      "browser_api",
		Operation:       "investment.lot.dispose",
		ChangeReason:    "final sale",
	})
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, int64(67), second[0].CostBasisValue)

	lots, err := repository.ListLots(ctx, 1, accountID, instrument.CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "closed", lots[0].Status)
	assert.Equal(t, exact.New(0), lots[0].RemainingQuantityValue)
	assert.Equal(t, int64(0), lots[0].RemainingCostBasisValue)
}

func TestPricingObservationStoresExactProviderAndManualData(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewPricingRepository(database)

	manualSourceID, err := repository.ManualSourceID(ctx)
	require.NoError(t, err)
	price, err := repository.CreatePriceObservation(ctx, CreatePriceObservationParams{
		BookID:       1,
		ActorUserID:  ownerID,
		RequestID:    "request-price",
		OriginType:   "browser_api",
		Operation:    "pricing.price.create",
		CreatedAt:    "2026-06-12T12:00:00Z",
		ChangeReason: "manual quote",
		Spec: PriceObservationSpec{
			BaseCommodityID:    instrument.CommodityID,
			QuoteCommodityID:   currencyID,
			QuoteType:          "manual",
			AdjustmentBasis:    "raw",
			PriceValue:         123456789,
			PriceScale:         6,
			BaseQuantityValue:  1,
			BaseQuantityScale:  0,
			ValuationDate:      "2026-06-12",
			ObservedAt:         sql.NullString{String: "2026-06-12T11:59:00Z", Valid: true},
			SourcePublishedAt:  sql.NullString{String: "2026-06-12T12:00:00Z", Valid: true},
			SourceID:           sql.NullInt64{Int64: manualSourceID, Valid: true},
			IsManual:           true,
			DerivationJSON:     `{}`,
			SeriesMetadataJSON: `{"series":"manual-test"}`,
			MetadataJSON:       `{"source":"test"}`,
		},
	})
	require.NoError(t, err)

	assert.NotZero(t, price.SeriesID)
	assert.Equal(t, int64(123456789), price.PriceValue)
	assert.Equal(t, 6, price.PriceScale)
	assert.Equal(t, int64(1), price.BaseQuantityValue)
	assert.Equal(t, "manual", price.QuoteType)
	assert.Equal(t, "raw", price.AdjustmentBasis)
	assert.Equal(t, "2026-06-12", price.ValuationDate)
	assert.Equal(t, "2026-06-12T11:59:00Z", price.ObservedAt.String)
	assert.Equal(t, "2026-06-12T12:00:00Z", price.SourcePublishedAt.String)
	assert.True(t, price.IsManual)
	assert.False(t, price.IsDerived)

	prices, err := repository.ListPriceObservations(ctx, 1, instrument.CommodityID, currencyID, 10)
	require.NoError(t, err)
	require.Len(t, prices, 1)
	assert.Equal(t, price.ID, prices[0].ID)
	assert.Equal(t, 1, countRows(t, database, "price_series"))
}

func TestPricingObservationRejectsNonPositivePriceAtStorage(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repository := NewPricingRepository(database)
	manualSourceID, err := repository.ManualSourceID(ctx)
	require.NoError(t, err)

	for _, priceValue := range []int64{0, -1} {
		t.Run(strconv.FormatInt(priceValue, 10), func(t *testing.T) {
			_, err := repository.CreatePriceObservation(ctx, CreatePriceObservationParams{
				BookID:       1,
				ActorUserID:  ownerID,
				RequestID:    "request-price-invalid",
				OriginType:   "browser_api",
				Operation:    "pricing.price.create",
				CreatedAt:    "2026-06-12T12:00:00Z",
				ChangeReason: "invalid quote",
				Spec: PriceObservationSpec{
					BaseCommodityID:    instrument.CommodityID,
					QuoteCommodityID:   currencyID,
					QuoteType:          "manual",
					AdjustmentBasis:    "raw",
					PriceValue:         priceValue,
					PriceScale:         6,
					BaseQuantityValue:  1,
					BaseQuantityScale:  0,
					ValuationDate:      "2026-06-12",
					SourceID:           sql.NullInt64{Int64: manualSourceID, Valid: true},
					IsManual:           true,
					DerivationJSON:     `{}`,
					SeriesMetadataJSON: `{}`,
					MetadataJSON:       `{}`,
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "price observation value must be positive")
		})
	}
}

func TestDefaultCostBasisProfileIsCreatedOnce(t *testing.T) {
	ctx := context.Background()
	database, ownerID, _ := migratedInvestmentTestDatabase(t)
	repository := NewInvestmentRepository(database)

	first, err := repository.EnsureDefaultCostBasisProfile(ctx, 1, ownerID, 0, "request-default-1", "2026-06-12T12:00:00Z")
	require.NoError(t, err)
	second, err := repository.EnsureDefaultCostBasisProfile(ctx, 1, ownerID, 0, "request-default-2", "2026-06-12T12:01:00Z")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "fifo", first.Method)
	assert.True(t, first.IsDefault)
	assert.Equal(t, 1, countRows(t, database, "cost_basis_profiles"))
}

func TestAutomationRuleCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	var manualSourceID int64
	require.NoError(t, database.QueryRowContext(ctx, `SELECT id FROM market_data_sources WHERE code = 'manual'`).Scan(&manualSourceID))
	repository := NewInvestmentRepository(database)

	created, err := repository.SaveAutomationRule(ctx, SaveAutomationRuleParams{
		BookID:       1,
		ActorUserID:  ownerID,
		RequestID:    "request-rule-create",
		OriginType:   "browser_api",
		Operation:    "investment.automation_rule.create",
		RecordedAt:   "2026-06-12T12:00:00Z",
		ChangeReason: "create rule",
		Spec: AutomationRuleSpec{
			SourceID:               sql.NullInt64{Int64: manualSourceID, Valid: true},
			InstrumentID:           sql.NullInt64{Int64: instrument.ID, Valid: true},
			EventFamily:            "dividend",
			Mode:                   "suggest",
			ConfidenceThresholdBPS: 9000,
			RequiredAccountsJSON:   `{"income_account_id":12}`,
			Status:                 "active",
			EffectiveFrom:          "2026-06-12",
		},
	})
	require.NoError(t, err)

	assert.NotZero(t, created.ID)
	assert.Equal(t, "suggest", created.Mode)
	assert.Equal(t, 9000, created.ConfidenceThresholdBPS)

	updated, err := repository.SaveAutomationRule(ctx, SaveAutomationRuleParams{
		BookID:       1,
		RuleID:       created.ID,
		ActorUserID:  ownerID,
		RequestID:    "request-rule-update",
		OriginType:   "browser_api",
		Operation:    "investment.automation_rule.update",
		RecordedAt:   "2026-06-12T12:01:00Z",
		ChangeReason: "trust source",
		Spec: AutomationRuleSpec{
			SourceID:               sql.NullInt64{Int64: manualSourceID, Valid: true},
			InstrumentID:           sql.NullInt64{Int64: instrument.ID, Valid: true},
			EventFamily:            "dividend",
			Mode:                   "auto_post",
			ConfidenceThresholdBPS: 9900,
			RequiredAccountsJSON:   `{"income_account_id":12,"cash_account_id":34}`,
			Status:                 "active",
			EffectiveFrom:          "2026-06-12",
		},
	})
	require.NoError(t, err)

	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "auto_post", updated.Mode)
	assert.Equal(t, 9900, updated.ConfidenceThresholdBPS)
	assert.Equal(t, 1, countRows(t, database, "investment_automation_rules"))
}

func migratedInvestmentTestDatabase(t *testing.T) (*sql.DB, int64, int64) {
	t.Helper()
	ctx := context.Background()
	database := openTestDatabase(t)
	require.NoError(t, Migrate(ctx, database))

	owner, err := NewSetupRepository(database).CompleteOwnerSetup(ctx, CompleteOwnerSetupParams{
		Username:         "owner",
		PasswordHash:     "password-hash",
		SessionTokenHash: "session-token-hash",
		CreatedAt:        "2026-06-12T08:00:00Z",
		SessionExpiresAt: "2026-06-13T08:00:00Z",
	})
	require.NoError(t, err)

	_, err = NewBookRepository(database).CompleteBookSetup(ctx, CompleteBookSetupParams{
		OwnerUserID: owner.ID,
		RequestID:   "request-book",
		OriginType:  "browser_api",
		Operation:   "setup.book.complete",
		Code:        "main",
		Name:        "Main",
		CreatedAt:   "2026-06-12T08:01:00Z",
	})
	require.NoError(t, err)

	currency, err := NewCommodityRepository(database).CreateCurrency(ctx, CreateCurrencyParams{
		BookID:          1,
		CreatedByUserID: owner.ID,
		RequestID:       "request-currency",
		OriginType:      "browser_api",
		Operation:       "currency.create",
		CreatedAt:       "2026-06-12T08:02:00Z",
		EffectiveFrom:   "2026-06-12",
		Spec: CurrencySpec{
			Code:             "EUR",
			Name:             "Euro",
			Symbol:           "EUR",
			StandardScale:    2,
			MaxQuantityScale: 6,
		},
	})
	require.NoError(t, err)

	return database, owner.ID, currency.ID
}

func createInvestmentTestInstrument(t *testing.T, database *sql.DB, ownerID int64, currencyID int64) InvestmentInstrumentRecord {
	t.Helper()
	instrument, err := NewInvestmentRepository(database).CreateInstrument(context.Background(), CreateInvestmentInstrumentParams{
		BookID:          1,
		CreatedByUserID: ownerID,
		RequestID:       "request-test-instrument",
		OriginType:      "browser_api",
		Operation:       "investment.instrument.create",
		CreatedAt:       "2026-06-12T10:00:00Z",
		EffectiveFrom:   "2026-06-12",
		ChangeReason:    "test instrument",
		Spec: InvestmentInstrumentSpec{
			CommodityCode:      "TEST",
			InstrumentType:     "stock",
			DisplayName:        "Test Security",
			Symbol:             "TEST",
			QuoteCommodityID:   sql.NullInt64{Int64: currencyID, Valid: true},
			TradingCommodityID: sql.NullInt64{Int64: currencyID, Valid: true},
			QuantityScale:      6,
			PriceScale:         6,
			IdentifiersJSON:    `{}`,
			MetadataJSON:       `{}`,
		},
	})
	require.NoError(t, err)
	return instrument
}

func createInvestmentTestAccount(t *testing.T, database *sql.DB, defaultCommodityID int64) int64 {
	t.Helper()
	result, err := database.ExecContext(context.Background(), `
		INSERT INTO accounts (
			book_id, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id
		)
		VALUES (1, NULL, '2026-06-12T09:00:00Z', 1, NULL, NULL)
	`)
	require.NoError(t, err)
	accountID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(context.Background(), `
		INSERT INTO account_versions (
			account_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, opened_on, closed_on, code, name, account_class,
			account_kind, parent_account_id, institution_id, country_code, default_commodity_id,
			quantity_scale_override, allows_postings, number_last4, external_ref_hint,
			comment_markdown, metadata_json, change_audit_event_id
		)
		VALUES (?, 1, '2026-06-12', '2026-06-12T09:00:00Z', 1, 'test account',
			'active', '2026-06-12', NULL, 'BROKERAGE', 'Brokerage', 'asset',
			'brokerage', NULL, NULL, '', ?, NULL, 1, '', '', '', '{}', NULL)
	`, accountID, defaultCommodityID)
	require.NoError(t, err)
	return accountID
}

func readCommodityKind(t *testing.T, database *sql.DB, commodityID int64) string {
	t.Helper()
	var kind string
	require.NoError(t, database.QueryRowContext(context.Background(), `SELECT kind FROM commodities WHERE id = ?`, commodityID).Scan(&kind))
	return kind
}

func countRows(t *testing.T, database *sql.DB, tableName string) int {
	t.Helper()
	var count int
	err := database.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM `+tableName).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestInvestmentInstrumentRejectsDuplicateCommodity(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	first := createInvestmentTestInstrument(t, database, ownerID, currencyID)

	_, err := NewInvestmentRepository(database).CreateInstrument(ctx, CreateInvestmentInstrumentParams{
		BookID:          1,
		CreatedByUserID: ownerID,
		RequestID:       "request-duplicate-instrument",
		OriginType:      "browser_api",
		Operation:       "investment.instrument.create",
		CreatedAt:       "2026-06-12T11:00:00Z",
		EffectiveFrom:   "2026-06-12",
		ChangeReason:    "duplicate",
		Spec: InvestmentInstrumentSpec{
			CommodityID:        sql.NullInt64{Int64: first.CommodityID, Valid: true},
			InstrumentType:     "stock",
			DisplayName:        "Duplicate Test Security",
			Symbol:             "TEST2",
			QuoteCommodityID:   sql.NullInt64{Int64: currencyID, Valid: true},
			TradingCommodityID: sql.NullInt64{Int64: currencyID, Valid: true},
			QuantityScale:      6,
			PriceScale:         6,
			IdentifiersJSON:    `{}`,
			MetadataJSON:       `{}`,
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvestmentInstrumentExists))
}

func createThreeLots(t *testing.T, database *sql.DB, ownerID, currencyID int64) (int64, int64, []InvestmentLotRecord) {
	t.Helper()
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)
	ctx := context.Background()
	lotData := []struct {
		date  string
		qty   int64
		basis int64
	}{
		{"2026-01-01", 100, 10000},
		{"2026-02-01", 200, 25000},
		{"2026-03-01", 150, 21000},
	}
	var lots []InvestmentLotRecord
	for i, d := range lotData {
		lot, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
			BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
			OpenedOn: d.date, QuantityValue: exact.New(d.qty), QuantityScale: 0,
			CostBasisValue: d.basis, CostBasisScale: 2, CostCommodityID: currencyID,
			MetadataJSON: `{}`, CreatedAt: d.date + "T09:00:00Z", CreatedByUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.acquire",
			ChangeReason: "test lot " + strconv.Itoa(i+1), EventKind: "acquisition",
		})
		require.NoError(t, err)
		lots = append(lots, lot)
	}
	return accountID, instrument.CommodityID, lots
}

func TestInvestmentLotsDisposeLIFO(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, lots := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Sell 150 units LIFO: newest lot first (lot[2]=150, qty exactly matches)
	disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(150), QuantityScale: 0,
		CostBasisMethod: "lifo", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "lifo sell",
	})
	require.NoError(t, err)
	require.Len(t, disposals, 1)
	// Should have disposed the newest lot (lot[2])
	assert.Equal(t, lots[2].ID, disposals[0].LotID)
	assert.Equal(t, exact.New(150), disposals[0].QuantityValue)
	assert.Equal(t, int64(21000), disposals[0].CostBasisValue)

	remaining, err := repo.ListLots(ctx, 1, accountID, commodityID)
	require.NoError(t, err)
	var open []InvestmentLotRecord
	for _, l := range remaining {
		if l.Status == "open" {
			open = append(open, l)
		}
	}
	require.Len(t, open, 2)
	assert.Equal(t, lots[0].ID, open[0].ID)
	assert.Equal(t, lots[1].ID, open[1].ID)
}

func TestInvestmentLotsDisposeLIFOCrossesLots(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, lots := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Sell 200 units LIFO: 150 from lot[2], then 50 from lot[1]
	disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(200), QuantityScale: 0,
		CostBasisMethod: "lifo", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "lifo cross lots",
	})
	require.NoError(t, err)
	require.Len(t, disposals, 2)
	assert.Equal(t, lots[2].ID, disposals[0].LotID)
	assert.Equal(t, exact.New(150), disposals[0].QuantityValue)
	assert.Equal(t, lots[1].ID, disposals[1].LotID)
	assert.Equal(t, exact.New(50), disposals[1].QuantityValue)

	// FIFO over same lots would give different first lot
	remaining, _ := repo.ListLots(ctx, 1, accountID, commodityID)
	var open []InvestmentLotRecord
	for _, l := range remaining {
		if l.Status == "open" {
			open = append(open, l)
		}
	}
	require.Len(t, open, 2)
	// lot[0] (oldest) should still be fully open
	assert.Equal(t, lots[0].ID, open[0].ID)
	assert.Equal(t, exact.New(100), open[0].RemainingQuantityValue)
}

func TestInvestmentLotsLIFOAndFIFOProduceDifferentBasis(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Simulate FIFO for 100 units
	fifoDisposals, err := repo.SimulateDisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisMethod: "fifo",
	})
	require.NoError(t, err)
	var fifoBasis int64
	for _, d := range fifoDisposals {
		fifoBasis += d.CostBasisValue
	}

	// Simulate LIFO for 100 units (lots stay unchanged because SimulateDisposeLots rolls back)
	lifoDisposals, err := repo.SimulateDisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisMethod: "lifo",
	})
	require.NoError(t, err)
	var lifoBasis int64
	for _, d := range lifoDisposals {
		lifoBasis += d.CostBasisValue
	}

	assert.NotEqual(t, fifoBasis, lifoBasis, "FIFO and LIFO should produce different cost basis")
}

func TestInvestmentLotsAverageCostPoolMath(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// pool: qty=450, basis=56000. sell qty=100 (= entire lot[0] of 100 qty).
	// lot[0] closes fully -> disposed basis = lot[0].costBasis = 10000.
	// per-lot-own-rate: dispose = lot.costBasis * take / lot.qty (truncate); full close takes entire lot cost.
	disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisMethod: "average_cost", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "avg cost sell",
	})
	require.NoError(t, err)
	var totalDisposedBasis int64
	for _, d := range disposals {
		totalDisposedBasis += d.CostBasisValue
	}
	assert.Equal(t, int64(10000), totalDisposedBasis)

	// Conservation: remaining basis + disposed = 56000
	lots, err := repo.ListLots(ctx, 1, accountID, commodityID)
	require.NoError(t, err)
	var remainingBasis int64
	for _, l := range lots {
		remainingBasis += l.RemainingCostBasisValue
	}
	assert.Equal(t, int64(56000), remainingBasis+totalDisposedBasis)
}

func TestInvestmentLotsAverageCostResidualConservation(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// 3 lots of 1 unit each, total basis=10 (minor units) → average_cost sale of 2 units
	// disposed_basis = 10 * 2 / 3 = 6 (truncate); residual = 0 (6 is exact... let's use 3 lots, basis 10)
	// Actually: 10*2/3 = 6 (truncate). Per-lot: lot1 takes 1 unit → 10*1/2=5 (wait)
	// Use: 3 lots of 1 qty, basis=[4,3,3]=10. Sell 2 units avg cost.
	// disposed_total = 10*2/3 = 6 (truncate). lot1: 6*1/2=3, lot2 (last): 6-3=3.
	// Remaining: lot3 has remaining_basis = 4 (lot3 untouched).
	// Total remaining = original - disposed = 10 - 6 = 4.
	for i, basis := range []int64{4, 3, 3} {
		_, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
			BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
			OpenedOn: "2026-0" + strconv.Itoa(i+1) + "-01",
			QuantityValue: exact.New(1), QuantityScale: 0,
			CostBasisValue: basis, CostBasisScale: 2, CostCommodityID: currencyID,
			MetadataJSON: `{}`, CreatedAt: "2026-0" + strconv.Itoa(i+1) + "-01T09:00:00Z",
			CreatedByUserID: ownerID, OriginType: "browser_api",
			Operation: "investment.lot.acquire", ChangeReason: "residual test",
			EventKind: "acquisition",
		})
		require.NoError(t, err)
	}

	disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(2), QuantityScale: 0,
		CostBasisMethod: "average_cost", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "residual conservation test",
	})
	require.NoError(t, err)
	var totalDisposed int64
	for _, d := range disposals {
		totalDisposed += d.CostBasisValue
		assert.GreaterOrEqual(t, d.CostBasisValue, int64(0), "no lot should have negative disposed basis")
	}

	lots, err := repo.ListLots(ctx, 1, accountID, instrument.CommodityID)
	require.NoError(t, err)
	var remainingBasis int64
	for _, l := range lots {
		assert.GreaterOrEqual(t, l.RemainingCostBasisValue, int64(0), "no lot should have negative remaining basis")
		remainingBasis += l.RemainingCostBasisValue
	}
	// Pool conservation: remaining + disposed == original total (10)
	assert.Equal(t, int64(10), remainingBasis+totalDisposed)
}

func TestInvestmentLotsAverageCostClosedLotHasZeroBasis(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	_, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		OpenedOn: "2026-01-01", QuantityValue: exact.New(10), QuantityScale: 0,
		CostBasisValue: 5000, CostBasisScale: 2, CostCommodityID: currencyID,
		MetadataJSON: `{}`, CreatedAt: "2026-01-01T09:00:00Z", CreatedByUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.acquire",
		ChangeReason: "close basis test", EventKind: "acquisition",
	})
	require.NoError(t, err)

	// Sell all 10 units — lot must close with zero remaining basis
	_, err = repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(10), QuantityScale: 0,
		CostBasisMethod: "average_cost", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "close basis",
	})
	require.NoError(t, err)

	lots, err := repo.ListLots(ctx, 1, accountID, instrument.CommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	assert.Equal(t, "closed", lots[0].Status)
	assert.Equal(t, int64(0), lots[0].RemainingCostBasisValue)
}

func TestInvestmentLotsAverageCostMismatchedScaleReturnsError(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Create two lots with different quantity scales (normally prevented by UI, but we test directly)
	_, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		OpenedOn: "2026-01-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisValue: 10000, CostBasisScale: 2, CostCommodityID: currencyID,
		MetadataJSON: `{}`, CreatedAt: "2026-01-01T09:00:00Z", CreatedByUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.acquire",
		ChangeReason: "scale mismatch lot 1", EventKind: "acquisition",
	})
	require.NoError(t, err)
	// Manually insert a lot with scale=2 to simulate divergence
	_, err = database.ExecContext(ctx, `
		INSERT INTO investment_lots (
			book_id, account_id, commodity_id, cost_commodity_id, opened_on,
			quantity_value, quantity_scale, remaining_quantity_value, remaining_quantity_scale,
			cost_basis_value, cost_basis_scale, remaining_cost_basis_value, remaining_cost_basis_scale,
			status, metadata_json, created_at, created_by_user_id, updated_at, updated_by_user_id
		) VALUES (1, ?, ?, ?, '2026-02-01', 50, 2, 50, 2, 5000, 2, 5000, 2, 'open', '{}', '2026-02-01T09:00:00Z', ?, '2026-02-01T09:00:00Z', ?)
	`, accountID, instrument.CommodityID, currencyID, ownerID, ownerID)
	require.NoError(t, err)

	_, err = repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(50), QuantityScale: 0,
		CostBasisMethod: "average_cost", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "scale mismatch test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scale")
}

func TestInvestmentLotsSpecificLotValidatesOwnershipAndQuantity(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, lots := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Over-allocate a single lot
	_, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(999), QuantityScale: 0,
		CostBasisMethod: "specific_lot",
		Allocations: []LotAllocation{{LotID: lots[0].ID, QuantityValue: exact.New(999), QuantityScale: 0}},
		CreatedAt: "2026-04-01T09:00:00Z", ActorUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "oversell specific",
	})
	require.ErrorIs(t, err, ErrInsufficientLots)
}

func TestInvestmentLotsExplicitAllocationsRejectedForFIFO(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, lots := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	_, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(50), QuantityScale: 0,
		CostBasisMethod: "fifo",
		Allocations: []LotAllocation{{LotID: lots[0].ID, QuantityValue: exact.New(50), QuantityScale: 0}},
		CreatedAt: "2026-04-01T09:00:00Z", ActorUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "fifo explicit reject",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specific_lot")
}

func TestInvestmentLotsUnknownMethodReturnsError(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	_, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(10), QuantityScale: 0,
		CostBasisMethod: "mystery_method", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "unknown method",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestInvestmentLotsSimulateDoesNotMutateLots(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	before, err := repo.ListLots(ctx, 1, accountID, commodityID)
	require.NoError(t, err)

	_, err = repo.SimulateDisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(200), QuantityScale: 0,
		CostBasisMethod: "fifo",
	})
	require.NoError(t, err)

	after, err := repo.ListLots(ctx, 1, accountID, commodityID)
	require.NoError(t, err)
	require.Equal(t, len(before), len(after))
	for i := range before {
		assert.Equal(t, before[i].RemainingQuantityValue, after[i].RemainingQuantityValue)
		assert.Equal(t, before[i].RemainingCostBasisValue, after[i].RemainingCostBasisValue)
		assert.Equal(t, before[i].Status, after[i].Status)
	}
}
