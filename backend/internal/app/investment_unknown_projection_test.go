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
	"rekenraam/backend/internal/exact"
)

func TestUnknownProjectedBasisDoesNotBecomeZeroGain(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown',
		remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
	require.NoError(t, err, "unknown basis must have an explicit status and no numeric amount")
	gains, err := f.investmentService.ListUnrealizedGains(context.Background())
	require.NoError(t, err)
	require.Len(t, gains, 1)
	require.Equal(t, db.InvestmentBasisUnknown, gains[0].BasisKnowledge)
	require.Equal(t, "unknown_basis", gains[0].GainUnavailable)
	require.Equal(t, "2", gains[0].QuantityValue.String())
	require.NotNil(t, gains[0].MarketValueValue, "a known price still values an unknown-basis position")
	require.Nil(t, gains[0].UnrealizedGainValue, "an unknown basis must never yield a calculated gain")
}

func TestUnknownProjectionMakesMixedPositionBasisUnavailable(t *testing.T) {
	t.Parallel()
	for _, unknownFirst := range []bool{false, true} {
		t.Run(map[bool]string{true: "first lot unknown", false: "last lot unknown"}[unknownFirst], func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			first := buyOn(t, f, "2026-01-01", 2, 2000)
			last := buyOn(t, f, "2026-01-02", 1, 4000)
			id := *last.LotID
			if unknownFirst {
				id = *first.LotID
			}
			_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown', remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL WHERE lot_id = ?`, id)
			require.NoError(t, err)
			positions, err := f.investmentService.Positions(context.Background())
			require.NoError(t, err)
			require.Len(t, positions, 1)
			require.Equal(t, "3", positions[0].QuantityValue.String())
			require.Equal(t, db.InvestmentBasisUnknown, positions[0].BasisKnowledge)
			gains, err := f.investmentService.ListUnrealizedGains(context.Background())
			require.NoError(t, err)
			require.Equal(t, "unknown_basis", gains[0].GainUnavailable)
			require.Nil(t, gains[0].UnrealizedGainValue)
			require.NotNil(t, gains[0].MarketValueValue)
			require.Zero(t, exact.ScaledIntFromInt64(*gains[0].MarketValueValue, *gains[0].MarketValueScale).Cmp(exact.ScaledIntFromInt64(12000, 2)))
		})
	}
}

func TestKnownZeroTransferBasisRemainsKnown(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	seedExternalTransferEquity(t, f.database)
	input := knownTransferInput(f)
	input.CarriedBasisValue = 0
	result, err := f.investmentService.ExternalTransferIn(context.Background(), input)
	require.NoError(t, err)
	lots, err := f.investmentService.ListLots(context.Background(), f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Len(t, lots, 1)
	require.Equal(t, db.InvestmentBasisKnown, lots[0].BasisKnowledge)
	require.Zero(t, lots[0].RemainingCostBasisValue)
	var basis sql.NullString
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value FROM investment_lot_state WHERE lot_id = ?`, *result.LotID).Scan(&basis))
	require.True(t, basis.Valid)
	require.Equal(t, "0", basis.String)
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
}

func TestUnknownProjectionRetainsQuantitySelfCheck(t *testing.T) {
	t.Parallel()
	for _, damageQuantity := range []bool{false, true} {
		t.Run(map[bool]string{true: "quantity damage", false: "quantity conserved"}[damageQuantity], func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			bought := buyOn(t, f, "2026-01-01", 2, 2000)
			_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown', remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
			require.NoError(t, err)
			if damageQuantity {
				_, err = f.database.Exec(`UPDATE investment_lot_state SET remaining_quantity_value = '1', remaining_quantity_scale = 0`)
				require.NoError(t, err)
			}
			check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
			require.Equal(t, SelfCheckFailed, check.Status)
			require.Contains(t, check.Sample, *bought.LotID)
			require.Contains(t, check.Summary, "unknown projected basis")
			require.NotContains(t, check.Summary, "disagree with their basis events", "unknown must not be compared as zero")
			want := int64(1)
			if damageQuantity {
				want = 3
				require.Contains(t, check.Summary, "holdings disagree")
				require.Contains(t, check.Summary, "disagree with their quantity events")
			}
			require.Equal(t, want, check.FindingCount)
			var basis sql.NullString
			require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value FROM investment_lot_state`).Scan(&basis))
			require.False(t, basis.Valid, "self-check leaves the unresolved projection unchanged")
		})
	}
}

