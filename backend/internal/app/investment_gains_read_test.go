package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/testdb"
)

func TestGainsSnapshotsUseTheOwnerDayForCapitalReturnLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReferenceVersionFixture(t)
	setOwnerTimeZone(t, f.database, "Europe/Amsterdam")
	useServiceCreatedHoldingAccount(t, f.investmentsTestFixture)
	buyOn(t, f.investmentsTestFixture, "2026-05-01", 2, 4000)
	_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f.investmentsTestFixture, "2026-06-01", 1000))
	require.NoError(t, err)

	// Keep the rename ahead of SQLite's real UTC date so an unpinned read
	// cannot see it, even if UTC midnight passes during the test.
	today := testdb.RealUTCToday()
	cutoff := testdb.UTCDayInstant(today, 7, "23:30:00")
	captured, err := time.Parse(time.RFC3339, cutoff)
	require.NoError(t, err)
	utcDay, ownerDay := testdb.UTCDay(today, 7), testdb.UTCDay(today, 8)
	f.accountService.SetNowForTest(func() time.Time { return captured })
	f.investmentService.SetNowForTest(func() time.Time { return captured })
	const newName = "Renamed holding"
	updateAccountVersion(t, f, f.holdingAccountID, newName, ownerDay)

	check := func(t *testing.T, wantDate string, profile GainsProfile, returns []CapitalReturnRead) {
		t.Helper()
		assert.Equal(t, wantDate, profile.ValuationAsOf)
		assert.Equal(t, cutoff, profile.KnowledgeCutoff)
		require.Len(t, returns, 1)
		assert.Equal(t, newName, returns[0].HoldingName)
	}

	t.Run("live gains", func(t *testing.T) {
		read, err := f.investmentService.ReadGains(ctx, GainsReadInput{})
		require.NoError(t, err)
		check(t, ownerDay, read.Profile, read.CapitalReturns)
	})
	t.Run("live statement", func(t *testing.T) {
		read, err := f.investmentService.ReadGainsStatement(ctx, "2026-06-01", utcDay, nil)
		require.NoError(t, err)
		check(t, utcDay, read.Statement.Profile, read.Statement.CapitalReturns)
	})
	savedCases := []struct {
		request  GainsRunRequest
		wantDate string
	}{
		{request: GainsRunRequest{ReportType: "gains"}, wantDate: ownerDay},
		{request: GainsRunRequest{ReportType: "statement", From: "2026-06-01", To: utcDay}, wantDate: utcDay},
	}
	for _, tc := range savedCases {
		t.Run("saved "+tc.request.ReportType, func(t *testing.T) {
			type document struct {
				Profile        GainsProfile        `json:"profile"`
				CapitalReturns []CapitalReturnRead `json:"capital_returns"`
			}
			run, err := f.investmentService.CreateGainsRun(ctx, GainsRunInput{
				OwnerUserID: f.ownerUserID,
				Request:     tc.request,
			}, func(p GainsRunProjection) (json.RawMessage, error) {
				returns := p.CapitalReturns
				if tc.request.ReportType == "statement" {
					require.NotNil(t, p.Statement)
					returns = p.Statement.CapitalReturns
				}
				return json.Marshal(document{Profile: p.Profile, CapitalReturns: returns})
			})
			require.NoError(t, err)
			saved, err := f.investmentService.GainsRun(ctx, run.ID)
			require.NoError(t, err)
			var output document
			require.NoError(t, json.Unmarshal(saved.OutputJSON, &output))
			check(t, tc.wantDate, output.Profile, output.CapitalReturns)
		})
	}
}

