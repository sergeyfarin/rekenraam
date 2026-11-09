package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func TestCSVIncomeMissingPayerPreservesKnownAmountsAndImportIdentity(t *testing.T) {
	t.Parallel()
	for _, omitColumn := range []bool{false, true} {
		name := "empty source cells"
		if omitColumn {
			name = "no payer column"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f, _, tax, profile := csvIncomeFixture(t)
			if omitColumn {
				var config csvProfileConfig
				require.NoError(t, json.Unmarshal([]byte(profile.ConfigJSON), &config))
				config.Investment.InstrumentColumn = ""
				encoded, err := json.Marshal(config)
				require.NoError(t, err)
				profile, err = f.importService.CreateImportProfile(ctx, CreateImportProfileInput{OwnerUserID: f.ownerUserID, Name: "No payer", AdapterKind: "csv", ConfigJSON: string(encoded)})
				require.NoError(t, err)
			}
			rows := "2026-03-01;DIV;;85,00;100,00;15,00;unassigned-div\n2026-03-02;TAX;;-2,00;;;unassigned-tax\n"
			staged := stageCSVIncome(t, f, profile, rows)
			preview, err := f.importService.PreviewCommit(ctx, PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
			require.NoError(t, err)
			require.Equal(t, 2, preview.IncludableCount)
			committed, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
			require.NoError(t, err)
			require.Equal(t, 2, committed.CommittedCount)
			fixture := &investmentsTestFixture{database: f.database}
			require.Equal(t, int64(8300), accountBalance(t, fixture, f.cashAccountID))
			require.Equal(t, int64(-10000), accountBalance(t, fixture, f.categoryAccountID))
			require.Equal(t, int64(1700), accountBalance(t, fixture, tax))
			stored, err := f.importRepo.ListAllImportStagedRows(ctx, staged.Batch.ID)
			require.NoError(t, err)
			for i, row := range stored {
				chain, err := f.investmentSvc.CorrectionChain(ctx, f.ownerUserID, row.CommittedTransactionID.Int64)
				require.NoError(t, err)
				require.Nil(t, chain.EffectiveDividend.CommodityID)
				require.Equal(t, i == 1, chain.EffectiveDividend.WithholdingOnly)
			}
			count := transactionCount(t, f)
			again := stageCSVIncome(t, f, profile, rows)
			for _, row := range again.Rows {
				require.Equal(t, "duplicate", row.DedupeStatus)
			}
			_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: again.Batch.ID})
			require.NoError(t, err)
			require.Equal(t, count, transactionCount(t, f))
		})
	}
}

func csvIncomeFixture(t *testing.T) (*investTestFixture, InvestmentInstrument, int64, ImportProfile) {
	t.Helper()
	f := newInvestTestFixture(t)
	instrument, err := f.investmentSvc.CreateInstrument(context.Background(), InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", ChangeReason: "fixture", CommodityCode: "PAYER", InstrumentType: "stock", DisplayName: "Payer", QuantityScale: 0, PriceScale: 2,
	})
	require.NoError(t, err)
	cash, err := f.investmentSvc.accountService.CreateAccount(context.Background(), CreateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create", ChangeReason: "fixture", Name: "Income cash", AccountClass: "asset", AccountKind: "brokerage_cash", DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	f.cashAccountID = cash.ID
	tax := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	config := fmt.Sprintf(`{"delimiter":"semicolon","date_column":"Date","amount_column":"Cash","date_layout":"YMD","decimal_separator":",","external_ref_column":"Ref","investment":{"kind":"column","kind_column":"Type","dividend_value":"DIV","withholding_value":"TAX","instrument_column":"Code","gross_column":"Gross","withholding_column":"Tax","cash_account_id":%d,"income_account_id":%d,"withholding_account_id":%d}}`, f.cashAccountID, f.categoryAccountID, tax)
	profile, err := f.importService.CreateImportProfile(context.Background(), CreateImportProfileInput{OwnerUserID: f.ownerUserID, Name: "Income", AdapterKind: "csv", ConfigJSON: config})
	require.NoError(t, err)
	return f, instrument, tax, profile
}

