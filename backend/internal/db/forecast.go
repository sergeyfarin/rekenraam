package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

// ForecastRepository owns the read-only database access used by the projected
// balances feature. It deliberately accepts the application-wide read-only
// pool: a forecast must never hold the writer's sole connection.
type ForecastRepository struct {
	readOnly *sql.DB
}

func NewForecastRepository(readOnly *sql.DB) *ForecastRepository {
	return &ForecastRepository{readOnly: readOnly}
}

// ForecastSnapshotLimits are hard input limits, before any projection result is
// built. Each query reads one additional row to distinguish a complete result
// from a plausible-looking prefix.
type ForecastSnapshotLimits struct {
	VersionRows                 int
	PostedPostingRows           int
	RelevantTemplates           int
	OccurrenceRows              int
	TemplateAndDraftPostingRows int
}

func DefaultForecastSnapshotLimits() ForecastSnapshotLimits {
	return ForecastSnapshotLimits{
		VersionRows:                 100000,
		PostedPostingRows:           1000000,
		RelevantTemplates:           2000,
		OccurrenceRows:              100000,
		TemplateAndDraftPostingRows: 250000,
	}
}

var ErrForecastInputTooLarge = errors.New("forecast input exceeds a configured limit")

// ForecastSnapshotRequest is normalized by a later application-service slice.
// AccountIDs are already the resolved posting accounts. An empty list is a
// real empty scope and is never widened into a whole-book query.
type ForecastSnapshotRequest struct {
	BookID      int64
	OwnerUserID int64
	AccountIDs  []int64
	ThroughDate string
	Limits      ForecastSnapshotLimits
}

// ForecastSnapshot contains raw, complete records. It intentionally does not
// contain balances, selected scope decisions, recurrence enumeration, or any
// business result; those belong to the application service in slice 2.
type ForecastSnapshot struct {
	TimeZone          string
	AccountVersions   []ForecastAccountVersionRecord
	CommodityVersions []ForecastCommodityVersionRecord
	PostedPostings    []ForecastPostingRecord
	Templates         []ForecastTemplateRecord
	TemplatePostings  []ForecastTemplatePostingRecord
	Occurrences       []ForecastOccurrenceRecord
	DraftPostings     []ForecastDraftPostingRecord
	PayeeNames        map[int64]string
}

type ForecastAccountVersionRecord struct {
	AccountID             int64
	BookID                int64
	SystemRole            sql.NullString
	VersionID             int64
	VersionSeq            int64
	EffectiveFrom         string
	Status                string
	OpenedOn              string
	ClosedOn              sql.NullString
	Name                  sql.NullString
	AccountClass          string
	AccountKind           string
	ParentAccountID       sql.NullInt64
	DefaultCommodityID    sql.NullInt64
	QuantityScaleOverride sql.NullInt64
	AllowsPostings        bool
}

type ForecastCommodityVersionRecord struct {
	CommodityID      int64
	BookID           int64
	VersionID        int64
	VersionSeq       int64
	EffectiveFrom    string
	Code             string
	Kind             string
	Status           string
	StandardScale    int
	MaxQuantityScale int
}

// ForecastPostingRecord is a selected-account leg of a current posted entry.
// It keeps all identity needed by the pure projection and never uses SQL SUM.
type ForecastPostingRecord struct {
	PostingID            int64
	TransactionID        int64
	TransactionVersionID int64
	JournalEntryID       int64
	EntryDate            string
	AccountID            int64
	CommodityID          int64
	QuantityValue        exact.Coefficient
	QuantityScale        int
}

type ForecastTemplateRecord struct {
	ID              int64
	BookID          int64
	Name            string
	Enabled         bool
	ArchivedAt      sql.NullString
	TransactionKind string
	PayeeID         sql.NullInt64
	PayeeName       sql.NullString
	Frequency       string
	IntervalCount   int
	ByWeekday       sql.NullInt64
	DayOfMonth      sql.NullInt64
	LastDayOfMonth  bool
	MonthOfYear     sql.NullInt64
	StartsOn        string
	EndsOn          sql.NullString
	MaxOccurrences  sql.NullInt64
	GenerateFrom    string
}

