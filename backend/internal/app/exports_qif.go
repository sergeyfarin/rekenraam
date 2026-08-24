package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// QIF is a lossy legacy format, and pretending otherwise is how EU users lost
// 100× amounts on import (T-35, T-36). What follows is deliberate about every
// place the format cannot carry what the ledger holds.

// ErrQIFSelectionUnsupported means the request names accounts QIF cannot
// express, and the caller has not acknowledged the omissions.
var ErrQIFSelectionUnsupported = errors.New("qif selection contains unsupported accounts")

// ErrQIFNothingToExport means nothing in the selection can be written at all.
var ErrQIFNothingToExport = errors.New("qif selection contains no exportable account")

// QIFDateLayout is how a file writes its dates. QIF carries no declaration of
// this, which is the whole problem: the layout must be stated out of band, and
// is — in the filename, and for an archive also in the manifest and the README.
type QIFDateLayout string

const (
	QIFDateLayoutMDY QIFDateLayout = "mdy"
	QIFDateLayoutDMY QIFDateLayout = "dmy"
)

// qifTypeByBaseKind maps the ledger's account vocabulary onto QIF's five
// account types. base_kind rather than account_kind, so a new kind inherits its
// family's mapping instead of silently becoming unsupported.
var qifTypeByBaseKind = map[string]string{
	"cash":             "Cash",
	"bank_account":     "Bank",
	"investment_cash":  "Bank",
	"revolving_credit": "CCard",
	"tangible_asset":   "Oth A",
	"non_cash_balance": "Oth A",
	"receivable":       "Oth A",
	"loan":             "Oth L",
	"payable":          "Oth L",
}

// qifUnsupportedReason explains, in a code the UI can translate and a human can
// read, why an account cannot be written as QIF.
const (
	qifReasonNotABalanceAccount = "not_a_balance_account"
	qifReasonInvestmentAccount  = "investment_account"
	qifReasonNonCurrency        = "non_currency_commodity"
	qifReasonNoCommodity        = "no_single_commodity"
)

// QIFAccountStatus is one account's verdict: either it gets a file, or it is
// named and skipped. It is never silently dropped.
type QIFAccountStatus struct {
	AccountID   int64  `json:"account_id"`
	AccountPath string `json:"account_path"`
	QIFType     string `json:"qif_type,omitempty"`
	Supported   bool   `json:"supported"`
	Reason      string `json:"reason,omitempty"`
}

// QIFSelection is what a QIF request resolves to before anything is written.
type QIFSelection struct {
	Supported   []QIFAccountStatus
	Unsupported []QIFAccountStatus
	Layout      QIFDateLayout
}

// classifyQIFAccounts decides which accounts can be written, and why the others
// cannot.
//
// Investment containers, security holdings, and crypto wallets are excluded in
// R3: !Type:Invst semantics vary per reader and would silently misstate cost
// basis. The CSV bundle is the lossless investment path.
func classifyQIFAccounts(accounts []db.ExportAccountRecord, paths map[int64]string, requested []int64) QIFSelection {
	wanted := idSet(requested)

	var selection QIFSelection
	for _, account := range accounts {
		if len(wanted) > 0 && !wanted[account.AccountID] {
			continue
		}

		status := QIFAccountStatus{AccountID: account.AccountID, AccountPath: paths[account.AccountID]}
		switch {
		case account.AccountClass != "asset" && account.AccountClass != "liability":
			// Categories and equity are not QIF accounts; they appear inside a
			// record's category field instead.
			status.Reason = qifReasonNotABalanceAccount
		case account.BaseKind == "investment_container" || account.BaseKind == "security_holding" || account.BaseKind == "digital_asset":
			status.Reason = qifReasonInvestmentAccount
		case !account.CommodityCode.Valid || strings.TrimSpace(account.CommodityCode.String) == "":
			// Without a single commodity a QIF file's amounts would mean
			// different things on different lines.
			status.Reason = qifReasonNoCommodity
		case account.CommodityKind.String != "currency":
			status.Reason = qifReasonNonCurrency
		default:
			qifType, ok := qifTypeByBaseKind[account.BaseKind]
			if !ok {
				qifType = "Oth A"
				if account.AccountClass == "liability" {
					qifType = "Oth L"
				}
			}
			status.Supported = true
			status.QIFType = qifType
		}

		if status.Supported {
			selection.Supported = append(selection.Supported, status)
			continue
		}
		// An account nobody asked for is not an omission worth reporting: a
		// whole-book export would otherwise "exclude" every category in the
		// ledger. Only a named account gets reported as excluded.
		if len(wanted) > 0 {
			selection.Unsupported = append(selection.Unsupported, status)
		}
	}

	return selection
}

