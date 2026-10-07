package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

// Until the public command admits unknown inputs, prepare its security-only
// journal through the current application builder and exercise the actual
// repository writer with explicit unknown opening evidence.
func seedUnknownTransferOpening(t *testing.T, f *investmentsTestFixture) int64 {
	t.Helper()
	seedExternalTransferEquity(t, f.database)
	input := knownTransferInput(f)
	input.CarriedBasisValue, input.CarriedBasisScale = 0, 0
	journal, transfer, err := f.investmentService.externalTransferInWrite(context.Background(), input)
	require.NoError(t, err)
	transfer.Lot.OpeningBasisKnowledge = db.InvestmentBasisUnknown
	_, lot, err := f.investmentService.repository.CreateExternalTransferIn(context.Background(), journal, transfer)
	require.NoError(t, err)
	return lot.ID
}

func TestUnknownImmutableOpeningRetainsQuantityAndNullEvidence(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	lotID := seedUnknownTransferOpening(t, f)
	lots, err := f.investmentService.ListLots(context.Background(), f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, "unknown", lots[0].OpeningBasisKnowledge)
	require.Equal(t, "unknown", lots[0].BasisKnowledge)
	require.Equal(t, "2", lots[0].RemainingQuantityValue.String())
	require.Equal(t, "2", lots[0].QuantityValue.String())
	gains, err := f.investmentService.ListUnrealizedGains(context.Background())
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.Nil(t, gains[0].UnrealizedGainValue)
	var amount, scale sql.NullString
	require.NoError(t, f.database.QueryRow(`SELECT cost_basis_value,cost_basis_scale FROM investment_lots WHERE id=?`, lotID).Scan(&amount, &scale))
	require.False(t, amount.Valid)
	require.False(t, scale.Valid)
	_, err = f.database.Exec(`UPDATE investment_lots SET opening_basis_knowledge='known',cost_basis_value='0',cost_basis_scale=0 WHERE id=?`, lotID)
	require.ErrorContains(t, err, "immutable")
	repository := db.NewInvestmentRepository(f.database)
	intents, err := repository.ListInvestmentReplayIntents(context.Background(), BookID, f.holdingAccountID, f.stockCommodityID, f.eurCommodityID, "long")
	require.NoError(t, err)
	require.Len(t, intents, 1)
	require.Equal(t, "unknown", intents[0].BasisKnowledge)
	check := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, check, CheckInvestmentFoundation).Status)
	require.Equal(t, SelfCheckPassed, resultFor(t, check, CheckInvestmentReplay).Status)
	quantities := resultFor(t, check, CheckLotReconciliation)
	require.Equal(t, int64(1), quantities.FindingCount, "only unresolved basis, not a quantity or numeric-zero mismatch")
	require.Contains(t, quantities.Summary, "unknown projected basis")
}

func TestImmutableOpeningAndEventKnowledgeRequiresNullPairs(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	input := knownTransferInput(f)
	input.CarriedBasisValue = 0
	_, err := f.investmentService.ExternalTransferIn(context.Background(), input)
	require.NoError(t, err)
	for _, table := range []struct{ name, trigger, knowledge string }{
		{"investment_lots", "investment_lots_opening_no_update", "opening_basis_knowledge"},
		{"investment_lot_events", "investment_lot_events_no_update", "basis_knowledge"},
	} {
		for _, knowledge := range []string{"known", "unknown"} {
			for mask := 0; mask < 4; mask++ {
				t.Run(table.name+"/"+knowledge+"/"+string(rune('0'+mask)), func(t *testing.T) {
					tx, err := f.database.BeginTx(context.Background(), nil)
					require.NoError(t, err)
					defer tx.Rollback()
					_, err = tx.Exec(`DROP TRIGGER ` + table.trigger)
					require.NoError(t, err)
					var value, scale any
					if mask&1 != 0 {
						value = "0"
					}
					if mask&2 != 0 {
						scale = 0
					}
					_, err = tx.Exec(`UPDATE `+table.name+` SET `+table.knowledge+`=?, cost_basis_value=?,cost_basis_scale=?`, knowledge, value, scale)
					if knowledge == "known" && mask == 3 || knowledge == "unknown" && mask == 0 {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, "CHECK constraint failed")
					}
				})
			}
		}
	}
}

func TestUnknownImmutableOpeningKeepsQuantityAndKnowledgeDamageVisible(t *testing.T) {
	t.Parallel()
	for _, damage := range []string{"quantity", "invented zero"} {
		t.Run(damage, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			lotID := seedUnknownTransferOpening(t, f)
			statement := `UPDATE investment_lot_state SET remaining_quantity_value='1' WHERE lot_id=?`
			if damage == "invented zero" {
				statement = `UPDATE investment_lot_state SET basis_knowledge='known',remaining_cost_basis_value='0',remaining_cost_basis_scale=0 WHERE lot_id=?`
			}
			_, err := f.database.Exec(statement, lotID)
			require.NoError(t, err)
			check := mustRunInvestmentSelfCheck(t, f)
			require.Equal(t, SelfCheckFailed, resultFor(t, check, CheckInvestmentReplay).Status)
			if damage == "quantity" {
				quantity := resultFor(t, check, CheckLotReconciliation)
				require.Contains(t, quantity.Summary, "holdings disagree")
				require.Contains(t, quantity.Summary, "disagree with their quantity events")
			}
		})
	}
}

