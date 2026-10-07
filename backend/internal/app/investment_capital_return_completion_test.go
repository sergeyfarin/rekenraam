package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

func TestCapitalReturnScaleIndependence(t *testing.T) {
	t.Parallel()
	allocations := make([][]*exact.ScaledInt, 2)
	for i, scale := range []int{0, 2} {
		f := newInvestmentsTestFixture(t)
		buyOn(t, f, "2026-05-01", 1, 1000)
		buyOn(t, f, "2026-05-02", 1, 1000)
		input := capitalReturnInput(f, "2026-06-01", 100)
		if scale == 0 {
			input.AmountValue, input.AmountScale = exact.New(1), 0
		}
		result, err := f.investmentService.CapitalReturn(context.Background(), input)
		require.NoError(t, err)
		for _, e := range result.Effects {
			allocations[i] = append(allocations[i], coefScaled(e.ReductionValue, e.ReductionScale))
			requireScaled(t, 5, 1, coefScaled(e.ReductionValue, e.ReductionScale), "equal units share an integer receipt equally")
		}
		t.Logf("amount scale %d reductions %s, %s", scale, allocations[i][0], allocations[i][1])
		requireInvestmentSelfCheckPasses(t, f)
		sale, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-07-01", 1))
		require.NoError(t, err)
		requireScaled(t, 950, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "later FIFO basis is independent of receipt spelling")
	}
	require.Zero(t, allocations[0][0].Cmp(allocations[1][0]), "equivalent monetary amounts must reduce the same lot basis")
}

func TestCapitalReturnOriginalDamageAfterRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 1, 700)
	_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
	require.NoError(t, err)
	_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 1, 500))
	require.NoError(t, err)
	require.Equal(t, 1, capitalReturnRevisionCount(t, f))
	requireInvestmentSelfCheckPasses(t, f)
	_, err = f.database.Exec(`DROP TRIGGER investment_capital_return_effects_no_update`)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_capital_return_effects SET allocated_value='1100' WHERE operation_id IN (SELECT operation_id FROM investment_capital_return_facts)`)
	require.NoError(t, err)
	run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
	require.NoError(t, err)
	foundation := resultFor(t, run, CheckInvestmentFoundation)
	replay := resultFor(t, run, CheckInvestmentReplay)
	t.Logf("damaged original 11.00 allocation for 10.00 receipt: foundation=%s replay=%s", foundation.Status, replay.Status)
	require.NotEqual(t, SelfCheckPassed, foundation.Status, "self-check must audit original receipt evidence after replay supersedes it")
}

func TestCapitalReturnReplacementKeepsSameDaySlot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-05-01", 2, 2000)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-06-01", 1))
	require.NoError(t, err)
	input := ReplaceInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: roc.Transaction.ID, Reason: "receipt was 6.00", Replacement: capitalReturnInput(f, "2026-06-01", 600)}
	before := f.transactionCount(t)
	preview, err := f.investmentService.PreviewCapitalReturnReplacement(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before, f.transactionCount(t))
	require.NotNil(t, preview.Impact.GainImpact)
	_, err = f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementRequired)
	require.Equal(t, before, f.transactionCount(t))
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	input.Replacement.AmountValue = exact.New(800)
	_, err = f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.ErrorIs(t, err, db.ErrGainImpactAcknowledgementStale)
	require.Equal(t, before, f.transactionCount(t))
	input.Replacement.AmountValue = exact.New(600)
	result, err := f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Equal(t, before+2, f.transactionCount(t))
	requireScaled(t, 700, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "replacement preserves the original slot before the sale")
	requireInvestmentSelfCheckPasses(t, f)
	// A second replacement retains the first operation's slot, too.
	input.TransactionID = result.Replacement.Transaction.ID
	input.Replacement.AmountValue = exact.New(800)
	preview, err = f.investmentService.PreviewCapitalReturnReplacement(ctx, input)
	require.NoError(t, err)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.NoError(t, err)
	requireScaled(t, 600, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "second replacement keeps the root slot")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnExplicitQuantitiesCapOnlyEntitledBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 2, 2000)
	input := capitalReturnInput(f, "2026-06-01", 1500)
	input.LotEntitlements = []CapitalReturnEntitlement{{LotID: *buy.LotID, QuantityValue: exact.New(1), QuantityScale: 0}}
	preview, err := f.investmentService.PreviewCapitalReturn(ctx, input)
	require.NoError(t, err)
	requireScaled(t, 1000, 2, coefScaled(preview.Effects[0].ReductionValue, preview.Effects[0].ReductionScale), "basis of one entitled unit")
	requireScaled(t, 500, 2, coefScaled(preview.Effects[0].ExcessValue, preview.Effects[0].ExcessScale), "unentitled basis cannot absorb excess")
	_, err = f.investmentService.CapitalReturn(ctx, input)
	require.NoError(t, err)
	requireScaled(t, 1000, 2, lotRemainingBasis(t, f, *buy.LotID), "basis of the holding remains")
	// A corrected opening keeps the fixed entitlement quantity on its successor.
	replacement, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 2, 1000))
	require.NoError(t, err)
	requireScaled(t, 500, 2, lotRemainingBasis(t, f, *replacement.Replacement.LotID), "one of two units still entitled after replay")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnExplicitQuantityOverHoldingRefusedWithoutWriting(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buy := buyOn(t, f, "2026-05-01", 1, 1000)
	input := capitalReturnInput(f, "2026-06-01", 100)
	input.LotEntitlements = []CapitalReturnEntitlement{{LotID: *buy.LotID, QuantityValue: exact.New(2)}}
	before := f.transactionCount(t)
	_, err := f.investmentService.PreviewCapitalReturn(context.Background(), input)
	require.Error(t, err)
	_, err = f.investmentService.CapitalReturn(context.Background(), input)
	require.Error(t, err)
	require.Equal(t, before, f.transactionCount(t))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnSelfCheckAuditsSupersededAndMissingRevisionEffects(t *testing.T) {
	t.Parallel()
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint("missing=", missing), func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			buy := buyOn(t, f, "2026-05-01", 1, 700)
			_, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 1000))
			require.NoError(t, err)
			corrected, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 1, 500))
			require.NoError(t, err)
			_, err = acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, corrected.Replacement, "2026-05-01", 1, 400))
			require.NoError(t, err)
			require.Equal(t, 2, capitalReturnRevisionCount(t, f))
			requireInvestmentSelfCheckPasses(t, f)
			if missing {
				_, err = f.database.Exec(`DROP TRIGGER investment_capital_return_revision_effects_no_delete`)
				require.NoError(t, err)
				_, err = f.database.Exec(`DELETE FROM investment_capital_return_revision_effects WHERE revision_id = (SELECT MIN(id) FROM investment_capital_return_revisions)`)
			} else {
				_, err = f.database.Exec(`DROP TRIGGER investment_capital_return_revision_effects_no_update`)
				require.NoError(t, err)
				_, err = f.database.Exec(`UPDATE investment_capital_return_revision_effects SET allocated_value='1100', allocated_scale=2 WHERE revision_id = (SELECT MIN(id) FROM investment_capital_return_revisions)`)
			}
			require.NoError(t, err)
			run, err := selfCheckOver(t, f.database).RunSelfCheck(ctx, "manual")
			require.NoError(t, err)
			require.NotEqual(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentFoundation).Status)
		})
	}
}

func TestCapitalReturnReplacementLateFailureRollsBackJournalFactsAndReplay(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 2, 2000)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	checkpoint := reconcileCash(t, f, "2026-07-01", -16)
	input := ReplaceInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: roc.Transaction.ID,
		Reason: "receipt corrected", ReconciliationOverride: true, Replacement: capitalReturnInput(f, "2026-06-01", 600)}
	preview, err := f.investmentService.PreviewCapitalReturnReplacement(ctx, input)
	require.NoError(t, err)
	require.Len(t, preview.Impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpoint, preview.Impact.AffectedCheckpoints[0].CheckpointID)
	before := buyReplacementPreviewSnapshot(t, f.database)
	_, err = f.database.Exec(`CREATE TRIGGER reject_capital_return_checkpoint AFTER UPDATE ON reconciliation_checkpoints
 WHEN NEW.status = 'invalidated' BEGIN SELECT RAISE(ABORT, 'forced checkpoint failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.ErrorContains(t, err, "forced checkpoint failure")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	requireScaled(t, 1600, 2, lotRemainingBasis(t, f, *buy.LotID), "failed replacement retains original basis")
	_, err = f.database.Exec(`DROP TRIGGER reject_capital_return_checkpoint`)
	require.NoError(t, err)
	_, err = f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDs(t, f))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnReplacementMovesEffectiveAndPaymentDatesThenReverses(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 2, 2000)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-06-01", 1))
	require.NoError(t, err)
	replacement := capitalReturnInput(f, "2026-06-02", 600)
	replacement.PaymentOn = "2026-06-05"
	input := ReplaceInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: roc.Transaction.ID, Reason: "wrong dates", Replacement: replacement}
	preview, err := f.investmentService.PreviewCapitalReturnReplacement(ctx, input)
	require.NoError(t, err)
	require.Len(t, preview.Effects, 1)
	requireScaled(t, 1, 0, coefScaled(preview.Effects[0].EntitledQuantityValue, preview.Effects[0].EntitledQuantityScale), "one unit is held at the corrected date")
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	corrected, err := f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "2026-06-05", corrected.Replacement.Transaction.TransactionDate)
	requireScaled(t, 1000, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "corrected date now follows the sale")
	requireScaled(t, 400, 2, lotRemainingBasis(t, f, *buy.LotID), "remaining units reduced at the later date")
	requireInvestmentSelfCheckPasses(t, f)
	_, err = f.investmentService.ReverseCapitalReturn(ctx, ReverseInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: corrected.Replacement.Transaction.ID, Reason: "notice withdrawn"})
	require.NoError(t, err)
	requireScaled(t, 1000, 2, lotRemainingBasis(t, f, *buy.LotID), "reversing the replacement restores basis")
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, roc.Transaction.ID)
	require.NoError(t, err)
	require.False(t, chain.CanReplaceCapitalReturn)
	require.False(t, chain.CanReverseCapitalReturn)
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnLargeReceiptRetainsExactNormalizedCoefficient(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-05-01", 1, 1000)
	input := capitalReturnInput(f, "2026-06-01", 100)
	input.AmountValue, input.AmountScale = exact.MustParse("10000000000000000000000000000000000000"), 0
	result, err := f.investmentService.CapitalReturn(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, result.Effects, 1)
	require.Zero(t, coefScaled(result.Effects[0].AllocatedValue, result.Effects[0].AllocatedScale).Cmp(coefScaled(input.AmountValue, 0)))
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnBundlePreservesOriginalReplacementAndQuantityRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buy := buyOn(t, f, "2026-05-01", 2, 2000)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	replacement := capitalReturnInput(f, "2026-06-01", 1500)
	replacement.LotEntitlements = []CapitalReturnEntitlement{{LotID: *buy.LotID, QuantityValue: exact.New(1)}}
	corrected, err := f.investmentService.ReplaceCapitalReturn(ctx, ReplaceInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: roc.Transaction.ID, Reason: "entitlement corrected", Replacement: replacement})
	require.NoError(t, err)
	newBuy, err := acknowledgedReplaceBuy(ctx, f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 2, 1000))
	require.NoError(t, err)
	chain, err := f.investmentService.CorrectionChain(ctx, f.ownerUserID, corrected.Replacement.Transaction.ID)
	require.NoError(t, err)
	require.Equal(t, *newBuy.Replacement.LotID, chain.EffectiveCapitalReturn.LotEntitlements[0].LotID)
	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(ctx, &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	files := make(map[string][][]string)
	for _, file := range archive.File {
		if !strings.HasPrefix(file.Name, "investment-capital-return-") {
			continue
		}
		reader, err := file.Open()
		require.NoError(t, err)
		rows, err := csv.NewReader(reader).ReadAll()
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		files[file.Name] = rows
	}
	require.Len(t, files["investment-capital-return-facts.csv"], 3, "both original and replacement exported")
	require.Equal(t, "open_lots_per_share", files["investment-capital-return-facts.csv"][1][9])
	require.Equal(t, "explicit_quantities", files["investment-capital-return-facts.csv"][2][9])
	require.Len(t, files["investment-capital-return-entitlements.csv"], 2)
	require.Equal(t, []string{strconv.FormatInt(*buy.LotID, 10), "1", "0"}, files["investment-capital-return-entitlements.csv"][1][2:])
	require.Len(t, files["investment-capital-return-effects.csv"], 3)
	require.Len(t, files["investment-capital-return-revisions.csv"], 2)
	require.Len(t, files["investment-capital-return-revision-effects.csv"], 2)
	require.Equal(t, strconv.FormatInt(*newBuy.Replacement.LotID, 10), files["investment-capital-return-revision-effects.csv"][1][2])
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnFractionalEntitlementsAllocateReceiptAndCapEachLot(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	first := buyOn(t, f, "2026-05-01", 3, 1200)
	second := buyOn(t, f, "2026-05-02", 2, 400)
	input := capitalReturnInput(f, "2026-06-01", 1000)
	input.LotEntitlements = []CapitalReturnEntitlement{
		{LotID: *first.LotID, QuantityValue: exact.New(15), QuantityScale: 1},
		{LotID: *second.LotID, QuantityValue: exact.New(5), QuantityScale: 1},
	}
	result, err := f.investmentService.CapitalReturn(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, result.Effects, 2)
	for index, expected := range []struct{ allocated, reduction, excess int64 }{{750, 600, 150}, {250, 100, 150}} {
		effect := result.Effects[index]
		requireScaled(t, expected.allocated, 2, coefScaled(effect.AllocatedValue, effect.AllocatedScale), "allocation by entitled quantities")
		requireScaled(t, expected.reduction, 2, coefScaled(effect.ReductionValue, effect.ReductionScale), "proportional basis cap")
		requireScaled(t, expected.excess, 2, coefScaled(effect.ExcessValue, effect.ExcessScale), "unentitled units retain their basis")
	}
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnEntitlementCorrectionPreservesUnchangedCashCheckpoint(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	first := buyOn(t, f, "2026-05-01", 1, 1000)
	buyOn(t, f, "2026-05-02", 1, 1000)
	roc, err := f.investmentService.CapitalReturn(ctx, capitalReturnInput(f, "2026-06-01", 400))
	require.NoError(t, err)
	checkpoint := reconcileCash(t, f, "2026-07-01", -16)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-08-01", 1))
	require.NoError(t, err)
	replacement := capitalReturnInput(f, "2026-06-01", 400)
	replacement.LotEntitlements = []CapitalReturnEntitlement{{LotID: *first.LotID, QuantityValue: exact.New(1)}}
	input := ReplaceInvestmentCapitalReturnInput{OwnerUserID: f.ownerUserID, TransactionID: roc.Transaction.ID, Reason: "only first lot entitled", Replacement: replacement}
	preview, err := f.investmentService.PreviewCapitalReturnReplacement(ctx, input)
	require.NoError(t, err)
	require.Empty(t, preview.Impact.AffectedCheckpoints, "combined inverse and receipt preserve reconciled cash")
	require.NotNil(t, preview.Impact.GainImpact)
	input.GainImpactAcknowledgement = preview.Impact.GainImpact.Acknowledgement
	_, err = f.investmentService.ReplaceCapitalReturn(ctx, input)
	require.NoError(t, err)
	require.Equal(t, []int64{checkpoint}, activeCheckpointIDs(t, f))
	requireScaled(t, 600, 2, saleEffectiveBasis(t, f, sale.Transaction.ID), "specific entitlement changes basis without changing cash")
	requireInvestmentSelfCheckPasses(t, f)
}

func TestCapitalReturnInvalidEntitlementsNeverFallBackToAllLots(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buy := buyOn(t, f, "2026-05-01", 1, 1000)
	valid := CapitalReturnEntitlement{LotID: *buy.LotID, QuantityValue: exact.New(1)}
	cases := map[string][]CapitalReturnEntitlement{
		"empty":         {},
		"zero":          {{LotID: *buy.LotID, QuantityValue: exact.New(0)}},
		"negative":      {{LotID: *buy.LotID, QuantityValue: exact.New(-1)}},
		"duplicate":     {valid, valid},
		"invalid scale": {{LotID: *buy.LotID, QuantityValue: exact.New(1), QuantityScale: 25}},
	}
	before := buyReplacementPreviewSnapshot(t, f.database)
	for name, entitlements := range cases {
		t.Run(name, func(t *testing.T) {
			input := capitalReturnInput(f, "2026-06-01", 100)
			input.LotEntitlements = entitlements
			_, err := f.investmentService.CapitalReturn(context.Background(), input)
			require.Error(t, err)
			require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
		})
	}
}

func TestCapitalReturnEntitlementAllocationScaleIncludesUnentitledPositionBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buy := buyOn(t, f, "2026-05-01", 3, 1000)
	buyOn(t, f, "2026-05-02", 1, 10000000000000000)
	input := capitalReturnInput(f, "2026-06-01", 500)
	input.LotEntitlements = []CapitalReturnEntitlement{{LotID: *buy.LotID, QuantityValue: exact.New(1)}}
	result, err := f.investmentService.CapitalReturn(context.Background(), input)
	require.NoError(t, err, "choose allocation precision against the whole position's representable basis")
	requireScaled(t, 33333, 4, coefScaled(result.Effects[0].ReductionValue, result.Effects[0].ReductionScale), "proportional cap at the position-wide backed-off scale")
	requireInvestmentSelfCheckPasses(t, f)
}