func stageCSVIncome(t *testing.T, f *investTestFixture, profile ImportProfile, rows string) StartImportResult {
	t.Helper()
	staged, err := f.importService.StartImport(context.Background(), StartImportInput{OwnerUserID: f.ownerUserID, ProfileID: &profile.ID,
		Input: RawInput{Filename: "income.csv", Bytes: []byte("Date;Type;Code;Cash;Gross;Tax;Ref\n" + rows)},
	})
	require.NoError(t, err)
	return staged
}

func TestCSVInvestmentIncomeCommitsCorrectsExportsAndDeduplicates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, instrument, tax, profile := csvIncomeFixture(t)
	rows := "2026-03-01;DIV;PAYER;85,00;100,00;15,00;div-1\n2026-03-02;TAX;PAYER;-5,00;;;tax-1\n"
	staged := stageCSVIncome(t, f, profile, rows)
	require.Len(t, staged.Rows, 2)

	preview, err := f.importService.PreviewCommit(ctx, PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Equal(t, 2, preview.IncludableCount)
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Equal(t, 2, result.CommittedCount)
	activity, err := f.importService.transactionService.ListTransactions(ctx, ListTransactionsInput{CommodityID: instrument.CommodityID})
	require.NoError(t, err)
	require.Len(t, activity.Transactions, 2)
	income, err := f.investmentSvc.repository.InvestmentIncome(ctx, BookID, "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	require.Len(t, income, 1)
	require.Equal(t, "10000", income[0].Value.String())
	stored, err := f.importRepo.ListAllImportStagedRows(ctx, staged.Batch.ID)
	require.NoError(t, err)
	for _, row := range stored {
		require.Len(t, row.CommitEffects, 1)
		require.True(t, row.CommitEffects[0].OperationID.Valid)
	}
	id := stored[1].CommittedTransactionID.Int64
	chain, err := f.investmentSvc.CorrectionChain(ctx, f.ownerUserID, id)
	require.NoError(t, err)
	require.True(t, chain.EffectiveDividend.WithholdingOnly)
	require.Equal(t, &instrument.CommodityID, chain.EffectiveDividend.CommodityID)
	_, err = f.investmentSvc.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: id, Reason: "tax corrected", Replacement: DividendInput{
		TransactionDate: "2026-03-02", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, AmountValue: 600, AmountScale: 2, WithholdingAccountID: &tax,
	}})
	require.NoError(t, err)
	duplicate := stageCSVIncome(t, f, profile, rows)
	for _, row := range duplicate.Rows {
		require.Equal(t, "duplicate", row.DedupeStatus)
	}
	snapshot, err := db.NewExportRepository(f.database).Snapshot(ctx)
	require.NoError(t, err)
	facts, err := db.NewExportRepository(f.database).ExportInvestmentFoundation(ctx, snapshot, BookID, "dividend-facts")
	require.NoError(t, err)
	require.Len(t, facts, 3)
	require.NoError(t, snapshot.Rollback())
}