// ResolveQIFSelection reports what a QIF export would and would not write,
// without writing it. The preview and the download share it, so the list a
// reader confirms is the list the download honours.
func (s *ExportService) ResolveQIFSelection(ctx context.Context, filter ExportFilter, layout string) (QIFSelection, error) {
	snapshot, err := s.repository.Snapshot(ctx)
	if err != nil {
		return QIFSelection{}, err
	}
	defer func() { _ = snapshot.Rollback() }()

	accounts, err := s.repository.ExportAccounts(ctx, snapshot, BookID)
	if err != nil {
		return QIFSelection{}, err
	}
	selection, _, err := s.resolveExportFilter(ctx, snapshot, filter, accounts)
	if err != nil {
		return QIFSelection{}, err
	}
	resolvedLayout, err := cleanQIFDateLayout(layout)
	if err != nil {
		return QIFSelection{}, err
	}

	classified := classifyQIFAccounts(accounts, accountExportPaths(accounts), selection.AccountIDs)
	classified.Layout = resolvedLayout
	return classified, nil
}

func cleanQIFDateLayout(value string) (QIFDateLayout, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", string(QIFDateLayoutMDY):
		return QIFDateLayoutMDY, nil
	case string(QIFDateLayoutDMY):
		return QIFDateLayoutDMY, nil
	default:
		return "", ValidationError{Message: "qif_date_layout must be mdy or dmy"}
	}
}

// QIFExportResult is what the writer produced, so a handler can name the file
// correctly without re-deriving the decision.
type QIFExportResult struct {
	Filename    string
	ContentType string
}

// WriteQIF writes either one .qif file or an archive of them.
//
// A bare file comes back only when all three hold: one account was requested,
// nothing was omitted, and the written file contains at least one decisive date
// (a day past the 12th) to anchor its layout. Otherwise the response is an
// archive, whose manifest and README state the layout that a bare file could
// only carry in its name. An ambiguous-only file has no in-band way to say how
// it was written, so it is never handed over alone.
func (s *ExportService) WriteQIF(
	ctx context.Context,
	out io.Writer,
	filter ExportFilter,
	layout string,
	allowPartial bool,
	onDecided func(QIFExportResult),
) error {
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
	resolvedLayout, err := cleanQIFDateLayout(layout)
	if err != nil {
		return err
	}

	paths := accountExportPaths(accounts)
	classified := classifyQIFAccounts(accounts, paths, selection.AccountIDs)
	classified.Layout = resolvedLayout

	if len(classified.Unsupported) > 0 && !allowPartial {
		return ErrQIFSelectionUnsupported
	}
	if len(classified.Supported) == 0 {
		return ErrQIFNothingToExport
	}

	stamp := s.now().UTC().Format("20060102")

	archiveResult := QIFExportResult{
		Filename:    "rekenraam-qif-" + stamp + ".zip",
		ContentType: "application/zip",
	}

	// One account and nothing omitted: render it before deciding, because
	// whether it may travel as a bare file depends on what its dates turned out
	// to be. The decision is announced through onDecided rather than returned,
	// so a handler can attach the right filename and content type *before* the
	// first byte reaches the client — the shape is not knowable afterwards.
	if len(classified.Supported) == 1 && len(classified.Unsupported) == 0 {
		account := classified.Supported[0]
		var buffer bytes.Buffer
		file, err := s.writeQIFAccount(ctx, &buffer, snapshot, selection, account, resolvedLayout, accounts)
		if err != nil {
			return err
		}
		if file.decisive {
			onDecided(QIFExportResult{
				Filename:    "rekenraam-" + qifSlug(account.AccountPath) + "-" + string(resolvedLayout) + "-" + stamp + ".qif",
				ContentType: "application/qif",
			})
			if _, err := out.Write(buffer.Bytes()); err != nil {
				return fmt.Errorf("write qif: %w", err)
			}
			return nil
		}

		onDecided(archiveResult)
		return s.writeQIFArchive(ctx, out, snapshot, selection, classified, resolved, accounts, map[int64][]byte{
			account.AccountID: buffer.Bytes(),
		})
	}

	onDecided(archiveResult)
	return s.writeQIFArchive(ctx, out, snapshot, selection, classified, resolved, accounts, nil)
}