type ForecastTemplatePostingRecord struct {
	TemplateID    int64
	PostingID     int64
	LineSeq       int
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
}

type ForecastOccurrenceRecord struct {
	ID                 int64
	TemplateID         int64
	OccurrenceDate     string
	Status             string
	TransactionID      sql.NullInt64
	TransactionStatus  sql.NullString
	TransactionDeleted bool
}

// ForecastDraftPostingRecord carries every posting of every entry in a linked
// current draft, including counterparts outside the selected account set and
// entries outside ThroughDate. That is what lets the later service validate a
// whole draft before it contributes a selected-account amount.
type ForecastDraftPostingRecord struct {
	OccurrenceID         int64
	TemplateID           int64
	OccurrenceDate       string
	TransactionID        int64
	TransactionVersionID int64
	TransactionDate      string
	TransactionKind      string
	JournalEntryID       int64
	EntryDate            string
	PostingID            int64
	AccountID            int64
	CommodityID          int64
	QuantityValue        exact.Coefficient
	QuantityScale        int
	PayeeID              sql.NullInt64
	PayeeName            sql.NullString
}

// WithSnapshot starts one deferred read transaction and keeps its *sql.Tx
// private to db. Every reader method below uses that transaction, preventing a
// forecast from mixing the old side of a concurrent generation with the new
// side of a concurrent posting.
func (r *ForecastRepository) WithSnapshot(ctx context.Context, fn func(*ForecastSnapshotReader) error) error {
	transaction, err := r.readOnly.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin forecast snapshot: %w", err)
	}
	defer rollbackTx(ctx, transaction)
	if err := fn(&ForecastSnapshotReader{transaction: transaction}); err != nil {
		return err
	}
	return nil
}

type ForecastSnapshotReader struct {
	transaction *sql.Tx
}

func (r *ForecastRepository) LoadSnapshot(ctx context.Context, request ForecastSnapshotRequest) (ForecastSnapshot, error) {
	return r.LoadResolvedSnapshot(ctx, request, func(ForecastSnapshot) (ForecastSnapshotResolution, error) {
		return ForecastSnapshotResolution{AccountIDs: request.AccountIDs, ThroughDate: request.ThroughDate}, nil
	})
}

type ForecastSnapshotResolution struct {
	AccountIDs  []int64
	ThroughDate string
}

// LoadResolvedSnapshot lets the application resolve account scope from the
// effective-dated reference rows before the account-dependent reads run. The
// resolver is pure and executes while the same read transaction remains open,
// so scope and financial inputs cannot straddle a concurrent write.
func (r *ForecastRepository) LoadResolvedSnapshot(ctx context.Context, request ForecastSnapshotRequest, resolve func(ForecastSnapshot) (ForecastSnapshotResolution, error)) (ForecastSnapshot, error) {
	limits := request.Limits
	if limits == (ForecastSnapshotLimits{}) {
		limits = DefaultForecastSnapshotLimits()
	}
	var result ForecastSnapshot
	err := r.WithSnapshot(ctx, func(reader *ForecastSnapshotReader) error {
		var err error
		result.TimeZone, err = reader.ownerTimeZone(ctx, request.OwnerUserID)
		if err != nil {
			return err
		}
		result.AccountVersions, err = reader.accountVersions(ctx, request.BookID, limits.VersionRows)
		if err != nil {
			return err
		}
		remainingVersions := limits.VersionRows - len(result.AccountVersions)
		if remainingVersions < 0 {
			return ErrForecastInputTooLarge
		}
		result.CommodityVersions, err = reader.commodityVersions(ctx, request.BookID, remainingVersions)
		if err != nil {
			return err
		}
		resolution, err := resolve(result)
		if err != nil {
			return err
		}
		if len(resolution.AccountIDs) == 0 {
			result.PayeeNames = map[int64]string{}
			return nil
		}
		result.PostedPostings, err = reader.postedPostings(ctx, request.BookID, resolution.AccountIDs, resolution.ThroughDate, limits.PostedPostingRows)
		if err != nil {
			return err
		}
		result.Templates, result.TemplatePostings, result.Occurrences, result.DraftPostings, err = reader.recurringInputs(ctx, request.BookID, resolution.AccountIDs, limits)
		if err != nil {
			return err
		}
		result.PayeeNames, err = reader.payeeNames(ctx, request.BookID, result.Templates, result.DraftPostings)
		return err
	})
	if err != nil {
		return ForecastSnapshot{}, err
	}
	return result, nil
}

