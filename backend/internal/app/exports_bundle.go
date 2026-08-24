package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"sort"
	"strconv"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// BundleSchemaVersion is the archive's own version, carried in manifest.json.
// Columns are appended within a version; a change that cannot be made by
// appending increments this and needs an ADR (ADR 0011).
const BundleSchemaVersion = 1

// bundleFile is one entry of the archive, recorded in the manifest with the
// checksum computed while it was written.
type bundleFile struct {
	Name   string `json:"name"`
	Rows   int64  `json:"rows"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// bundleManifest is what makes a scoped export reproducible and checkable. It
// is written last, because a checksum can only be taken after the file it
// describes exists.
type bundleManifest struct {
	SchemaVersion              int                  `json:"schema_version"`
	GeneratedAt                string               `json:"generated_at"`
	SelectionUnit              string               `json:"selection_unit"`
	RecordPolicy               string               `json:"record_policy"`
	Query                      ResolvedExportFilter `json:"query"`
	IncludesSystemAccounts     bool                 `json:"includes_system_accounts"`
	AllTransactionsComplete    bool                 `json:"all_transactions_complete"`
	IncompleteTransactionCount int64                `json:"incomplete_transaction_count"`
	OutOfScopePostingsIncluded int64                `json:"out_of_scope_postings_included"`
	Columns                    []string             `json:"ledger_columns"`
	Excluded                   []string             `json:"excluded"`
	Attachments                manifestAttachments  `json:"attachments"`
	Files                      []bundleFile         `json:"files"`
}

type manifestAttachments struct {
	Included  bool    `json:"included"`
	Directory *string `json:"directory"`
	Reason    string  `json:"reason"`
}

// countingHashWriter tracks size and checksum as an entry is written, so no
// file has to be re-read to describe it.
type countingHashWriter struct {
	out    io.Writer
	digest hash.Hash
	bytes  int64
}

func newCountingHashWriter(out io.Writer) *countingHashWriter {
	return &countingHashWriter{out: out, digest: sha256.New()}
}

func (w *countingHashWriter) Write(p []byte) (int, error) {
	written, err := w.out.Write(p)
	w.bytes += int64(written)
	if written > 0 {
		w.digest.Write(p[:written])
	}
	return written, err
}

func (w *countingHashWriter) checksum() string { return hex.EncodeToString(w.digest.Sum(nil)) }

// WriteBundle streams the archive: the ledger, the reference tables it joins
// to, a trial balance that reconciles it, and the manifest that describes all
// of them.
//
// It is the only scoped export. A flat CSV cannot carry a manifest, so a
// filtered request that arrives here comes back with its own description of
// what the filter resolved to and what it therefore contains.
func (s *ExportService) WriteBundle(ctx context.Context, out io.Writer, filter ExportFilter) error {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = snapshot.Rollback() }()

	accounts, err := s.repository.ExportAccounts(ctx, snapshot, BookID)
	if err != nil {
		return err
	}
	selection, resolved, err := s.resolveExportFilter(ctx, snapshot, filter, accounts)
	if err != nil {
		return err
	}
	totals, err := s.repository.LedgerExportTotals(ctx, snapshot, BookID, selection)
	if err != nil {
		return err
	}

	paths := accountExportPaths(accounts)
	archive := zip.NewWriter(out)
	manifest := bundleManifest{
		SchemaVersion:              BundleSchemaVersion,
		GeneratedAt:                s.now().UTC().Format(time.RFC3339),
		SelectionUnit:              ExportSelectionUnit,
		RecordPolicy:               ExportedRecordPolicy,
		Query:                      resolved,
		IncludesSystemAccounts:     true,
		AllTransactionsComplete:    totals.IncompleteTransactionCount == 0,
		IncompleteTransactionCount: totals.IncompleteTransactionCount,
		Columns:                    LedgerExportColumns,
		Excluded:                   ExportExcludedData,
		Attachments: manifestAttachments{
			Included:  false,
			Directory: nil,
			Reason:    "not implemented",
		},
	}

	addFile := func(name string, write func(io.Writer) (int64, error)) error {
		entry, err := archive.Create(name)
		if err != nil {
			return fmt.Errorf("create %s in export bundle: %w", name, err)
		}
		counted := newCountingHashWriter(entry)
		rows, err := write(counted)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, bundleFile{
			Name:   name,
			Rows:   rows,
			Bytes:  counted.bytes,
			SHA256: counted.checksum(),
		})
		return nil
	}

	if err := addFile("README.txt", func(w io.Writer) (int64, error) {
		return 0, writeBundleReadme(w, resolved)
	}); err != nil {
		return err
	}

	var outOfScope int64
	if err := addFile("ledger.csv", func(w io.Writer) (int64, error) {
		rows, offScope, err := s.writeBundleLedger(ctx, w, snapshot, selection, paths)
		outOfScope = offScope
		return rows, err
	}); err != nil {
		return err
	}
	manifest.OutOfScopePostingsIncluded = outOfScope

	dimensionFiles := []struct {
		name  string
		write func(io.Writer) (int64, error)
	}{
		{"accounts.csv", func(w io.Writer) (int64, error) { return writeAccountsCSV(w, accounts, paths) }},
		{"categories.csv", func(w io.Writer) (int64, error) { return writeCategoriesCSV(w, accounts, paths) }},
		{"payees.csv", func(w io.Writer) (int64, error) { return s.writePayeesCSV(ctx, w, snapshot) }},
		{"commodities.csv", func(w io.Writer) (int64, error) { return s.writeCommoditiesCSV(ctx, w, snapshot) }},
		{"tags.csv", func(w io.Writer) (int64, error) { return s.writeTagsCSV(ctx, w, snapshot) }},
		{"lots.csv", func(w io.Writer) (int64, error) { return s.writeLotsCSV(ctx, w, snapshot, paths) }},
		{"prices.csv", func(w io.Writer) (int64, error) { return s.writePricesCSV(ctx, w, snapshot) }},
		{"trial-balance.csv", func(w io.Writer) (int64, error) {
			return s.writeTrialBalanceCSV(ctx, w, snapshot, selection, paths)
		}},
	}
	for _, file := range dimensionFiles {
		if err := addFile(file.name, file.write); err != nil {
			return err
		}
	}

	// Last, so every checksum it carries already exists. A truncated download
	// therefore fails to open rather than looking complete.
	entry, err := archive.Create("manifest.json")
	if err != nil {
		return fmt.Errorf("create manifest in export bundle: %w", err)
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("write export manifest: %w", err)
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("close export bundle: %w", err)
	}

	return nil
}

// writeBundleLedger writes ledger.csv and counts the postings the archive holds
// only because their journal entry was selected — the ones a filter did not ask
// for and could not drop without unbalancing the entry.
func (s *ExportService) writeBundleLedger(ctx context.Context, out io.Writer, snapshot *sql.Tx, selection db.LedgerExportSelection, paths map[int64]string) (int64, int64, error) {
	if _, err := io.WriteString(out, utf8ByteOrderMark); err != nil {
		return 0, 0, fmt.Errorf("write export byte order mark: %w", err)
	}
	writer := csv.NewWriter(out)
	if err := writer.Write(LedgerExportColumns); err != nil {
		return 0, 0, fmt.Errorf("write export header: %w", err)
	}

	accountScope := idSet(selection.AccountIDs)
	commodityScope := idSet(selection.CommodityIDs)

	var rows, outOfScope int64
	err := s.repository.StreamLedgerPostings(ctx, snapshot, BookID, selection, func(record db.LedgerExportPostingRecord) error {
		if err := writer.Write(ledgerExportRow(record, paths)); err != nil {
			return fmt.Errorf("write export row: %w", err)
		}
		rows++
		if !postingIsInScope(record.AccountID, record.CommodityID, accountScope, commodityScope) {
			outOfScope++
		}
		return nil
	})
	if err != nil {
		return rows, outOfScope, err
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return rows, outOfScope, fmt.Errorf("flush export: %w", err)
	}

	return rows, outOfScope, nil
}

// postingIsInScope is the ADR 0011 definition, in one place: named by the
// filter in every restricted dimension, or unrestricted there.
func postingIsInScope(accountID int64, commodityID int64, accountScope map[int64]bool, commodityScope map[int64]bool) bool {
	if len(accountScope) > 0 && !accountScope[accountID] {
		return false
	}
	if len(commodityScope) > 0 && !commodityScope[commodityID] {
		return false
	}
	return true
}

func newBundleCSV(out io.Writer, header []string) (*csv.Writer, error) {
	if _, err := io.WriteString(out, utf8ByteOrderMark); err != nil {
		return nil, fmt.Errorf("write byte order mark: %w", err)
	}
	writer := csv.NewWriter(out)
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	return writer, nil
}

func finishBundleCSV(writer *csv.Writer, rows int64) (int64, error) {
	writer.Flush()
	if err := writer.Error(); err != nil {
		return rows, fmt.Errorf("flush bundle file: %w", err)
	}
	return rows, nil
}

func writeAccountsCSV(out io.Writer, accounts []db.ExportAccountRecord, paths map[int64]string) (int64, error) {
	writer, err := newBundleCSV(out, []string{
		"account_id", "account_path", "parent_account_id", "account_class", "account_kind",
		"name", "code", "system_role", "status", "opened_on", "closed_on",
		"institution", "default_commodity", "allows_postings",
	})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, account := range accounts {
		record := []string{
			strconv.FormatInt(account.AccountID, 10),
			paths[account.AccountID],
			nullableID(account.ParentAccountID),
			account.AccountClass,
			account.AccountKind,
			account.Name.String,
			account.Code.String,
			account.SystemRole.String,
			account.Status,
			account.OpenedOn,
			account.ClosedOn.String,
			account.InstitutionName.String,
			account.CommodityCode.String,
			boolToken(account.AllowsPostings),
		}
		if err := writer.Write(record); err != nil {
			return rows, fmt.Errorf("write account row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

// writeCategoriesCSV restates the income and expense accounts in category
// vocabulary. The overlap with accounts.csv is deliberate: a category *is* an
// account here, and this file carries the metadata the account row does not.
func writeCategoriesCSV(out io.Writer, accounts []db.ExportAccountRecord, paths map[int64]string) (int64, error) {
	writer, err := newBundleCSV(out, []string{
		"account_id", "category_path", "parent_account_id", "category_type",
		"name", "status", "is_builtin", "is_starter", "builtin_key",
	})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, account := range accounts {
		if account.AccountClass != "income" && account.AccountClass != "expense" {
			continue
		}
		categoryType := account.CategoryType.String
		if categoryType == "" {
			categoryType = account.AccountClass
		}
		record := []string{
			strconv.FormatInt(account.AccountID, 10),
			paths[account.AccountID],
			nullableID(account.ParentAccountID),
			categoryType,
			account.Name.String,
			account.Status,
			boolToken(account.CategoryBuiltin.Valid && account.CategoryBuiltin.Bool),
			boolToken(account.CategoryStarter.Valid && account.CategoryStarter.Bool),
			account.CategoryBuiltinKey.String,
		}
		if err := writer.Write(record); err != nil {
			return rows, fmt.Errorf("write category row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

func (s *ExportService) writePayeesCSV(ctx context.Context, out io.Writer, snapshot *sql.Tx) (int64, error) {
	payees, err := s.repository.ExportPayees(ctx, snapshot, BookID)
	if err != nil {
		return 0, err
	}
	writer, err := newBundleCSV(out, []string{"payee_id", "name", "status"})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, payee := range payees {
		if err := writer.Write([]string{strconv.FormatInt(payee.PayeeID, 10), payee.Name, payee.Status}); err != nil {
			return rows, fmt.Errorf("write payee row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

func (s *ExportService) writeCommoditiesCSV(ctx context.Context, out io.Writer, snapshot *sql.Tx) (int64, error) {
	commodities, err := s.repository.ExportCommodities(ctx, snapshot, BookID)
	if err != nil {
		return 0, err
	}
	writer, err := newBundleCSV(out, []string{
		"commodity_id", "code", "kind", "name", "symbol", "standard_scale", "max_quantity_scale", "status",
	})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, commodity := range commodities {
		record := []string{
			strconv.FormatInt(commodity.CommodityID, 10),
			commodity.Code,
			commodity.Kind,
			commodity.Name,
			commodity.Symbol,
			strconv.Itoa(commodity.StandardScale),
			strconv.Itoa(commodity.MaxQuantityScale),
			commodity.Status,
		}
		if err := writer.Write(record); err != nil {
			return rows, fmt.Errorf("write commodity row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

func (s *ExportService) writeTagsCSV(ctx context.Context, out io.Writer, snapshot *sql.Tx) (int64, error) {
	tags, err := s.repository.ExportTags(ctx, snapshot, BookID)
	if err != nil {
		return 0, err
	}
	writer, err := newBundleCSV(out, []string{"tag_id", "name", "kind", "status"})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, tag := range tags {
		if err := writer.Write([]string{strconv.FormatInt(tag.TagID, 10), tag.Name, tag.Kind, tag.Status}); err != nil {
			return rows, fmt.Errorf("write tag row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

func (s *ExportService) writeLotsCSV(ctx context.Context, out io.Writer, snapshot *sql.Tx, paths map[int64]string) (int64, error) {
	lots, err := s.repository.ExportLots(ctx, snapshot, BookID)
	if err != nil {
		return 0, err
	}
	writer, err := newBundleCSV(out, []string{
		"lot_id", "account_id", "account_path", "commodity_id", "opened_on", "status",
		"quantity", "remaining_quantity", "cost_basis", "remaining_cost_basis",
		"cost_commodity_id", "source_transaction_id",
	})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, lot := range lots {
		record := []string{
			strconv.FormatInt(lot.LotID, 10),
			strconv.FormatInt(lot.AccountID, 10),
			paths[lot.AccountID],
			strconv.FormatInt(lot.CommodityID, 10),
			lot.OpenedOn,
			lot.Status,
			exact.Decimal(lot.QuantityValue, lot.QuantityScale),
			exact.Decimal(lot.RemainingQuantityValue, lot.RemainingQuantityScale),
			exact.Decimal(exact.New(lot.CostBasisValue), lot.CostBasisScale),
			exact.Decimal(exact.New(lot.RemainingCostBasisValue), lot.RemainingCostBasisScale),
			strconv.FormatInt(lot.CostCommodityID, 10),
			nullableID(lot.SourceTransactionID),
		}
		if err := writer.Write(record); err != nil {
			return rows, fmt.Errorf("write lot row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

func (s *ExportService) writePricesCSV(ctx context.Context, out io.Writer, snapshot *sql.Tx) (int64, error) {
	prices, err := s.repository.ExportPrices(ctx, snapshot, BookID)
	if err != nil {
		return 0, err
	}
	writer, err := newBundleCSV(out, []string{
		"base_commodity_id", "quote_commodity_id", "valuation_date", "price",
		"base_quantity", "quote_type", "adjustment_basis", "is_manual", "is_derived", "source",
	})
	if err != nil {
		return 0, err
	}

	var rows int64
	for _, price := range prices {
		record := []string{
			strconv.FormatInt(price.BaseCommodityID, 10),
			strconv.FormatInt(price.QuoteCommodityID, 10),
			price.ValuationDate,
			exact.Decimal(exact.New(price.PriceValue), price.PriceScale),
			exact.Decimal(exact.New(price.BaseQuantityValue), price.BaseQuantityScale),
			price.QuoteType,
			price.AdjustmentBasis,
			boolToken(price.IsManual),
			boolToken(price.IsDerived),
			price.SourceCode.String,
		}
		if err := writer.Write(record); err != nil {
			return rows, fmt.Errorf("write price row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

// trialBalanceKey is one account and commodity — balances never cross either.
type trialBalanceKey struct {
	accountID   int64
	commodityID int64
}

// trialBalanceRow accumulates the five independent figures the archive states.
// Everything is folded in Go through exact.ScaledInt: coefficients are strings
// in SQLite and summing them there would be a float in disguise.
type trialBalanceRow struct {
	opening       *exact.ScaledInt
	exportedIn    *exact.ScaledInt
	exportedOut   *exact.ScaledInt
	excludedIn    *exact.ScaledInt
	actualClosing *exact.ScaledInt
}

func newTrialBalanceRow() *trialBalanceRow {
	return &trialBalanceRow{
		opening:       exact.NewScaledInt(),
		exportedIn:    exact.NewScaledInt(),
		exportedOut:   exact.NewScaledInt(),
		excludedIn:    exact.NewScaledInt(),
		actualClosing: exact.NewScaledInt(),
	}
}

// writeTrialBalanceCSV states, per account and commodity, what this archive can
// justify and what it cannot.
//
// Two things force more than one closing figure. Entry-complete selection pulls
// counterpart postings from accounts the filter never named, so their exported
// movement is only part of their real movement. And transaction-basis selection
// exports entries dated outside the range, which a single closing figure would
// double-count or omit. Hence: the selection basis decides which rows are in the
// file; balances are always entry-date arithmetic (ADR 0011).
func (s *ExportService) writeTrialBalanceCSV(ctx context.Context, out io.Writer, snapshot *sql.Tx, selection db.LedgerExportSelection, paths map[int64]string) (int64, error) {
	accountScope := idSet(selection.AccountIDs)
	commodityScope := idSet(selection.CommodityIDs)

	rowsByKey := map[trialBalanceKey]*trialBalanceRow{}
	err := s.repository.StreamTrialBalancePostings(ctx, snapshot, BookID, selection, func(record db.TrialBalancePostingRecord) error {
		key := trialBalanceKey{accountID: record.AccountID, commodityID: record.CommodityID}
		row := rowsByKey[key]
		if row == nil {
			row = newTrialBalanceRow()
			rowsByKey[key] = row
		}

		beforeRange := selection.From != "" && record.EntryDate < selection.From
		afterRange := selection.To != "" && record.EntryDate > selection.To
		inRange := !beforeRange && !afterRange

		if beforeRange {
			row.opening.AddCoefficient(record.QuantityValue, record.QuantityScale)
		}
		if !afterRange {
			row.actualClosing.AddCoefficient(record.QuantityValue, record.QuantityScale)
		}
		switch {
		case record.Selected && inRange:
			row.exportedIn.AddCoefficient(record.QuantityValue, record.QuantityScale)
		case record.Selected:
			row.exportedOut.AddCoefficient(record.QuantityValue, record.QuantityScale)
		case inRange:
			row.excludedIn.AddCoefficient(record.QuantityValue, record.QuantityScale)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	writer, err := newBundleCSV(out, []string{
		"account_id", "account_path", "commodity_id", "in_scope",
		"opening_balance", "exported_in_range_movement", "exported_out_of_range_movement",
		"excluded_in_range_movement", "derived_closing_balance", "actual_closing_balance",
	})
	if err != nil {
		return 0, err
	}

	keys := make([]trialBalanceKey, 0, len(rowsByKey))
	for key := range rowsByKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].accountID != keys[j].accountID {
			return keys[i].accountID < keys[j].accountID
		}
		return keys[i].commodityID < keys[j].commodityID
	})

	var rows int64
	for _, key := range keys {
		row := rowsByKey[key]

		// derived is what this archive's in-range content alone justifies. It
		// is never called the account's closing balance, because for a
		// counterpart account it is not one.
		derived := exact.NewScaledInt()
		derived.AddScaled(row.opening)
		derived.AddScaled(row.exportedIn)

		// Each figure is rendered at the scale it accumulated at, and two
		// figures of one row may therefore differ — a column that never
		// received a posting reads "0" where its neighbour reads "0.00". This
		// mirrors the decision R2 reached for cashflow measures and reverted a
		// common-scale restatement over: deepening a value costs digits against
		// the 38-digit ceiling and can overflow a figure that was
		// representable as recorded. The values are exact either way, and
		// README.txt tells the reader so.
		figures := make([]string, 0, 6)
		for _, value := range []*exact.ScaledInt{
			row.opening, row.exportedIn, row.exportedOut, row.excludedIn, derived, row.actualClosing,
		} {
			rendered, err := renderScaled(value)
			if err != nil {
				return rows, err
			}
			figures = append(figures, rendered)
		}

		record := append([]string{
			strconv.FormatInt(key.accountID, 10),
			paths[key.accountID],
			strconv.FormatInt(key.commodityID, 10),
			boolToken(postingIsInScope(key.accountID, key.commodityID, accountScope, commodityScope)),
		}, figures...)
		if err := writer.Write(record); err != nil {
			return rows, fmt.Errorf("write trial balance row: %w", err)
		}
		rows++
	}

	return finishBundleCSV(writer, rows)
}

// renderScaled turns an accumulated figure into the export's decimal text,
// refusing rather than truncating when it exceeds the exact coefficient limit.
func renderScaled(value *exact.ScaledInt) (string, error) {
	coefficient, err := value.Coefficient()
	if err != nil {
		return "", fmt.Errorf("render trial balance figure: %w", err)
	}
	return exact.Decimal(coefficient, value.Scale()), nil
}

// writeBundleReadme explains the archive to whoever opens it in a year, in
// prose rather than in field names. It states the guarantees, and just as
// deliberately states what the archive does *not* justify: an unexplained
// number is worse than an absent one.
func writeBundleReadme(out io.Writer, filter ResolvedExportFilter) error {
	sections := []string{
		`REKENRAAM LEDGER EXPORT`,
		`This archive is a portable copy of a double-entry ledger. Every file is
UTF-8 CSV with a byte order mark, LF line endings, RFC 4180 quoting, and a
header row first. Amounts are exact decimal strings: point separator, no group
separators, debit-positive, written at the scale they were recorded at. No
value in this archive was ever a floating-point number.`,
		`WHAT IS HERE
  ledger.csv         one row per posting, the ledger itself
  accounts.csv       every account, all classes, with its full path
  categories.csv     the income and expense accounts restated as categories
  payees.csv         payees by id
  commodities.csv    currencies, securities, and crypto, with their scales
  tags.csv           tag names behind the tag columns in ledger.csv
  lots.csv           investment lots as they currently stand
  prices.csv         non-voided price observations
  trial-balance.csv  balances that let you check ledger.csv against the book
  manifest.json      what this export is, what it contains, and its checksums`,
		`THE BALANCE GUARANTEE
Group ledger.csv by journal_entry_id and commodity: each group sums to zero.
That holds under every filter, because a filter selects whole journal entries
and never individual postings. Grouping by transaction_id also sums to zero for
every row whose transaction_complete column reads true; where it reads false,
this archive holds only part of that transaction.
System accounts (commodity_trading and its siblings) are included. Reports in
the app exclude them; an export that dropped them would emit transactions that
do not balance.`,
		`READING trial-balance.csv
  opening_balance                 the account's real balance the day before the
                                  range started, from all history
  exported_in_range_movement      the sum of this archive's rows for that
                                  account and commodity, inside the range
  exported_out_of_range_movement  rows this archive carries from outside it
                                  (only ever non-zero on transaction basis)
  excluded_in_range_movement      in-range movement the filter left out
  derived_closing_balance         opening_balance + exported_in_range_movement:
                                  what this archive alone can justify
  actual_closing_balance          the account's real balance at the range end
Two identities hold exactly:
  opening_balance + exported_in_range_movement = derived_closing_balance
  derived_closing_balance + excluded_in_range_movement = actual_closing_balance
Each figure is written at the scale it was accumulated at, so one row may read
"0" in one column and "0.00" in the next. Both are exact zero; the values are
never restated at a common scale, because deepening a figure costs digits
against the exact 38-digit limit.
The derived figure is NOT the account's closing balance. For an account present
only as a counterpart to something you filtered for, it is a partial figure by
construction. opening_balance, excluded_in_range_movement, and
actual_closing_balance are stated from the book, not justified by this archive.`,
		`ACCOUNTS AND CATEGORIES OVERLAP ON PURPOSE
A category is an account here: an account whose class is income or expense.
accounts.csv is the complete set; categories.csv restates that subset with the
metadata only categories carry. Do not add the two together.`,
		`WHAT THIS ARCHIVE DOES NOT CONTAIN
Credentials and secrets, authentication sessions and multi-factor enrolment,
audit and authentication events, import profiles and batches, connection
configuration, background work, and superseded, draft, or voided transaction
versions. This is an export of the ledger, not a copy of the installation. The
audit-complete record is the SQLite backup.
Attachments are not included; they are not implemented yet, and manifest.json
says so rather than leaving you to assume either way.
lots.csv is a statement of current lot state, not a replayable event log: cost
basis cannot be reconstructed from postings alone, which is why it is here.`,
	}

	if filter.From != "" || filter.To != "" || len(filter.AccountIDs) > 0 || len(filter.CommodityIDs) > 0 {
		scope := "THIS EXPORT IS FILTERED\nSee manifest.json for the exact query and what it resolved to."
		for _, note := range filter.Notes {
			scope += "\n  - " + note
		}
		sections = append(sections, scope)
	}

	for _, section := range sections {
		if _, err := io.WriteString(out, section+"\n\n"); err != nil {
			return fmt.Errorf("write export readme: %w", err)
		}
	}

	return nil
}