// qifFileStats is what one written account file turned out to contain.
type qifFileStats struct {
	records int64
	// decisive is true when some date in the file has a day past the 12th,
	// which forces any reader's layout detection to agree with what was
	// written. Without one, the file's layout is undecidable from its content.
	decisive bool
}

func (s *ExportService) writeQIFArchive(
	ctx context.Context,
	out io.Writer,
	snapshot *sql.Tx,
	selection db.LedgerExportSelection,
	classified QIFSelection,
	resolved ResolvedExportFilter,
	accounts []db.ExportAccountRecord,
	prerendered map[int64][]byte,
) error {
	archive := zip.NewWriter(out)

	manifest := struct {
		SchemaVersion int                  `json:"schema_version"`
		GeneratedAt   string               `json:"generated_at"`
		Format        string               `json:"format"`
		DateLayout    string               `json:"date_layout"`
		DecimalMark   string               `json:"decimal_mark"`
		Query         ResolvedExportFilter `json:"query"`
		Included      []QIFAccountStatus   `json:"included_accounts"`
		Excluded      []QIFAccountStatus   `json:"excluded_accounts"`
		Files         []bundleFile         `json:"files"`
	}{
		SchemaVersion: BundleSchemaVersion,
		GeneratedAt:   s.now().UTC().Format(time.RFC3339),
		Format:        "qif",
		DateLayout:    string(classified.Layout),
		DecimalMark:   ".",
		Query:         resolved,
		Included:      classified.Supported,
		Excluded:      classified.Unsupported,
	}

	readme, err := archive.Create("README.txt")
	if err != nil {
		return fmt.Errorf("create qif readme: %w", err)
	}
	if err := writeQIFReadme(readme, classified); err != nil {
		return err
	}

	for _, account := range classified.Supported {
		name := qifSlug(account.AccountPath) + "-" + strconv.FormatInt(account.AccountID, 10) + "-" + string(classified.Layout) + ".qif"
		entry, err := archive.Create(name)
		if err != nil {
			return fmt.Errorf("create %s in qif archive: %w", name, err)
		}
		counted := newCountingHashWriter(entry)

		var stats qifFileStats
		if rendered, ok := prerendered[account.AccountID]; ok {
			if _, err := counted.Write(rendered); err != nil {
				return fmt.Errorf("write %s: %w", name, err)
			}
		} else {
			stats, err = s.writeQIFAccount(ctx, counted, snapshot, selection, account, classified.Layout, accounts)
			if err != nil {
				return err
			}
		}

		manifest.Files = append(manifest.Files, bundleFile{
			Name:   name,
			Rows:   stats.records,
			Bytes:  counted.bytes,
			SHA256: counted.checksum(),
		})
	}

	entry, err := archive.Create("manifest.json")
	if err != nil {
		return fmt.Errorf("create qif manifest: %w", err)
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("write qif manifest: %w", err)
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("close qif archive: %w", err)
	}

	return nil
}