// Only a sale may leave its gain unresolved (T-145). A write-off over unknown
// basis would report a loss it cannot know, so it stays refused, atomically.
func TestWriteOffRefusesUnknownProjectionAtomically(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			bought := buyOn(t, f, "2026-01-01", 2, 2000)
			_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown', remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
			require.NoError(t, err)
			var before int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&before))
			input := backdatedWriteOff(f, "2026-02-01", 1)
			input.CostBasisMethod = method
			if method == "specific_lot" {
				input.LotAllocations = []InvestmentLotAllocationInput{{LotID: *bought.LotID, QuantityValue: exact.New(1)}}
			}
			_, err = f.investmentService.WriteOff(context.Background(), input)
			require.ErrorContains(t, err, "basis is unknown")
			var after, decisions int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&after))
			require.Equal(t, before, after)
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_disposal_decisions`).Scan(&decisions))
			require.Zero(t, decisions)
		})
	}
}

func TestUnknownProjectionExportsEmptyBasisAndKnowledge(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown', remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, NewExportService(db.NewExportRepository(f.database)).WriteBundle(context.Background(), &out, ExportFilter{}))
	archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	seen := 0
	for _, file := range archive.File {
		if file.Name != "lots.csv" && file.Name != "investment-lot-state.csv" {
			continue
		}
		reader, err := file.Open()
		require.NoError(t, err)
		rows, err := csv.NewReader(reader).ReadAll()
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		require.Len(t, rows, 2)
		require.Equal(t, "unknown", rows[1][slices.Index(rows[0], "basis_knowledge")])
		if file.Name == "lots.csv" {
			require.Empty(t, rows[1][10])
			require.Equal(t, "2", rows[1][8])
		} else {
			require.Empty(t, rows[1][4])
			require.Empty(t, rows[1][5])
		}
		seen++
	}
	require.Equal(t, 2, seen)
}

func TestProjectionBasisKnowledgeRequiresMatchingNullability(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	buyOn(t, f, "2026-01-01", 2, 2000)
	for _, test := range []struct{ name, assignment string }{
		{"known without amount", "remaining_cost_basis_value = NULL"},
		{"known without scale", "remaining_cost_basis_scale = NULL"},
		{"unknown with amount", "basis_knowledge = 'unknown', remaining_cost_basis_scale = NULL"},
		{"unknown with scale", "basis_knowledge = 'unknown', remaining_cost_basis_value = NULL"},
		{"unknown with numeric pair", "basis_knowledge = 'unknown'"},
		{"invalid knowledge", "basis_knowledge = 'estimated'"},
		{"null knowledge", "basis_knowledge = NULL"},
		{"negative known basis", "remaining_cost_basis_value = '-1'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.database.Exec(`UPDATE investment_lot_state SET ` + test.assignment)
			require.Error(t, err)
		})
	}
	require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
}

func TestSelfCheckFindsRestoredBasisKnowledgePairDamage(t *testing.T) {
	t.Parallel()
	for _, assignment := range []string{
		"remaining_cost_basis_value = NULL",
		"basis_knowledge = 'unknown'",
		"basis_knowledge = 'estimated'",
	} {
		t.Run(assignment, func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			bought := buyOn(t, f, "2026-01-01", 2, 2000)
			_, err := f.database.Exec(`PRAGMA ignore_check_constraints = ON`)
			require.NoError(t, err)
			_, err = f.database.Exec(`UPDATE investment_lot_state SET ` + assignment)
			require.NoError(t, err)
			check := resultFor(t, mustRunInvestmentSelfCheck(t, f), CheckLotReconciliation)
			require.Equal(t, SelfCheckFailed, check.Status)
			require.EqualValues(t, 1, check.FindingCount)
			require.Contains(t, check.Summary, "inconsistent projected basis knowledge and amounts")
			require.Contains(t, check.Sample, *bought.LotID)
		})
	}
}

func TestKnownOpeningReplayReconstructsUnknownProjectionAtomically(t *testing.T) {
	t.Parallel()
	for _, abort := range []bool{false, true} {
		t.Run(map[bool]string{true: "rollback retains unknown", false: "known evidence reconstructs basis"}[abort], func(t *testing.T) {
			t.Parallel()
			f := newInvestmentsTestFixture(t)
			bought := buyOn(t, f, "2026-01-01", 2, 2000)
			_, err := f.database.Exec(`UPDATE investment_lot_state SET basis_knowledge = 'unknown', remaining_cost_basis_value = NULL, remaining_cost_basis_scale = NULL`)
			require.NoError(t, err)
			var before int
			require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&before))
			if abort {
				_, err = f.database.Exec(`CREATE TRIGGER reject_known_basis_fixture BEFORE UPDATE ON investment_lot_state
					WHEN NEW.basis_knowledge = 'known' BEGIN SELECT RAISE(ABORT, 'fixture basis rebuild failure'); END`)
				require.NoError(t, err)
			}
			_, err = acknowledgedReplaceBuy(context.Background(), f.investmentService, ReplaceInvestmentBuyInput{
				OwnerUserID: f.ownerUserID, TransactionID: bought.Transaction.ID, Reason: "correct sourced purchase",
				Replacement: InvestmentTradeInput{TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
					HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
					QuantityValue: exact.New(2), CashAmountValue: 4000, CashAmountScale: 2},
			})
			var original string
			require.NoError(t, f.database.QueryRow(`SELECT cost_basis_value FROM investment_lots WHERE id = ?`, *bought.LotID).Scan(&original))
			require.Equal(t, "2000", original, "reconstruction never rewrites opening evidence")
			if abort {
				require.ErrorContains(t, err, "fixture basis rebuild failure")
				var after int
				require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&after))
				require.Equal(t, before, after)
				var knowledge string
				var basis sql.NullString
				require.NoError(t, f.database.QueryRow(`SELECT basis_knowledge, remaining_cost_basis_value FROM investment_lot_state WHERE lot_id = ?`, *bought.LotID).Scan(&knowledge, &basis))
				require.Equal(t, "unknown", knowledge)
				require.False(t, basis.Valid)
			} else {
				require.NoError(t, err)
				require.Equal(t, SelfCheckPassed, mustRunInvestmentSelfCheck(t, f).Status)
				positions, err := f.investmentService.Positions(context.Background())
				require.NoError(t, err)
				require.Equal(t, "known", positions[0].BasisKnowledge)
				require.Zero(t, exact.ScaledIntFromInt64(positions[0].RemainingCostBasisValue, positions[0].RemainingCostBasisScale).Cmp(exact.ScaledIntFromInt64(4000, 2)))
			}
		})
	}
}
