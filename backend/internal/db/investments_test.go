package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

	// pool: qty=450, basis=56000. sell qty=100 units (entire lot[0]).
	// pool-rate reported disposal = 56000*100/450 = 12444 (truncate).
	// lot[0] is the only lot touched and is "last" (fills entire sell qty) → reportedBasis = 12444.
	// DB deduction for lot[0] = 10000 (full lot close).
	// Remaining DB basis = 25000 + 21000 = 46000.
	// DB conservation: 46000 + 10000 (deducted) = 56000 ✓
	// Reported conservation: 12444 ≠ remaining DB — this is the blending redistribution effect.
	disposals, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisMethod: "average_cost", CreatedAt: "2026-04-01T09:00:00Z",
		ActorUserID: ownerID, OriginType: "browser_api",
		Operation: "investment.lot.dispose", ChangeReason: "avg cost sell",
	})
	require.NoError(t, err)
	require.Len(t, disposals, 1)
	// Reported basis is pool-rate: 56000 * 100 / 450 = 12444 (truncate).
	assert.Equal(t, int64(12444), disposals[0].CostBasisValue)

	// DB remaining basis: lot[0] closed (deducted 10000), lots[1]+[2] untouched.
	lots, err := repo.ListLots(ctx, 1, accountID, commodityID)
	require.NoError(t, err)
	var remainingBasis int64
	for _, l := range lots {
		remainingBasis += l.RemainingCostBasisValue
	}
	assert.Equal(t, int64(46000), remainingBasis)
}

