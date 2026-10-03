package app

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-117 #132: known-basis transfer-in and sales/write-offs dated behind a
// later disposal are admitted through complete chronological replay. Later
// decisions keep their recorded method, provenance and explicit elections;
// changed gains need the preview's acknowledgement; an impossible later
// decision is named and nothing is written. Backdated reinvestment, which
// already replayed, stays pinned by TestReinvestmentGainImpactRequiresAcknowledgement.

func gainsByDate(t *testing.T, f *investmentsTestFixture) map[string]RealizedGainEntry {
	t.Helper()
	gains, err := f.investmentService.ListRealizedGains(context.Background(), GainsReportParams{})
	require.NoError(t, err)
	byDate := make(map[string]RealizedGainEntry, len(gains))
	for _, gain := range gains {
		require.NotContains(t, byDate, gain.DisposalDate, "one disposal per date in these fixtures")
		byDate[gain.DisposalDate] = gain
	}
	return byDate
}

func backdatedWriteOff(f *investmentsTestFixture, date string, quantity int64) InvestmentWriteOffInput {
	return InvestmentWriteOffInput{OwnerUserID: f.ownerUserID, TransactionDate: date,
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		QuantityValue: exact.New(quantity), Reason: "delisted", ChangeReason: "delisted"}
}

func TestBackdatedTransferInFromOldBrokerReplaysCurrentBrokerSaleByOriginalDate(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	seedExternalTransferEquity(t, f.database)
	// The current broker's history is entered first: a May buy at 40.00 per
	// share and a July FIFO sale of both shares for 100.00 (gain 20.00).
	buyOn(t, f, "2026-05-01", 2, 8000)
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	// Then the old broker's shares arrive, dated March but bought in 2020.
	input := knownTransferInput(f)
	input.EffectiveOn = "2026-03-01"
	input.CarriedBasisValue = 3000

	before := buyReplacementPreviewSnapshot(t, f.database)
	impact, err := f.investmentService.PreviewExternalTransferInReconciliationImpact(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
	require.NotNil(t, impact.GainImpact)
	require.Len(t, impact.GainImpact.Changes, 1)
	requireScaled(t, 2000, 2, impact.GainImpact.Changes[0].Before.Gain, "gain before the transfer")
	requireScaled(t, 7000, 2, impact.GainImpact.Changes[0].After.Gain, "FIFO now takes the 2020 lot")

	_, err = f.investmentService.ExternalTransferIn(ctx, input)
	require.ErrorIs(t, err, ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "refusal leaves no partial write")
	input.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	transfer, err := f.investmentService.ExternalTransferIn(ctx, input)
	require.NoError(t, err)
	july := gainsByDate(t, f)["2026-07-01"]
	assertMoneyValue(t, 7000, 2, july.RealizedGainValue, july.RealizedGainScale, "committed revised gain")
	var revisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_disposal_revisions`).Scan(&revisions))
	assert.Equal(t, 1, revisions, "the July decision keeps its original evidence and gains a revision")

	// opened_on still controls availability: a transfer effective after the
	// sale cannot reach back into it, however old its shares are. It is in
	// order, replays nothing and needs no acknowledgement — but its original
	// date still puts it first for the next FIFO sale.
	later := knownTransferInput(f)
	later.EffectiveOn = "2026-08-01"
	later.OriginalAcquiredOn = "2019-01-01"
	later.CarriedBasisValue = 1000
	_, err = f.investmentService.ExternalTransferIn(ctx, later)
	require.NoError(t, err)
	july = gainsByDate(t, f)["2026-07-01"]
	assertMoneyValue(t, 7000, 2, july.RealizedGainValue, july.RealizedGainScale, "July unchanged")
	september := sellInput(f, "2026-09-01", 2)
	_, err = f.investmentService.Sell(ctx, september)
	require.NoError(t, err)
	sale := gainsByDate(t, f)["2026-09-01"]
	assertMoneyValue(t, -1000, 2, sale.DisposedBasisValue, sale.DisposedBasisScale, "2019 lot sold first")
	require.NotNil(t, transfer.LotID)
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestBackdatedDisposalBehindLaterSaleUnderEveryMethod(t *testing.T) {
	for _, kind := range []string{"sale", "write-off"} {
		for _, test := range []struct {
			method     string
			julyBefore int64 // disposed basis at scale 2
			julyAfter  int64 // at scale 6; equals before when unchanged
		}{
			{method: "fifo", julyBefore: 5000, julyAfter: 50000000},
			{method: "lifo", julyBefore: 15000, julyAfter: 150000000},
			// 5 Jan shares at 10.00 left plus 10 June shares at 30.00 pool to
			// 350.00 over 15; 5 shares carry 116.666666 at scale 6.
			{method: "average_cost", julyBefore: 10000, julyAfter: 116666666},
			{method: "specific_lot", julyBefore: 5000, julyAfter: 50000000},
		} {
			t.Run(kind+"/"+test.method, func(t *testing.T) {
				t.Parallel()
				f := newInvestmentsTestFixture(t)
				ctx := context.Background()
				january := buyOn(t, f, "2026-01-01", 10, 10000)
				buyOn(t, f, "2026-06-01", 10, 30000)
				july := sellInput(f, "2026-07-01", 5)
				july.CostBasisMethod = test.method
				election := []InvestmentLotAllocationInput{{LotID: *january.LotID, QuantityValue: exact.New(5)}}
				if test.method == "specific_lot" {
					july.LotAllocations = election
				}
				_, err := f.investmentService.Sell(ctx, july)
				require.NoError(t, err)
				assertMoneyValue(t, -test.julyBefore, 2, gainsByDate(t, f)["2026-07-01"].DisposedBasisValue,
					gainsByDate(t, f)["2026-07-01"].DisposedBasisScale, "July before")

				var impact ReconciliationImpact
				commit := func(ack string) error {
					if kind == "sale" {
						march := sellInput(f, "2026-03-01", 5)
						march.CostBasisMethod = test.method
						if test.method == "specific_lot" {
							march.LotAllocations = election
						}
						if ack == "preview" {
							impact, err = f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, march)
							require.NoError(t, err)
							preview, err := f.investmentService.PreviewSell(ctx, march)
							require.NoError(t, err)
							assertMoneyValue(t, 5000, 2, preview.RealizedGain, preview.RealizedGainScale, "preview gain on the January lot")
							return nil
						}
						march.GainImpactAcknowledgement = ack
						_, err := f.investmentService.Sell(ctx, march)
						return err
					}
					writeOff := backdatedWriteOff(f, "2026-03-01", 5)
					writeOff.CostBasisMethod = test.method
					if test.method == "specific_lot" {
						writeOff.LotAllocations = election
					}
					if ack == "preview" {
						impact, err = f.investmentService.WriteOffReconciliationImpact(ctx, writeOff)
						require.NoError(t, err)
						_, err = f.investmentService.PreviewWriteOff(ctx, writeOff)
						require.NoError(t, err)
						return nil
					}
					writeOff.GainImpactAcknowledgement = ack
					_, err := f.investmentService.WriteOff(ctx, writeOff)
					return err
				}
				before := buyReplacementPreviewSnapshot(t, f.database)
				require.NoError(t, commit("preview"))
				require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database), "preview writes nothing")
				require.NotNil(t, impact.GainImpact)
				changed := exact.ScaledIntFromInt64(test.julyBefore, 2).Cmp(exact.ScaledIntFromInt64(test.julyAfter, 6)) != 0
				if changed {
					require.Len(t, impact.GainImpact.Changes, 1)
					require.ErrorIs(t, commit(""), ErrGainImpactAcknowledgementRequired)
					require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
				} else {
					require.Empty(t, impact.GainImpact.Changes, "an unchanged later gain needs no acknowledgement")
				}
				require.NoError(t, commit(impact.GainImpact.Acknowledgement))

				gains := gainsByDate(t, f)
				// Only the January lot was held in March.
				assertMoneyValue(t, -5000, 2, gains["2026-03-01"].DisposedBasisValue, gains["2026-03-01"].DisposedBasisScale, "March")
				assertMoneyValue(t, -test.julyAfter, 6, gains["2026-07-01"].DisposedBasisValue, gains["2026-07-01"].DisposedBasisScale, "July after")
				// The July decision's recorded method and provenance are kept.
				var method, tier string
				require.NoError(t, f.database.QueryRow(`SELECT cost_basis_method, resolution_tier
					FROM investment_disposal_decisions WHERE event_date = '2026-07-01'`).Scan(&method, &tier))
				assert.Equal(t, test.method, method)
				assert.Equal(t, "transaction", tier)
				positions, err := f.investmentService.Positions(ctx)
				require.NoError(t, err)
				require.Len(t, positions, 1)
				assert.Equal(t, "10", positions[0].QuantityValue.String())
				assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
			})
		}
	}
}

func TestBackdatedDisposalNamesImpossibleLaterDecisionWithoutWriting(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, f *investmentsTestFixture) (InvestmentTradeInput, InvestmentTradeInput)
	}{
		{name: "quantity", setup: func(t *testing.T, f *investmentsTestFixture) (InvestmentTradeInput, InvestmentTradeInput) {
			buyOn(t, f, "2026-01-01", 10, 10000)
			return sellInput(f, "2026-07-01", 10), sellInput(f, "2026-03-01", 5)
		}},
		{name: "explicit election", setup: func(t *testing.T, f *investmentsTestFixture) (InvestmentTradeInput, InvestmentTradeInput) {
			january := buyOn(t, f, "2026-01-01", 10, 10000)
			buyOn(t, f, "2026-02-01", 10, 30000)
			july := sellInput(f, "2026-07-01", 8)
			july.CostBasisMethod = "specific_lot"
			july.LotAllocations = []InvestmentLotAllocationInput{{LotID: *january.LotID, QuantityValue: exact.New(8)}}
			march := sellInput(f, "2026-03-01", 5)
			march.CostBasisMethod = "specific_lot"
			march.LotAllocations = []InvestmentLotAllocationInput{{LotID: *january.LotID, QuantityValue: exact.New(5)}}
			return july, march
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			july, march := test.setup(t, f)
			committed, err := f.investmentService.Sell(ctx, july)
			require.NoError(t, err)
			before := buyReplacementPreviewSnapshot(t, f.database)
			for _, attempt := range []func() error{
				func() error { _, err := f.investmentService.PreviewSell(ctx, march); return err },
				func() error {
					_, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, march)
					return err
				},
				func() error { _, err := f.investmentService.Sell(ctx, march); return err },
			} {
				err := attempt()
				var dependency InvestmentSaleDependencyError
				require.ErrorAs(t, err, &dependency)
				require.NotNil(t, committed.DisposalDecision.ID)
				assert.Equal(t, *committed.DisposalDecision.ID, dependency.DecisionID, "the July decision is named")
				require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
			}
		})
	}
}

func TestBackdatedSaleTakesSameDaySlotAfterEarlierEntries(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 5, 5000)  // 10.00 per share
	buyOn(t, f, "2026-03-01", 5, 10000) // 20.00 per share
	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-07-01", 2))
	require.NoError(t, err)
	// A March 1 sale of 8 needs the March 1 purchase entered before it: the
	// sale's slot follows the day's earlier entries.
	march := sellInput(f, "2026-03-01", 8)
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, march)
	require.NoError(t, err)
	march.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	_, err = f.investmentService.Sell(ctx, march)
	require.NoError(t, err)
	gains := gainsByDate(t, f)
	assertMoneyValue(t, -11000, 2, gains["2026-03-01"].DisposedBasisValue, gains["2026-03-01"].DisposedBasisScale, "5 Jan + 3 Mar")
	assertMoneyValue(t, -4000, 2, gains["2026-07-01"].DisposedBasisValue, gains["2026-07-01"].DisposedBasisScale, "July takes the last March shares")

	// A purchase entered afterwards on March 1 takes a later slot: the March
	// sale cannot have sold it, and July's FIFO choice is unchanged.
	late := backdatedBuy(f, "2026-03-01", 5, 50000)
	late.GainImpactAcknowledgement = previewBuyGainImpact(t, f, late).Acknowledgement
	_, err = f.investmentService.Buy(ctx, late)
	require.NoError(t, err)
	gains = gainsByDate(t, f)
	assertMoneyValue(t, -11000, 2, gains["2026-03-01"].DisposedBasisValue, gains["2026-03-01"].DisposedBasisScale, "March unchanged")
	assertMoneyValue(t, -4000, 2, gains["2026-07-01"].DisposedBasisValue, gains["2026-07-01"].DisposedBasisScale, "July unchanged")
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
}

func TestBackdatedSaleLateRefusalsRollBackReplayAndSourceIdentity(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)
	buyOn(t, f, "2026-06-01", 10, 30000)
	july := sellInput(f, "2026-07-01", 5)
	july.CostBasisMethod = "average_cost"
	_, err := f.investmentService.Sell(ctx, july)
	require.NoError(t, err)
	march := sellInput(f, "2026-03-01", 5)
	march.CostBasisMethod = "average_cost"
	impact, err := f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, march)
	require.NoError(t, err)
	require.Len(t, impact.GainImpact.Changes, 1)
	before := buyReplacementPreviewSnapshot(t, f.database)

	// A token for a different change set is refused after replay ran.
	march.GainImpactAcknowledgement = "stale"
	_, err = f.investmentService.Sell(ctx, march)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrGainImpactAcknowledgementStale) || errors.Is(err, ErrGainImpactAcknowledgementRequired))
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	// Import acceptance runs last, inside the same transaction: its failure
	// rolls back the journal, historical disposal, decision and revisions.
	march.GainImpactAcknowledgement = impact.GainImpact.Acknowledgement
	var sawTransaction int64
	_, err = f.investmentService.sellWithPostWrite(ctx, march, func(tx *sql.Tx, transactionID int64) error {
		sawTransaction = transactionID
		var decisions int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM investment_disposal_decisions WHERE event_date = '2026-03-01'`).Scan(&decisions); err != nil {
			return err
		}
		if decisions != 1 {
			return errors.New("historical decision not visible to source acceptance")
		}
		return errors.New("import identity already accepted")
	})
	require.ErrorContains(t, err, "import identity already accepted")
	assert.Positive(t, sawTransaction)
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))

	result, err := f.investmentService.sellWithPostWrite(ctx, march, func(*sql.Tx, int64) error { return nil })
	require.NoError(t, err)
	require.NotNil(t, result.DisposalDecision)
	var revisions int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_disposal_revisions`).Scan(&revisions))
	assert.Equal(t, 2, revisions, "replay appends an effective revision for each decision it ran, the new one included")
	assert.Equal(t, SelfCheckPassed, resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckInvestmentFoundation).Status)
	_ = db.GainImpactRevised
}