func TestCSVInvestmentIncomeRefusesUnresolvedAndMismatchedRowsWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, row := range []string{
		"2026-03-01;DIV;UNKNOWN;85,00;100,00;15,00;one\n",
		"2026-03-01;OTHER;PAYER;85,00;100,00;15,00;one\n",
		"2026-03-01;DIV;PAYER;85,00;85,00;15,00;one\n",
		"2026-03-01;DIV;PAYER;85,00;100.00;15,00;one\n",
		"2026-03-01;DIV;PAYER;85.00;100,00;15,00;one\n",
		"2026-03-01;TAX;PAYER;5,00;;;one\n",
	} {
		t.Run(row, func(t *testing.T) {
			t.Parallel()
			f, _, _, profile := csvIncomeFixture(t)
			staged := stageCSVIncome(t, f, profile, "2026-02-01;DIV;PAYER;85,00;100,00;15,00;valid-first\n"+row)
			before := csvIncomeDurableCount(t, f)
			_, err := f.importService.PreviewCommit(context.Background(), PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
			var validation ValidationError
			require.ErrorAs(t, err, &validation)
			_, err = f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
			require.ErrorAs(t, err, &validation)
			require.Equal(t, before, csvIncomeDurableCount(t, f))
			batch, err := f.importRepo.ImportBatchByID(context.Background(), BookID, staged.Batch.ID)
			require.NoError(t, err)
			require.Equal(t, "previewing", batch.Status)
		})
	}
}

func csvIncomeDurableCount(t *testing.T, f *investTestFixture) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"transactions", "audit_events", "investment_operations", "investment_dividend_facts", "import_commit_identities"} {
		var count int
		require.NoError(t, f.database.QueryRow("SELECT count(*) FROM "+table).Scan(&count))
		counts[table] = count
	}
	return counts
}

func TestCSVInvestmentIncomeReviewCanResolveUnknownKindAndInstrument(t *testing.T) {
	t.Parallel()
	f, instrument, _, profile := csvIncomeFixture(t)
	staged := stageCSVIncome(t, f, profile, "2026-03-01;OTHER;UNKNOWN;85,00;100,00;15,00;one\n")
	var resolution ImportRowResolution
	require.NoError(t, json.Unmarshal([]byte(staged.Rows[0].ResolutionJSON), &resolution))
	resolution.InvestmentKind, resolution.InstrumentCommodityID = "dividend", instrument.CommodityID
	encoded, err := json.Marshal(resolution)
	require.NoError(t, err)
	require.NoError(t, f.importService.PatchImportBatch(context.Background(), PatchImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID, RowResolutions: []RowResolutionPatch{{RowID: staged.Rows[0].ID, DedupeStatus: "new", ResolutionJSON: string(encoded)}}}))
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Equal(t, 1, result.CommittedCount)
}

func TestCSVInvestmentIdentityFailureRollsBackJournalAndFacts(t *testing.T) {
	t.Parallel()
	f, _, _, profile := csvIncomeFixture(t)
	staged := stageCSVIncome(t, f, profile, "2026-03-01;DIV;PAYER;85,00;100,00;15,00;one\n")
	failInvestmentImportIdentityWrites(t, f)
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Equal(t, 1, result.FailedCount)
	for _, table := range []string{"transactions", "investment_operations", "investment_dividend_facts", "import_commit_identities"} {
		var count int
		require.NoError(t, f.database.QueryRow("SELECT count(*) FROM "+table).Scan(&count))
		require.Zero(t, count, table)
	}
}

func TestCSVInvestmentPreviewAndReconciliationRefusalLeaveNoWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, _, _, profile := csvIncomeFixture(t)
	_, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 10000, AmountScale: 2})
	require.NoError(t, err)
	fixture := &investmentsTestFixture{database: f.database, ownerUserID: f.ownerUserID, cashAccountID: f.cashAccountID, eurCommodityID: f.eurCommodityID, transactionService: f.importService.transactionService}
	checkpoint := reconcileCash(t, fixture, "2026-04-01", 100)
	staged := stageCSVIncome(t, f, profile, "2026-03-01;DIV;PAYER;85,00;100,00;15,00;one\n")
	before := csvIncomeDurableCount(t, f)
	preview, err := f.importService.PreviewCommit(ctx, PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Len(t, preview.ReconciliationIssues, 1)
	require.Equal(t, []int64{checkpoint}, preview.ReconciliationIssues[0].CheckpointIDs)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID, ReconciliationOverride: unboundOverride})
	require.NoError(t, err)
	require.Equal(t, 1, result.CommittedCount)
	require.Empty(t, activeCheckpointIDs(t, fixture))
}