// writeQIFAccount writes one account's transactions.
//
// Postings arrive ordered by entry, so an entry's postings are contiguous: the
// ones in this account become records, and the rest become the record's
// category, transfer, or splits.
func (s *ExportService) writeQIFAccount(
	ctx context.Context,
	out io.Writer,
	snapshot *sql.Tx,
	selection db.LedgerExportSelection,
	account QIFAccountStatus,
	layout QIFDateLayout,
	accounts []db.ExportAccountRecord,
) (qifFileStats, error) {
	classByAccount := map[int64]string{}
	systemAccounts := map[int64]bool{}
	for _, record := range accounts {
		classByAccount[record.AccountID] = record.AccountClass
		if record.SystemRole.Valid && strings.TrimSpace(record.SystemRole.String) != "" {
			systemAccounts[record.AccountID] = true
		}
	}
	paths := accountExportPaths(accounts)

	// The file holds this account's entries: the user's own date and commodity
	// scope, narrowed to entries this account takes part in.
	accountSelection := selection
	accountSelection.AccountIDs = []int64{account.AccountID}

	var stats qifFileStats
	if _, err := io.WriteString(out, "!Type:"+account.QIFType+"\n"); err != nil {
		return stats, fmt.Errorf("write qif header: %w", err)
	}

	var entry []db.LedgerExportPostingRecord
	flush := func() error {
		if len(entry) == 0 {
			return nil
		}
		records, decisive, err := writeQIFEntry(out, entry, account, layout, classByAccount, systemAccounts, paths)
		if err != nil {
			return err
		}
		stats.records += records
		stats.decisive = stats.decisive || decisive
		entry = entry[:0]
		return nil
	}

	err := s.repository.StreamLedgerPostings(ctx, snapshot, BookID, accountSelection, func(record db.LedgerExportPostingRecord) error {
		if len(entry) > 0 && entry[0].JournalEntryID != record.JournalEntryID {
			if err := flush(); err != nil {
				return err
			}
		}
		entry = append(entry, record)
		return nil
	})
	if err != nil {
		return stats, err
	}
	if err := flush(); err != nil {
		return stats, err
	}

	return stats, nil
}

// writeQIFEntry turns one journal entry into this account's records.
func writeQIFEntry(
	out io.Writer,
	entry []db.LedgerExportPostingRecord,
	account QIFAccountStatus,
	layout QIFDateLayout,
	classByAccount map[int64]string,
	systemAccounts map[int64]bool,
	paths map[int64]string,
) (int64, bool, error) {
	var own, counterparts []db.LedgerExportPostingRecord
	for _, posting := range entry {
		if posting.AccountID == account.AccountID {
			own = append(own, posting)
			continue
		}
		counterparts = append(counterparts, posting)
	}
	if len(own) == 0 {
		return 0, false, nil
	}

	var records int64
	var decisive bool
	for _, posting := range own {
		lines, err := qifRecordLines(posting, own, counterparts, layout, classByAccount, systemAccounts, paths)
		if err != nil {
			return records, decisive, err
		}
		if _, err := io.WriteString(out, strings.Join(lines, "\n")+"\n^\n"); err != nil {
			return records, decisive, fmt.Errorf("write qif record: %w", err)
		}
		records++
		decisive = decisive || qifDateIsDecisive(posting.EntryDate)
	}

	return records, decisive, nil
}