func TestLiveGainsCorrectionBetweenOperationalAndFXKeepsSameCurrencyGain(t *testing.T) {
	t.Parallel()
	for _, method := range []string{GainsFXValuationDateAll, GainsFXHistoricBasis} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			gainsFXCurrency(t, f)
			buy := buyOn(t, f, "2026-01-01", 2, 4000)
			priceStockOn(t, f, "2026-03-31", 3000)
			var afterCorrection map[string][][]any
			f.investmentService.SetGainsReadStageForTest(func(stage string) {
				if stage != "operational" {
					return
				}
				_, err := acknowledgedReplaceBuy(context.Background(), f.investmentService, ReplaceInvestmentBuyInput{
					OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "correct acquisition cost",
					Replacement: InvestmentTradeInput{TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
						HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
						QuantityValue: exact.New(2), CashAmountValue: 8000, CashAmountScale: 2},
				})
				require.NoError(t, err)
				afterCorrection = buyReplacementPreviewSnapshot(t, f.database)
			})
			got, err := f.investmentService.ReadGains(context.Background(), GainsReadInput{
				ValuationAsOf: "2026-03-31", AlternativeMethod: "person_pooled_average",
				FX: &GainsFXInput{CommodityID: f.eurCommodityID, Method: method},
			})
			require.NoError(t, err)
			require.NotNil(t, afterCorrection)
			require.Len(t, got.Unrealized, 1)
			assertMoneyValue(t, 2000, 2, *got.Unrealized[0].UnrealizedGainValue, *got.Unrealized[0].UnrealizedGainScale, "original EUR +20")
			require.NotNil(t, got.Reporting)
			assertFXAmount(t, 2000, 2, got.Reporting.UnrealizedTotal)
			require.NotNil(t, got.Alternative)
			require.Len(t, got.Alternative.Unrealized, 1)
			assertMoneyValue(t, 2000, 2, *got.Alternative.Unrealized[0].UnrealizedGainValue, *got.Alternative.Unrealized[0].UnrealizedGainScale, "comparison uses original acquisition")
			assert.Equal(t, afterCorrection, buyReplacementPreviewSnapshot(t, f.database), "read adds no writes beyond the deliberate correction")
			f.investmentService.SetGainsReadStageForTest(nil)
			next, err := f.investmentService.ReadGains(context.Background(), GainsReadInput{ValuationAsOf: "2026-03-31"})
			require.NoError(t, err)
			assertMoneyValue(t, -2000, 2, *next.Unrealized[0].UnrealizedGainValue, *next.Unrealized[0].UnrealizedGainScale, "next request sees corrected history")
		})
	}
}

func TestLiveGainsObservationWithdrawalsBetweenStagesKeepQuoteAndFXEvidence(t *testing.T) {
	t.Parallel()
	for _, method := range []string{GainsFXValuationDateAll, GainsFXHistoricBasis} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			usd := gainsFXCurrency(t, f)
			buyOn(t, f, "2026-01-01", 2, 4000)
			priceStockOn(t, f, "2026-03-31", 3000)
			gainsFXRate(t, f, usd, "2026-01-01", 50)
			gainsFXRate(t, f, usd, "2026-03-31", 120)
			var afterWithdrawal map[string][][]any
			f.investmentService.SetGainsReadStageForTest(func(stage string) {
				if stage != "operational" {
					return
				}
				withdrawGainsObservations(t, f)
				afterWithdrawal = buyReplacementPreviewSnapshot(t, f.database)
			})
			got, err := f.investmentService.ReadGains(context.Background(), GainsReadInput{ValuationAsOf: "2026-03-31",
				AlternativeMethod: "person_pooled_average", FX: &GainsFXInput{CommodityID: usd, Method: method}})
			require.NoError(t, err)
			require.NotNil(t, afterWithdrawal)
			require.Len(t, got.Alternative.Unrealized, 1)
			assertMoneyValue(t, 2000, 2, *got.Alternative.Unrealized[0].UnrealizedGainValue, *got.Alternative.Unrealized[0].UnrealizedGainScale, "withdrawn quote remains in this response")
			want := int64(2400)
			if method == GainsFXHistoricBasis {
				want = 5200
			}
			assertFXAmount(t, want, 2, got.Reporting.UnrealizedTotal)
			for _, component := range got.Reporting.Unrealized[0].Components {
				assert.Positive(t, component.ObservationID, "withdrawn FX evidence is retained")
			}
			assert.Equal(t, afterWithdrawal, buyReplacementPreviewSnapshot(t, f.database))
			f.investmentService.SetGainsReadStageForTest(nil)
			next, err := f.investmentService.ReadGains(context.Background(), GainsReadInput{ValuationAsOf: "2026-03-31", FX: &GainsFXInput{CommodityID: usd, Method: method}})
			require.NoError(t, err)
			assert.Nil(t, next.Unrealized[0].MarketValueValue)
			assert.Nil(t, next.Reporting.UnrealizedTotal.Amount)
		})
	}
}