func TestUnknownImmutableOpeningExportsNullAmountsAndOriginalKnowledge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedUnknownTransferOpening(t, f)
	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(context.Background(), &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	seen := 0
	for _, file := range archive.File {
		var amount, scale, knowledge string
		switch file.Name {
		case "lots.csv":
			amount, knowledge = "cost_basis", "opening_basis_knowledge"
		case "investment-lot-facts.csv":
			amount, scale, knowledge = "consideration_value", "consideration_scale", "opening_basis_knowledge"
		case "investment-lot-events.csv":
			amount, scale, knowledge = "cost_basis_value", "cost_basis_scale", "basis_knowledge"
		default:
			continue
		}
		reader, err := file.Open()
		require.NoError(t, err)
		rows, err := csv.NewReader(reader).ReadAll()
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.Len(t, rows, 2)
		require.Empty(t, rows[1][slices.Index(rows[0], amount)])
		if scale != "" {
			require.Empty(t, rows[1][slices.Index(rows[0], scale)])
		}
		require.Equal(t, "unknown", rows[1][slices.Index(rows[0], knowledge)])
		seen++
	}
	require.Equal(t, 3, seen)
}

func TestUnknownOpeningReplayPersistsNullsWithoutChangingKnownEvidence(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 2, 2000)
	sale, err := f.investmentService.Sell(ctx, sellInput(f, "2026-03-01", 1))
	require.NoError(t, err)
	_, err = acknowledgedReverseSale(ctx, f.investmentService, ReverseInvestmentSaleInput{
		OwnerUserID: f.ownerUserID, OperationID: saleOperationIDForTest(t, f, sale.Transaction.ID), Reason: "canceled trade",
	})
	require.NoError(t, err)
	seedExternalTransferEquity(t, f.database)
	input := knownTransferInput(f)
	input.EffectiveOn, input.CarriedBasisValue, input.CarriedBasisScale = "2026-02-01", 0, 0
	journal, transfer, err := f.investmentService.externalTransferInWrite(ctx, input)
	require.NoError(t, err)
	transfer.Lot.OpeningBasisKnowledge = db.InvestmentBasisUnknown
	journal.GainImpact = gainImpactPolicy("")
	// The canceled depletion still makes this a replay admission. No effective
	// disposal remains, so this exercises the supported opening-only replay.
	var before int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&before))
	for range 2 {
		_, err = f.investmentService.repository.SimulateExternalTransferIn(ctx, journal, transfer)
		require.NoError(t, err)
	}
	var after int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&after))
	require.Equal(t, before, after)
	_, lot, err := f.investmentService.repository.CreateExternalTransferIn(ctx, journal, transfer)
	require.NoError(t, err)
	var amount, scale sql.NullString
	var knowledge string
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value,remaining_cost_basis_scale,basis_knowledge
		FROM investment_lot_state WHERE lot_id=?`, lot.ID).Scan(&amount, &scale, &knowledge))
	require.False(t, amount.Valid)
	require.False(t, scale.Valid)
	require.Equal(t, "unknown", knowledge)
	check := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, check, CheckInvestmentReplay).Status)
	require.Equal(t, SelfCheckPassed, resultFor(t, check, CheckInvestmentFoundation).Status)
}

func TestUnknownOpeningWriterRefusesInventedAmountAndCurrencyJournalAtomically(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"invented amount", "currency journal"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			seedExternalTransferEquity(t, f.database)
			input := knownTransferInput(f)
			if kind == "invented amount" {
				input.CarriedBasisValue = 0
			}
			journal, transfer, err := f.investmentService.externalTransferInWrite(context.Background(), input)
			require.NoError(t, err)
			transfer.Lot.OpeningBasisKnowledge = db.InvestmentBasisUnknown
			transfer.Lot.CostBasisValue, transfer.Lot.CostBasisScale = 0, 0
			if kind == "invented amount" {
				transfer.Lot.CostBasisValue = 1
			}
			before := buyReplacementPreviewSnapshot(t, f.database)
			_, _, err = f.investmentService.repository.CreateExternalTransferIn(context.Background(), journal, transfer)
			require.Error(t, err)
			require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
		})
	}
}

func TestUnknownImmutableOpeningKeepsDisposalAdmissionGated(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedUnknownTransferOpening(t, f)
	before := buyReplacementPreviewSnapshot(t, f.database)
	input := sellInput(f, "2026-07-01", 1)
	_, err := f.investmentService.PreviewSell(context.Background(), input)
	require.ErrorContains(t, err, "basis is unknown")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
	_, err = f.investmentService.Sell(context.Background(), input)
	require.ErrorContains(t, err, "basis is unknown")
	require.Equal(t, before, buyReplacementPreviewSnapshot(t, f.database))
}