func qifRecordLines(
	posting db.LedgerExportPostingRecord,
	own []db.LedgerExportPostingRecord,
	counterparts []db.LedgerExportPostingRecord,
	layout QIFDateLayout,
	classByAccount map[int64]string,
	systemAccounts map[int64]bool,
	paths map[int64]string,
) ([]string, error) {
	date, err := formatQIFDate(posting.EntryDate, layout)
	if err != nil {
		return nil, err
	}

	// Amounts always use a point and never a group separator: the absence of
	// grouping is what keeps a written amount unambiguous to any detector,
	// ours included.
	amount := exact.Decimal(posting.QuantityValue, posting.QuantityScale)

	lines := []string{"D" + date, "T" + amount, "U" + amount}
	if payee := qifField(posting.PayeeName); payee != "" {
		lines = append(lines, "P"+payee)
	}

	memo := qifField(posting.PostingMemo)
	if memo == "" {
		memo = qifField(posting.EntryMemo)
	}
	if memo == "" {
		memo = qifField(posting.Description)
	}

	if reference := qifField(posting.ExternalRefHint.String); reference != "" {
		lines = append(lines, "N"+reference)
	}
	switch posting.ReconciliationStatus {
	case "cleared":
		lines = append(lines, "C*")
	case "reconciled":
		lines = append(lines, "CX")
	}

	foreign := foreignCounterparts(posting, counterparts, systemAccounts)
	if primary, ok := primaryCounterpart(posting, counterparts, systemAccounts); ok {
		lines = append(lines, "L"+qifCounterpartLabel(primary, classByAccount, paths))
	}

	switch {
	case len(foreign) > 0:
		// An exchange: the other side is in a different commodity, so it cannot
		// be summed into this record. State it rather than inventing a rate.
		memo = appendQIFNote(memo, "other side: "+qifForeignSummary(foreign))
	case len(own) > 1:
		// Several postings of this account in one entry: each is its own
		// record, so attributing the shared counterparts to any one of them as
		// split amounts would double-count them.
		memo = appendQIFNote(memo, "one of several postings in this entry")
	}

	if memo != "" {
		lines = append(lines, "M"+memo)
	}

	// Splits only where they are honest: several counterparts, all in this
	// account's commodity, and one posting of ours to attribute them to.
	if len(counterparts) > 1 && len(foreign) == 0 && len(own) == 1 {
		for _, counterpart := range counterparts {
			lines = append(lines, "S"+qifCounterpartLabel(counterpart, classByAccount, paths))
			if splitMemo := qifField(counterpart.PostingMemo); splitMemo != "" {
				lines = append(lines, "E"+splitMemo)
			}
			// The split carries this account's side of that counterpart, so the
			// splits sum to T exactly — the entry balances, so their negation
			// must.
			negated := exact.NewScaledInt()
			negated.AddCoefficient(counterpart.QuantityValue, counterpart.QuantityScale)
			lines = append(lines, "$"+renderScaled(negated.Negated()))
		}
	}

	return lines, nil
}

// primaryCounterpart picks the leg a human would call the other side.
//
// An exchange balances per commodity through commodity_trading, so an entry's
// first counterpart is often a system account carrying the same currency —
// true, and useless as a label. The real other side is the account the money
// ended up in, which is the non-system leg in the other commodity.
func primaryCounterpart(
	posting db.LedgerExportPostingRecord,
	counterparts []db.LedgerExportPostingRecord,
	systemAccounts map[int64]bool,
) (db.LedgerExportPostingRecord, bool) {
	for _, counterpart := range counterparts {
		if !systemAccounts[counterpart.AccountID] && counterpart.CommodityID != posting.CommodityID {
			return counterpart, true
		}
	}
	for _, counterpart := range counterparts {
		if !systemAccounts[counterpart.AccountID] {
			return counterpart, true
		}
	}
	if len(counterparts) > 0 {
		return counterparts[0], true
	}
	return db.LedgerExportPostingRecord{}, false
}

// foreignCounterparts are the legs in another commodity — the ones QIF has no
// way to express in this file's currency. A system leg (commodity_trading) is
// the ledger's own bookkeeping, not a side of the exchange a reader cares
// about, so it is left out of the summary while still being ignored for splits.
func foreignCounterparts(
	posting db.LedgerExportPostingRecord,
	counterparts []db.LedgerExportPostingRecord,
	systemAccounts map[int64]bool,
) []db.LedgerExportPostingRecord {
	var foreign, systemForeign []db.LedgerExportPostingRecord
	for _, counterpart := range counterparts {
		if counterpart.CommodityID == posting.CommodityID {
			continue
		}
		if systemAccounts[counterpart.AccountID] {
			systemForeign = append(systemForeign, counterpart)
			continue
		}
		foreign = append(foreign, counterpart)
	}
	if len(foreign) == 0 {
		return systemForeign
	}
	return foreign
}