func withdrawGainsObservations(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	pricing := NewPricingService(db.NewPricingRepository(f.database))
	rows, err := f.database.Query(`SELECT id FROM price_observations WHERE voided_at IS NULL ORDER BY id`)
	require.NoError(t, err)
	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for _, id := range ids {
		_, err := pricing.VoidPrice(context.Background(), VoidPriceInput{OwnerUserID: f.ownerUserID, ObservationID: id, VoidReason: "withdraw during composition"})
		require.NoError(t, err)
	}
}

func TestLiveStatementBoundariesDisposalsIncomeAndFXShareEffectiveHistory(t *testing.T) {
	t.Parallel()
	for _, method := range []string{GainsFXValuationDateAll, GainsFXHistoricBasis} {
		t.Run(method, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			usd := gainsFXCurrency(t, f)
			buy := buyOn(t, f, "2026-01-01", 2, 4000)
			sale := sellInput(f, "2026-03-01", 1)
			sale.CashAmountValue = 5000
			_, err := f.investmentService.Sell(context.Background(), sale)
			require.NoError(t, err)
			dividend := dividendOn(t, f, "2026-03-15", 300)
			priceStockOn(t, f, "2026-02-28", 2500)
			priceStockOn(t, f, "2026-03-31", 6000)
			for _, date := range []string{"2026-01-01", "2026-02-28", "2026-03-01", "2026-03-15", "2026-03-31"} {
				gainsFXRate(t, f, usd, date, 100)
			}
			var afterWrites map[string][][]any
			f.investmentService.SetGainsReadStageForTest(func(stage string) {
				switch stage {
				case "closing":
					_, err := acknowledgedReplaceBuy(context.Background(), f.investmentService, ReplaceInvestmentBuyInput{
						OwnerUserID: f.ownerUserID, TransactionID: buy.Transaction.ID, Reason: "correct before opening read",
						Replacement: InvestmentTradeInput{TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
							HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
							QuantityValue: exact.New(2), CashAmountValue: 8000, CashAmountScale: 2}})
					require.NoError(t, err)
					_, err = f.investmentService.ReverseDividend(context.Background(), ReverseInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: dividend.ID, Reason: "withdraw before income read"})
					require.NoError(t, err)
				case "statement":
					withdrawGainsObservations(t, f)
					afterWrites = buyReplacementPreviewSnapshot(t, f.database)
				}
			})
			got, err := f.investmentService.ReadGainsStatement(context.Background(), "2026-03-01", "2026-03-31", &GainsFXInput{CommodityID: usd, Method: method})
			require.NoError(t, err)
			require.NotNil(t, afterWrites)
			row := onlyStatementRow(t, f, got.Statement)
			assertStatementAmount(t, 1000, 2, row.Opening.Gain, "original opening")
			assertStatementAmount(t, 4000, 2, row.Closing.Gain, "original closing")
			assertStatementAmount(t, 3000, 2, row.Realized, "original disposal")
			assertStatementAmount(t, 300, 2, &row.Income, "original income")
			assertStatementAmount(t, 6300, 2, row.Total, "original period result")
			assertFXAmount(t, 1000, 2, got.Reporting.Opening)
			assertFXAmount(t, 4000, 2, got.Reporting.Closing)
			assertFXAmount(t, 3000, 2, got.Reporting.Realized)
			assertFXAmount(t, 300, 2, got.Reporting.Income)
			assertFXAmount(t, 6300, 2, got.Reporting.Total)
			assert.Equal(t, afterWrites, buyReplacementPreviewSnapshot(t, f.database))
		})
	}
}

func TestLiveGainsSuccessFailureAndCancellationLeaveFinancialAuditAndRunRowsUnchanged(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 4000)
	before := buyReplacementPreviewSnapshot(t, f.database)
	for _, tc := range []struct {
		name, method string
		wantError    bool
	}{
		{"success", "person_pooled_average", false}, {"failure_after_operational_replay", "invalid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.investmentService.ReadGains(context.Background(), GainsReadInput{ValuationAsOf: "2026-03-31", AlternativeMethod: tc.method})
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.investmentService.SetGainsReadStageForTest(func(stage string) {
		if stage == "operational" {
			cancel()
		}
	})
	_, err := f.investmentService.ReadGains(ctx, GainsReadInput{AlternativeMethod: "person_pooled_average"})
	require.True(t, errors.Is(err, context.Canceled), "%v", err)
	assert.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	var runs int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_gains_runs`).Scan(&runs))
	assert.Zero(t, runs)
}