func (r *ForecastSnapshotReader) ownerTimeZone(ctx context.Context, ownerUserID int64) (string, error) {
	var timeZone string
	err := r.transaction.QueryRowContext(ctx, `SELECT time_zone FROM user_preferences WHERE user_id = ?`, ownerUserID).Scan(&timeZone)
	if errors.Is(err, sql.ErrNoRows) {
		return "UTC", nil
	}
	if err != nil {
		return "", fmt.Errorf("read forecast owner time zone: %w", err)
	}
	return timeZone, nil
}

func (r *ForecastSnapshotReader) accountVersions(ctx context.Context, bookID int64, limit int) ([]ForecastAccountVersionRecord, error) {
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT a.id, a.book_id, a.system_role, av.id, av.version_seq, av.effective_from,
			av.status, av.opened_on, av.closed_on, av.name, av.account_class, av.account_kind,
			av.parent_account_id, av.default_commodity_id, av.quantity_scale_override, av.allows_postings
		FROM accounts a JOIN account_versions av ON av.account_id = a.id
		WHERE a.book_id = ?
		ORDER BY a.id, av.effective_from, av.version_seq
		LIMIT ?`, bookID, plusOne(limit))
	if err != nil {
		return nil, fmt.Errorf("read forecast account versions: %w", err)
	}
	defer rows.Close()
	var records []ForecastAccountVersionRecord
	for rows.Next() {
		var record ForecastAccountVersionRecord
		var allows int
		if err := rows.Scan(&record.AccountID, &record.BookID, &record.SystemRole, &record.VersionID, &record.VersionSeq, &record.EffectiveFrom, &record.Status, &record.OpenedOn, &record.ClosedOn, &record.Name, &record.AccountClass, &record.AccountKind, &record.ParentAccountID, &record.DefaultCommodityID, &record.QuantityScaleOverride, &allows); err != nil {
			return nil, fmt.Errorf("scan forecast account version: %w", err)
		}
		record.AllowsPostings = allows == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast account versions: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}

func (r *ForecastSnapshotReader) commodityVersions(ctx context.Context, bookID int64, limit int) ([]ForecastCommodityVersionRecord, error) {
	if limit < 0 {
		return nil, ErrForecastInputTooLarge
	}
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT c.id, c.book_id, cv.id, cv.version_seq, cv.effective_from, c.code, c.kind,
			cv.status, cv.standard_scale, cv.max_quantity_scale
		FROM commodities c JOIN commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.book_id = ? ORDER BY c.id, cv.effective_from, cv.version_seq LIMIT ?`, bookID, plusOne(limit))
	if err != nil {
		return nil, fmt.Errorf("read forecast commodity versions: %w", err)
	}
	defer rows.Close()
	var records []ForecastCommodityVersionRecord
	for rows.Next() {
		var record ForecastCommodityVersionRecord
		if err := rows.Scan(&record.CommodityID, &record.BookID, &record.VersionID, &record.VersionSeq, &record.EffectiveFrom, &record.Code, &record.Kind, &record.Status, &record.StandardScale, &record.MaxQuantityScale); err != nil {
			return nil, fmt.Errorf("scan forecast commodity version: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast commodity versions: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}

func (r *ForecastSnapshotReader) postedPostings(ctx context.Context, bookID int64, accountIDs []int64, throughDate string, limit int) ([]ForecastPostingRecord, error) {
	clause, args := idsClause("pv.account_id", accountIDs)
	if clause == "" {
		return []ForecastPostingRecord{}, nil
	}
	args = append([]any{bookID, throughDate}, args...)
	args = append(args, plusOne(limit))
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT pv.id, tv.transaction_id, tv.id, je.id, je.entry_date, pv.account_id,
			pv.commodity_id, pv.quantity_value, pv.quantity_scale
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE tv.book_id = ? AND tv.status = 'posted' AND t.deleted_at IS NULL
			AND je.entry_date <= ? AND `+clause+`
		ORDER BY je.entry_date, tv.transaction_id, tv.id, je.id, pv.line_seq, pv.id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read forecast posted postings: %w", err)
	}
	defer rows.Close()
	var records []ForecastPostingRecord
	for rows.Next() {
		var record ForecastPostingRecord
		if err := rows.Scan(&record.PostingID, &record.TransactionID, &record.TransactionVersionID, &record.JournalEntryID, &record.EntryDate, &record.AccountID, &record.CommodityID, &record.QuantityValue, &record.QuantityScale); err != nil {
			return nil, fmt.Errorf("scan forecast posted posting: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast posted postings: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}

func (r *ForecastSnapshotReader) recurringInputs(ctx context.Context, bookID int64, accountIDs []int64, limits ForecastSnapshotLimits) ([]ForecastTemplateRecord, []ForecastTemplatePostingRecord, []ForecastOccurrenceRecord, []ForecastDraftPostingRecord, error) {
	clause, accountArgs := idsClause("rtp.account_id", accountIDs)
	if clause == "" {
		return []ForecastTemplateRecord{}, []ForecastTemplatePostingRecord{}, []ForecastOccurrenceRecord{}, []ForecastDraftPostingRecord{}, nil
	}
	// The second arm deliberately joins current draft postings, rather than the
	// template: a template may have been archived or edited after it generated.
	query := `SELECT DISTINCT template_id FROM (
		SELECT rt.id AS template_id FROM recurring_templates rt JOIN recurring_template_postings rtp ON rtp.template_id = rt.id WHERE rt.book_id = ? AND ` + clause + `
		UNION
		SELECT ro.template_id FROM recurring_occurrences ro JOIN current_transaction_versions tv ON tv.transaction_id = ro.transaction_id JOIN transactions t ON t.id = tv.transaction_id JOIN journal_entries je ON je.transaction_version_id = tv.id JOIN posting_versions pv ON pv.journal_entry_id = je.id WHERE ro.book_id = ? AND tv.status = 'draft' AND t.deleted_at IS NULL AND ` + strings.Replace(clause, "rtp.", "pv.", 1) + `
	) ORDER BY template_id LIMIT ?`
	args := append([]any{bookID}, accountArgs...)
	args = append(args, bookID)
	args = append(args, accountArgs...)
	args = append(args, plusOne(limits.RelevantTemplates))
	rows, err := r.transaction.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("read relevant forecast templates: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, nil, nil, fmt.Errorf("scan relevant forecast template: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("close relevant forecast templates: %w", err)
	}
	if len(ids) > limits.RelevantTemplates {
		return nil, nil, nil, nil, ErrForecastInputTooLarge
	}
	if len(ids) == 0 {
		return []ForecastTemplateRecord{}, []ForecastTemplatePostingRecord{}, []ForecastOccurrenceRecord{}, []ForecastDraftPostingRecord{}, nil
	}
	templates, err := r.templates(ctx, bookID, ids)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	postings, err := r.templatePostings(ctx, bookID, ids, limits.TemplateAndDraftPostingRows)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	occurrences, err := r.occurrences(ctx, bookID, ids, limits.OccurrenceRows)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	drafts, err := r.draftPostings(ctx, bookID, ids, limits.TemplateAndDraftPostingRows-len(postings))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return templates, postings, occurrences, drafts, nil
}

func (r *ForecastSnapshotReader) templates(ctx context.Context, bookID int64, ids []int64) ([]ForecastTemplateRecord, error) {
	clause, args := idsClause("rt.id", ids)
	args = append([]any{bookID}, args...)
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT rt.id, rt.book_id, rt.name, rt.enabled, rt.archived_at,
			rt.transaction_kind, rt.payee_id, rt.payee_name, rt.frequency,
			rt.interval_count, rt.by_weekday, rt.day_of_month, rt.last_day_of_month,
			rt.month_of_year, rt.starts_on, rt.ends_on, rt.max_occurrences, rt.generate_from
		FROM recurring_templates rt WHERE rt.book_id = ? AND `+clause+` ORDER BY rt.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("read forecast templates: %w", err)
	}
	defer rows.Close()
	var records []ForecastTemplateRecord
	for rows.Next() {
		var record ForecastTemplateRecord
		var enabled, last int
		if err := rows.Scan(&record.ID, &record.BookID, &record.Name, &enabled, &record.ArchivedAt, &record.TransactionKind, &record.PayeeID, &record.PayeeName, &record.Frequency, &record.IntervalCount, &record.ByWeekday, &record.DayOfMonth, &last, &record.MonthOfYear, &record.StartsOn, &record.EndsOn, &record.MaxOccurrences, &record.GenerateFrom); err != nil {
			return nil, fmt.Errorf("scan forecast template: %w", err)
		}
		record.Enabled, record.LastDayOfMonth = enabled == 1, last == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast templates: %w", err)
	}
	return records, nil
}

func (r *ForecastSnapshotReader) templatePostings(ctx context.Context, bookID int64, ids []int64, limit int) ([]ForecastTemplatePostingRecord, error) {
	if limit < 0 {
		return nil, ErrForecastInputTooLarge
	}
	clause, idsArgs := idsClause("rtp.template_id", ids)
	args := append([]any{bookID, bookID}, idsArgs...)
	args = append(args, plusOne(limit))
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT rtp.template_id, rtp.id, rtp.line_seq, rtp.account_id, rtp.commodity_id, rtp.quantity_value, rtp.quantity_scale
		FROM recurring_template_postings rtp JOIN recurring_templates rt ON rt.id = rtp.template_id
		WHERE rtp.book_id = ? AND rt.book_id = ? AND `+clause+`
		ORDER BY rtp.template_id, rtp.line_seq, rtp.id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read forecast template postings: %w", err)
	}
	defer rows.Close()
	var records []ForecastTemplatePostingRecord
	for rows.Next() {
		var record ForecastTemplatePostingRecord
		if err := rows.Scan(&record.TemplateID, &record.PostingID, &record.LineSeq, &record.AccountID, &record.CommodityID, &record.QuantityValue, &record.QuantityScale); err != nil {
			return nil, fmt.Errorf("scan forecast template posting: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast template postings: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}

func (r *ForecastSnapshotReader) occurrences(ctx context.Context, bookID int64, ids []int64, limit int) ([]ForecastOccurrenceRecord, error) {
	clause, args := idsClause("ro.template_id", ids)
	args = append([]any{bookID}, args...)
	args = append(args, plusOne(limit))
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT ro.id, ro.template_id, ro.occurrence_date, ro.status, ro.transaction_id,
			tv.status, CASE WHEN t.deleted_at IS NOT NULL THEN 1 ELSE 0 END
		FROM recurring_occurrences ro
		LEFT JOIN transactions t ON t.id = ro.transaction_id AND t.book_id = ro.book_id
		LEFT JOIN current_transaction_versions tv ON tv.transaction_id = t.id
		WHERE ro.book_id = ? AND `+clause+`
		ORDER BY ro.template_id, ro.occurrence_date, ro.id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read forecast occurrences: %w", err)
	}
	defer rows.Close()
	var records []ForecastOccurrenceRecord
	for rows.Next() {
		var record ForecastOccurrenceRecord
		var deleted int
		if err := rows.Scan(&record.ID, &record.TemplateID, &record.OccurrenceDate, &record.Status, &record.TransactionID, &record.TransactionStatus, &deleted); err != nil {
			return nil, fmt.Errorf("scan forecast occurrence: %w", err)
		}
		record.TransactionDeleted = deleted == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast occurrences: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}

func (r *ForecastSnapshotReader) draftPostings(ctx context.Context, bookID int64, ids []int64, limit int) ([]ForecastDraftPostingRecord, error) {
	if limit < 0 {
		return nil, ErrForecastInputTooLarge
	}
	clause, idsArgs := idsClause("ro.template_id", ids)
	args := append([]any{bookID, bookID}, idsArgs...)
	args = append(args, plusOne(limit))
	rows, err := r.transaction.QueryContext(ctx, `
		SELECT ro.id, ro.template_id, ro.occurrence_date, tv.transaction_id, tv.id, tv.transaction_date, tv.transaction_kind,
			je.id, je.entry_date, pv.id, pv.account_id, pv.commodity_id, pv.quantity_value,
			pv.quantity_scale, tv.payee_id, tv.payee_name
		FROM recurring_occurrences ro
		JOIN current_transaction_versions tv ON tv.transaction_id = ro.transaction_id
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE ro.book_id = ? AND tv.book_id = ? AND tv.status = 'draft' AND t.deleted_at IS NULL AND `+clause+`
		ORDER BY ro.id, je.entry_seq, pv.line_seq, pv.id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("read forecast draft postings: %w", err)
	}
	defer rows.Close()
	var records []ForecastDraftPostingRecord
	for rows.Next() {
		var record ForecastDraftPostingRecord
		if err := rows.Scan(&record.OccurrenceID, &record.TemplateID, &record.OccurrenceDate, &record.TransactionID, &record.TransactionVersionID, &record.TransactionDate, &record.TransactionKind, &record.JournalEntryID, &record.EntryDate, &record.PostingID, &record.AccountID, &record.CommodityID, &record.QuantityValue, &record.QuantityScale, &record.PayeeID, &record.PayeeName); err != nil {
			return nil, fmt.Errorf("scan forecast draft posting: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast draft postings: %w", err)
	}
	if len(records) > limit {
		return nil, ErrForecastInputTooLarge
	}
	return records, nil
}

func (r *ForecastSnapshotReader) payeeNames(ctx context.Context, bookID int64, templates []ForecastTemplateRecord, drafts []ForecastDraftPostingRecord) (map[int64]string, error) {
	set := map[int64]struct{}{}
	for _, record := range templates {
		if record.PayeeID.Valid {
			set[record.PayeeID.Int64] = struct{}{}
		}
	}
	for _, record := range drafts {
		if record.PayeeID.Valid {
			set[record.PayeeID.Int64] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	clause, args := idsClause("p.id", ids)
	if clause == "" {
		return map[int64]string{}, nil
	}
	args = append([]any{bookID}, args...)
	rows, err := r.transaction.QueryContext(ctx, `SELECT p.id, pv.name FROM payees p JOIN current_payee_versions pv ON pv.payee_id = p.id WHERE p.book_id = ? AND `+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("read forecast payee names: %w", err)
	}
	defer rows.Close()
	result := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan forecast payee name: %w", err)
		}
		result[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forecast payee names: %w", err)
	}
	return result, nil
}

func idsClause(column string, ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	parts := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		parts[i] = "?"
		args[i] = id
	}
	return column + " IN (" + strings.Join(parts, ",") + ")", args
}
func plusOne(limit int) int {
	if limit < 0 || limit == int(^uint(0)>>1) {
		return limit
	}
	return limit + 1
}