func TestCSVInvestmentAutoDecimalDetectionIncludesGrossAndTax(t *testing.T) {
	t.Parallel()
	// Cash alone is ambiguous grouping; independent gross/tax decimals settle the file.
	profile := &ImportProfile{ID: 1, ConfigJSON: `{"delimiter":"semicolon","date_column":"Date","amount_column":"Cash","date_layout":"YMD","investment":{"kind":"dividend","instrument_column":"Code","gross_column":"Gross","withholding_column":"Tax"}}`}
	result, err := (&CSVAdapter{}).Parse(context.Background(), RawInput{Bytes: []byte("Date;Code;Cash;Gross;Tax\n2026-03-01;PAYER;1,000;1100.25;100.25\n")}, profile)
	require.NoError(t, err)
	require.Equal(t, "1000", result.Rows[0].Amount)
	require.Equal(t, "1100.25", result.Rows[0].Investment.GrossAmount)
	require.Equal(t, "100.25", result.Rows[0].Investment.WithholdingAmount)
}

func TestCSVInvestmentNoReferenceIdentityKeepsDistinctPayersAndOccurrences(t *testing.T) {
	t.Parallel()
	f, _, _, profile := csvIncomeFixture(t)
	_, err := f.investmentSvc.CreateInstrument(context.Background(), InvestmentInstrumentInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", ChangeReason: "fixture", CommodityCode: "OTHER", InstrumentType: "stock", DisplayName: "Other payer", QuantityScale: 0, PriceScale: 2,
	})
	require.NoError(t, err)
	rows := "2026-03-01;DIV;PAYER;85,00;100,00;15,00;\n2026-03-01;DIV;OTHER;85,00;100,00;15,00;\n2026-03-01;DIV;PAYER;85,00;100,00;15,00;\n"
	staged := stageCSVIncome(t, f, profile, rows)
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Equal(t, 3, result.CommittedCount)
	duplicate := stageCSVIncome(t, f, profile, rows)
	for _, row := range duplicate.Rows {
		require.Equal(t, "duplicate", row.DedupeStatus)
	}
}

func TestCSVInvestmentMappedPayeeIsReviewedAndRetained(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, _, _, profile := csvIncomeFixture(t)
	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(profile.ConfigJSON), &config))
	config["payee_column"] = "Payee"
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	mapped := string(encoded)
	profile, err = f.importService.UpdateImportProfile(ctx, UpdateImportProfileInput{OwnerUserID: f.ownerUserID, ProfileID: profile.ID, ConfigJSON: &mapped})
	require.NoError(t, err)
	staged, err := f.importService.StartImport(ctx, StartImportInput{OwnerUserID: f.ownerUserID, ProfileID: &profile.ID, Input: RawInput{Filename: "income.csv", Bytes: []byte("Date;Type;Code;Cash;Gross;Tax;Ref;Payee\n2026-03-01;DIV;PAYER;85,00;100,00;15,00;one;Payer broker\n")}})
	require.NoError(t, err)
	before := csvIncomeDurableCount(t, f)
	_, err = f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	payee, err := NewPayeeService(db.NewPayeeRepository(f.database), db.NewAccountRepository(f.database)).CreatePayee(ctx, CreatePayeeInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "payee.create", Name: "Payer broker", EffectiveFrom: "2026-01-01", ChangeReason: "fixture",
	})
	require.NoError(t, err)
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: staged.Batch.ID})
	require.NoError(t, err)
	require.Equal(t, 1, result.CommittedCount)
	stored, err := f.importRepo.ListAllImportStagedRows(ctx, staged.Batch.ID)
	require.NoError(t, err)
	transaction, err := f.importService.transactionService.Transaction(ctx, stored[0].CommittedTransactionID.Int64)
	require.NoError(t, err)
	require.Equal(t, &payee.ID, transaction.PayeeID)
}