func qifForeignSummary(foreign []db.LedgerExportPostingRecord) string {
	parts := make([]string, 0, len(foreign))
	for _, posting := range foreign {
		parts = append(parts, exact.Decimal(posting.QuantityValue, posting.QuantityScale)+" "+posting.CommodityCode)
	}
	return strings.Join(parts, ", ")
}

// qifCounterpartLabel writes a category as its path and a balance account in
// QIF's transfer form, which is what tells a reader the money moved rather than
// was spent.
func qifCounterpartLabel(counterpart db.LedgerExportPostingRecord, classByAccount map[int64]string, paths map[int64]string) string {
	path := paths[counterpart.AccountID]
	switch classByAccount[counterpart.AccountID] {
	case "income", "expense":
		return path
	default:
		return "[" + path + "]"
	}
}

func appendQIFNote(memo string, note string) string {
	if memo == "" {
		return note
	}
	return memo + " (" + note + ")"
}

// qifField strips the newlines a QIF field cannot contain: every line is a
// field, so an embedded newline would silently become a different record.
func qifField(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.TrimSpace(value)
}

func formatQIFDate(isoDate string, layout QIFDateLayout) (string, error) {
	parsed, err := time.Parse(time.DateOnly, isoDate)
	if err != nil {
		return "", fmt.Errorf("format qif date %q: %w", isoDate, err)
	}
	if layout == QIFDateLayoutDMY {
		return parsed.Format("02/01/2006"), nil
	}
	return parsed.Format("01/02/2006"), nil
}

// qifDateIsDecisive reports whether this date forces a reader's layout
// detection to agree with what was written: a day past the 12th cannot be a
// month, so the ambiguity disappears.
func qifDateIsDecisive(isoDate string) bool {
	parsed, err := time.Parse(time.DateOnly, isoDate)
	if err != nil {
		return false
	}
	return parsed.Day() > 12
}

var qifSlugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func qifSlug(path string) string {
	slug := qifSlugUnsafe.ReplaceAllString(strings.ToLower(path), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "account"
	}
	if len(slug) > 60 {
		slug = strings.Trim(slug[:60], "-")
	}
	return slug
}

func writeQIFReadme(out io.Writer, selection QIFSelection) error {
	var builder strings.Builder
	builder.WriteString("REKENRAAM QIF EXPORT\n\n")
	builder.WriteString("Dates in these files are written as ")
	if selection.Layout == QIFDateLayoutDMY {
		builder.WriteString("DD/MM/YYYY")
	} else {
		builder.WriteString("MM/DD/YYYY")
	}
	builder.WriteString(".\n")
	builder.WriteString(`QIF has no way to declare that, which is why it is stated here, in
manifest.json, and in every filename. If you import these files somewhere else,
set that layout explicitly. Amounts always use a point as the decimal mark and
never a group separator; reading them with a comma-decimal setting will be
wrong, and for amounts with three decimal places it will be wrong silently.

WHAT QIF CANNOT CARRY
One file per account, each in that account's own currency. A transfer appears
as [Account Name]; a category appears by its full path. An entry whose other
side is in a different currency is written with that side stated in the memo
rather than converted, because inventing a rate would be worse than being
explicit. Investment accounts are not written at all — !Type:Invst means
different things to different readers and would misstate cost basis. Use the
CSV bundle for anything involving securities or crypto.

`)

	if len(selection.Unsupported) > 0 {
		builder.WriteString("ACCOUNTS NOT WRITTEN\n")
		for _, account := range selection.Unsupported {
			builder.WriteString("  " + account.AccountPath + " — " + account.Reason + "\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("ACCOUNTS WRITTEN\n")
	for _, account := range selection.Supported {
		builder.WriteString("  " + account.AccountPath + " (!Type:" + account.QIFType + ")\n")
	}

	if _, err := io.WriteString(out, builder.String()); err != nil {
		return fmt.Errorf("write qif readme: %w", err)
	}
	return nil
}