func TestInvestmentLotsAverageCostResidualConservation(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID := createInvestmentTestAccount(t, database, currencyID)
	instrument := createInvestmentTestInstrument(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// 3 lots: qty=[1,1,1], basis=[4,3,3] → pool qty=3, basis=10.
	// Sell 2 units average_cost.
	// Pool-rate reported disposal: 10*2/3 = 6 (truncate).
	//   lot[0]: reportedBasis = 10*1/3 = 3; lotDeduction = 4 (full close).
	//   lot[1]: reportedBasis = residual = 6-3 = 3; lotDeduction = 3 (full close).
	// DB remaining: lot[2].remainingCostBasis = 3.
	// DB conservation: deducted(4+3) + remaining(3) = 10 ✓
	// Reported disposal total: 6 (pool rate).
	for i, basis := range []int64{4, 3, 3} {
		_, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
			BookID: 1, AccountID: accountID, CommodityID: instrument.CommodityID,
			OpenedOn:        fmt.Sprintf("2026-%02d-01", i+1),
			QuantityValue:   exact.New(1), QuantityScale: 0,
			CostBasisValue:  basis, CostBasisScale: 2, CostCommodityID: currencyID,
			MetadataJSON: `{}`, CreatedAt: fmt.Sprintf("2026-%02d-01T09:00:00Z", i+1),
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
	require.Len(t, disposals, 2)
	// Reported basis uses pool rate: total must equal disposedBasisTotal = 6.
	var totalReported int64
	for _, d := range disposals {
		assert.GreaterOrEqual(t, d.CostBasisValue, int64(0), "no lot should report negative disposed basis")
		totalReported += d.CostBasisValue
	}
	assert.Equal(t, int64(6), totalReported, "sum of reported disposal basis should equal pool-rate total")

	lots, err := repo.ListLots(ctx, 1, accountID, instrument.CommodityID)
	require.NoError(t, err)
	var remainingBasis int64
	for _, l := range lots {
		assert.GreaterOrEqual(t, l.RemainingCostBasisValue, int64(0), "no lot should have negative remaining basis")
		remainingBasis += l.RemainingCostBasisValue
	}
	// DB remaining should equal lot[2]'s untouched basis.
	assert.Equal(t, int64(3), remainingBasis)
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
	require.ErrorIs(t, err, ErrInvalidDisposalParams)
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
	require.ErrorIs(t, err, ErrInvalidDisposalParams)
}

func TestInvestmentLotsSpecificLotAllocationTotalMismatchReturnsError(t *testing.T) {
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, lots := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Allocate 50 but claim to sell 100 — totals must match.
	_, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		EventDate: "2026-04-01", QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisMethod: "specific_lot",
		Allocations:     []LotAllocation{{LotID: lots[0].ID, QuantityValue: exact.New(50), QuantityScale: 0}},
		CreatedAt: "2026-04-01T09:00:00Z", ActorUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "total mismatch",
	})
	require.ErrorIs(t, err, ErrInvalidDisposalParams)
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
	require.ErrorIs(t, err, ErrInvalidDisposalParams)
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

func TestListRealizedGainsNilProceedsWhenNoTransaction(t *testing.T) {
	// When lots are disposed without an associated transaction (TransactionID=0),
	// proceeds are unknown → ProceedsValue=0 and RealizedGainValue = −disposed_basis.
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Dispose 100 units (cost basis 10000 at scale 2 = 100.00 EUR) with no transaction.
	_, err := repo.DisposeLots(ctx, DisposeLotsParams{
		BookID: 1, AccountID: accountID, CommodityID: commodityID,
		TransactionID: 0, EventDate: "2026-04-01",
		QuantityValue: exact.New(100), QuantityScale: 0,
		CostBasisMethod: "fifo",
		CreatedAt: "2026-04-01T10:00:00Z", ActorUserID: ownerID,
		OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "test",
	})
	require.NoError(t, err)

	gains, err := repo.ListRealizedGains(ctx, 1, RealizedGainsParams{})
	require.NoError(t, err)
	require.Len(t, gains, 1)

	g := gains[0]
	assert.Equal(t, accountID, g.AccountID)
	assert.Equal(t, commodityID, g.CommodityID)
	assert.Equal(t, "2026-04-01", g.DisposalDate)
	assert.Equal(t, int64(0), g.ProceedsValue)
	// Disposal events store cost_basis_value as negative (the basis deducted from the lot).
	assert.Equal(t, int64(-10000), g.DisposedBasisValue)
	// Gain = proceeds + disposed_basis = 0 + (−10000) = −10000 (a loss of €100.00).
	assert.Equal(t, int64(-10000), g.RealizedGainValue)
}

func TestListRealizedGainsDateFilter(t *testing.T) {
	// Two disposals on different dates; verify From/To filter includes/excludes correctly.
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	for _, date := range []string{"2026-04-01", "2026-05-01"} {
		_, err := repo.DisposeLots(ctx, DisposeLotsParams{
			BookID: 1, AccountID: accountID, CommodityID: commodityID,
			TransactionID: 0, EventDate: date,
			QuantityValue: exact.New(10), QuantityScale: 0,
			CostBasisMethod: "fifo",
			CreatedAt: date + "T10:00:00Z", ActorUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: date,
		})
		require.NoError(t, err)
	}

	// Both included when no filter.
	all, err := repo.ListRealizedGains(ctx, 1, RealizedGainsParams{})
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// Only the April disposal is within From/To.
	filtered, err := repo.ListRealizedGains(ctx, 1, RealizedGainsParams{
		From: "2026-04-01",
		To:   "2026-04-30",
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "2026-04-01", filtered[0].DisposalDate)
}

func TestListRealizedGainsMultiLotSaleProducesOneRow(t *testing.T) {
	// A sell transaction that disposes two lots must produce one realized-gains row,
	// not two. The full sale proceeds appear once on the transaction; the old per-lot
	// query would have duplicated them, inflating gains.
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	repo := NewInvestmentRepository(database)

	// Insert a bare transaction row to satisfy the FK constraint.
	result, err := database.ExecContext(ctx, `
		INSERT INTO transactions (book_id, created_at, created_by_user_id)
		VALUES (1, '2026-04-01T10:00:00Z', ?)`, ownerID)
	require.NoError(t, err)
	txID, err := result.LastInsertId()
	require.NoError(t, err)

	// Dispose two separate lots of 50 units each, both referencing the same transaction.
	for _, qty := range []int64{50, 50} {
		_, err := repo.DisposeLots(ctx, DisposeLotsParams{
			BookID: 1, AccountID: accountID, CommodityID: commodityID,
			TransactionID: txID, EventDate: "2026-04-01",
			QuantityValue: exact.New(qty), QuantityScale: 0,
			CostBasisMethod: "fifo",
			CreatedAt: "2026-04-01T10:00:00Z", ActorUserID: ownerID,
			OriginType: "browser_api", Operation: "investment.lot.dispose", ChangeReason: "multi-lot sell",
		})
		require.NoError(t, err)
	}

	gains, err := repo.ListRealizedGains(ctx, 1, RealizedGainsParams{})
	require.NoError(t, err)

	// The two disposals share one transaction_id → they must aggregate into one row.
	require.Len(t, gains, 1, "expected one gains row for a multi-lot sell transaction")

	g := gains[0]
	// Sold qty = 50 + 50 = 100 (positive, negated back).
	assert.Equal(t, exact.New(100), g.QuantityValue)
	// disposed_basis: first 50 from lot-1 (basis 10000 × 50/100 = 5000, negative) +
	//                 next 50 from lot-1 remainder (basis 5000, negative) = −10000.
	assert.Equal(t, int64(-10000), g.DisposedBasisValue)
}

func TestPositionsWithGainsNilWhenNoPrice(t *testing.T) {
	// A position without a price observation must have nil gain fields.
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	_ = accountID
	repo := NewInvestmentRepository(database)

	gains, err := repo.PositionsWithGains(ctx, 1)
	require.NoError(t, err)
	require.Len(t, gains, 1)
	assert.Equal(t, commodityID, gains[0].CommodityID)
	assert.Nil(t, gains[0].MarketValueValue, "expected nil market value when no price")
	assert.Nil(t, gains[0].UnrealizedGainValue, "expected nil gain when no price")
}

func TestPositionsWithGainsBaseQuantityNormalized(t *testing.T) {
	// Price stored as 150 EUR at base_quantity=100, base_quantity_scale=0.
	// Per-unit price = 150/100 = €1.50.
	// Position: 450 units at cost 56 000 (scale 2) = €560.00.
	// Market = 450 × (150/100) = 675 at scale 2 = €675.00.
	// Gain = 675 00 − 56 000 = ... but scale needs to match.
	// market_scale = qty_scale(0) + price_scale(2) − base_qty_scale(0) = 2
	// market_numerator = 450 × 150 = 67 500; denominator = 100 → market = 675 at scale 2.
	// cost = 56 000 at scale 2; gain = 675 − 560 = ... wait, 675 - 56000 is at scale 2:
	// market = 67500/100 = 675 at scale 2 = €6.75 (NOT €675).
	// That's the actual per-unit calc: per-unit price €1.50 × 450 units = €675, but
	// at scale 2 that's 67500 minor units. Let me re-derive with integer math:
	// market_numerator = 450 × 150 = 67500; divide by 100 = 675 (integer).
	// So market = 675 at scale 2 = €6.75? No — 675 / 10^2 = 6.75 EUR. That's wrong.
	//
	// Let me re-read the scale logic: market_scale = qty_scale + price_scale - base_qty_scale.
	// qty=450 (scale 0), price=150 (scale 2), base_qty=100 (scale 0):
	// market_scale = 0 + 2 - 0 = 2.
	// market = 450 × 150 / 100 = 675 at scale 2 → 675 / 10^2 = €6.75.
	//
	// But the real economic answer: 450 units × (150 EUR / 100 shares) = 450 × 1.50 = €675.
	// The mismatch: price_value=150 at price_scale=2 means price = 150 / 10^2 = €1.50.
	// base_quantity=100 at base_qty_scale=0 means per 100 shares.
	// per-unit price = (150 / 10^2) / (100 / 10^0) = (1.50 EUR) / 100 = 0.015 EUR/share.
	// market = 450 × 0.015 = €6.75 = 675 at scale 2. ✓
	//
	// So for a simpler test: price=200 at scale 2, base_qty=1 scale 0 (per-unit, same as before).
	// This test uses base_qty=100 and verifies the division happens.
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	_ = accountID
	repo := NewInvestmentRepository(database)
	pricingRepo := NewPricingRepository(database)

	manualSourceID, err := pricingRepo.ManualSourceID(ctx)
	require.NoError(t, err)

	// Price: 20000 (scale 4) per 100 shares (base_qty=100, base_qty_scale=0).
	// Per-unit price = 20000 / 10^4 / (100 / 10^0) = 2.0000 / 100 = €0.02/share.
	// market_scale = 0 + 4 - 0 = 4.
	// market_numerator = 450 × 20000 = 9_000_000; denominator = 100 → market = 90_000 at scale 4.
	// 90000 / 10^4 = €9.00.
	// cost = 56000 at scale 2; aligned to scale 4: 56000 × 100 = 5_600_000.
	// gain = 90000 − 5_600_000 = −5_510_000 at scale 4.
	_, err = pricingRepo.CreatePriceObservation(ctx, CreatePriceObservationParams{
		BookID: 1, ActorUserID: ownerID, RequestID: "request-price-base-qty",
		OriginType: "browser_api", Operation: "pricing.price.create",
		CreatedAt: "2026-04-01T12:00:00Z", ChangeReason: "test base qty",
		Spec: PriceObservationSpec{
			BaseCommodityID:    commodityID,
			QuoteCommodityID:   currencyID,
			QuoteType:          "manual",
			AdjustmentBasis:    "raw",
			PriceValue:         20000,
			PriceScale:         4,
			BaseQuantityValue:  100,
			BaseQuantityScale:  0,
			ValuationDate:      "2026-04-01",
			ObservedAt:         sql.NullString{String: "2026-04-01T12:00:00Z", Valid: true},
			SourceID:           sql.NullInt64{Int64: manualSourceID, Valid: true},
			IsManual:           true,
			DerivationJSON:     `{}`,
			SeriesMetadataJSON: `{}`,
			MetadataJSON:       `{}`,
		},
	})
	require.NoError(t, err)

	gains, err := repo.PositionsWithGains(ctx, 1)
	require.NoError(t, err)
	require.Len(t, gains, 1)

	g := gains[0]
	require.NotNil(t, g.MarketValueValue)
	// market = 450 × 20000 / 100 = 90000 at scale 4.
	assert.Equal(t, int64(90000), *g.MarketValueValue)
	assert.Equal(t, 4, *g.MarketValueScale)
}

func TestPositionsWithGainsComputedCorrectly(t *testing.T) {
	// 100 units at cost 10 000 minor units (scale 2) = €100.00.
	// Price observation: 200 minor units (scale 2) = €2.00 per unit → market €200.00.
	// Expected unrealized gain: €200.00 − €100.00 = €100.00 at the combined scale.
	ctx := context.Background()
	database, ownerID, currencyID := migratedInvestmentTestDatabase(t)
	accountID, commodityID, _ := createThreeLots(t, database, ownerID, currencyID)
	_ = accountID
	repo := NewInvestmentRepository(database)
	pricingRepo := NewPricingRepository(database)

	manualSourceID, err := pricingRepo.ManualSourceID(ctx)
	require.NoError(t, err)

	// Record a price observation: 200 minor units at scale 2 = €2.00 per unit.
	_, err = pricingRepo.CreatePriceObservation(ctx, CreatePriceObservationParams{
		BookID: 1, ActorUserID: ownerID, RequestID: "request-price",
		OriginType: "browser_api", Operation: "pricing.price.create",
		CreatedAt: "2026-04-01T12:00:00Z", ChangeReason: "test price",
		Spec: PriceObservationSpec{
			BaseCommodityID:    commodityID,
			QuoteCommodityID:   currencyID,
			QuoteType:          "manual",
			AdjustmentBasis:    "raw",
			PriceValue:         200,
			PriceScale:         2,
			BaseQuantityValue:  1,
			BaseQuantityScale:  0,
			ValuationDate:      "2026-04-01",
			ObservedAt:         sql.NullString{String: "2026-04-01T12:00:00Z", Valid: true},
			SourceID:           sql.NullInt64{Int64: manualSourceID, Valid: true},
			IsManual:           true,
			DerivationJSON:     `{}`,
			SeriesMetadataJSON: `{}`,
			MetadataJSON:       `{}`,
		},
	})
	require.NoError(t, err)

	gains, err := repo.PositionsWithGains(ctx, 1)
	require.NoError(t, err)
	require.Len(t, gains, 1)

	g := gains[0]
	require.NotNil(t, g.MarketValueValue, "expected market value to be set when price exists")
	require.NotNil(t, g.UnrealizedGainValue)

	// quantity_value = 100+200+150 = 450 at scale 0; price = 200 at scale 2.
	// market = 450 × 200 = 90 000 at scale 0+2=2 → 90 000 / 100 = €900.00.
	// cost = 10000+25000+21000 = 56 000 at scale 2.
	// gain = 90 000 − 56 000 = 34 000 at scale 2.
	assert.Equal(t, int64(90000), *g.MarketValueValue)
	assert.Equal(t, int64(34000), *g.UnrealizedGainValue)
	assert.Equal(t, 2, *g.UnrealizedGainScale)
}
