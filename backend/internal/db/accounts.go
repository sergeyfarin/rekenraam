package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrSystemAccountsSetupComplete = errors.New("system accounts setup already complete")
	ErrAccountDeleteNotPermitted   = errors.New("account cannot be deleted")
	// ErrAccountStructureLocked and ErrAccountCurrencyLocked report a
	// structural or default-commodity edit that the account write found
	// postings for inside its own database transaction (T-100). The service
	// asks the same questions first, for a cheap answer and a message that
	// names the field; these fire when the answer changed before the write.
	ErrAccountStructureLocked = errors.New("account structure cannot be changed after postings exist")
	ErrAccountCurrencyLocked  = errors.New("account currency cannot be changed after postings exist")
	// ErrAccountHasUnarchivedChildren refuses archiving a parent while a
	// child is still active or closed, and ErrAccountParentArchived and
	// ErrAccountInstitutionArchived refuse the reverse: a non-archived version
	// under an archived parent or institution. Either state leaves an account
	// that no later write can keep (#294).
	ErrAccountHasUnarchivedChildren = errors.New("account has child accounts that are not archived")
	ErrAccountParentArchived        = errors.New("account parent is archived")
	ErrAccountInstitutionArchived   = errors.New("account institution is archived")
	// ErrAccountParentNotAllowed and ErrAccountParentCycle are the parent
	// rules re-asked inside the write: a parent of the same class that is
	// neither a system account nor a category, and no date on which the new
	// version makes the account its own ancestor (#293).
	ErrAccountParentNotAllowed = errors.New("account parent is not allowed")
	ErrAccountParentCycle      = errors.New("account parent would create a cycle")
)

// AccountVersionBackdatedError refuses a lifecycle or structural version dated
// before the account's latest version (#293). LatestEffectiveFrom is the
// earliest date such a change may take effect.
type AccountVersionBackdatedError struct {
	LatestEffectiveFrom string
}

func (e AccountVersionBackdatedError) Error() string {
	return fmt.Sprintf("lifecycle and structure changes cannot take effect before %s, when the account's latest version takes effect; an earlier date may change descriptive fields only", e.LatestEffectiveFrom)
}

// AccountPostingsOutsideDatesError reports an account version that would
// govern a posting the account already has and refuse it — a close dated
// before the posting, or a backdated archive (#292). EntryDate is the earliest
// such posting's entry date.
type AccountPostingsOutsideDatesError struct {
	EntryDate string
}

func (e AccountPostingsOutsideDatesError) Error() string {
	return fmt.Sprintf("account change would leave a posting on %s outside the account's open dates", e.EntryDate)
}

type AccountRepository struct {
	database *sql.DB
}

type AccountRecord struct {
	ID                    int64
	BookID                int64
	IsSystem              bool
	SystemRole            sql.NullString
	CreatedAt             string
	CreatedByUserID       int64
	VersionID             int64
	VersionSeq            int64
	EffectiveFrom         string
	RecordedAt            string
	ChangedByUserID       int64
	ChangeReason          string
	Status                string
	OpenedOn              string
	ClosedOn              sql.NullString
	Code                  sql.NullString
	Name                  sql.NullString
	AccountClass          string
	AccountKind           string
	ParentAccountID       sql.NullInt64
	InstitutionID         sql.NullInt64
	CountryCode           sql.NullString
	DefaultCommodityID    sql.NullInt64
	QuantityScaleOverride sql.NullInt64
	AllowsPostings        bool
	HasActivity           bool
	NumberLast4           sql.NullString
	ExternalRefHint       sql.NullString
	CommentMarkdown       string
	MetadataJSON          string
	CostBasisMethod       sql.NullString
}

type AccountSpec struct {
	Code                  string
	Name                  string
	AccountClass          string
	AccountKind           string
	ParentAccountID       sql.NullInt64
	InstitutionID         sql.NullInt64
	CountryCode           string
	DefaultCommodityID    sql.NullInt64
	EnsureDefaultCurrency *CurrencySpec
	QuantityScaleOverride sql.NullInt64
	AllowsPostings        bool
	NumberLast4           string
	ExternalRefHint       string
	CommentMarkdown       string
	MetadataJSON          string
	OpenedOn              string
	ClosedOn              sql.NullString
	CostBasisMethod       string
}

type ListAccountsParams struct {
	BookID          int64
	Status          string
	AccountClass    string
	IncludeArchived bool
	IncludeSystem   bool
	Query           string
	// After continues a listing strictly after this position in the list
	// order; nil starts at the first account (#518).
	After *AccountListPosition
	Limit int
}

// AccountListPosition is one account's place in the account-list order:
// class rank, then display sort name, then id. The id makes it total, so a
// keyset cursor built from it never skips or repeats an unchanged account
// (#518).
type AccountListPosition struct {
	ClassRank int
	SortName  string
	ID        int64
}

// AccountListPage is one bounded page of the account list. Next is the
// position of its last row when more rows follow, and nil on the last page.
type AccountListPage struct {
	Records []AccountRecord
	Next    *AccountListPosition
}

type CreateAccountParams struct {
	BookID          int64
	CreatedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            AccountSpec
	ChangeReason    string
	CreatedAt       string
	EffectiveFrom   string
}

type UpdateAccountParams struct {
	BookID          int64
	AccountID       int64
	ChangedByUserID int64
	AuthSessionID   int64
	RequestID       string
	OriginType      string
	Operation       string
	Spec            AccountSpec
	Status          string
	ChangeReason    string
	RecordedAt      string
	EffectiveFrom   string
	// RequireNoPostingsForStructure and RequireNoPostingsForCommodity re-ask,
	// inside the write transaction, the question the service already asked
	// outside it: does this account have any postings? An account whose
	// structure or default commodity is changing must have none, and a posting
	// prepared against the old structure can commit in between (T-100). Set
	// them whenever the corresponding part of the spec actually changes.
	RequireNoPostingsForStructure bool
	RequireNoPostingsForCommodity bool
}

type DeleteAccountParams struct {
	BookID    int64
	AccountID int64
}

type SystemAccountSpec struct {
	Role         string
	AccountClass string
	AccountKind  string
}

type StarterCashAccountSpec struct {
	CommodityID  int64
	CurrencyCode string
}

type EnsureSystemAccountsParams struct {
	BookID               int64
	ChangedByUserID      int64
	AuthSessionID        int64
	RequestID            string
	OriginType           string
	Operation            string
	Specs                []SystemAccountSpec
	StarterCashSpecs     []StarterCashAccountSpec
	CreatedAt            string
	EffectiveFrom        string
	StarterEffectiveFrom string
}

type EnsureSystemAccountsRecord struct {
	Accounts []AccountRecord
}

type AccountCommodityRecord struct {
	ID               int64
	BookID           int64
	Code             string
	Kind             string
	Status           string
	MaxQuantityScale int
}

type AccountKindRecord struct {
	Code             string
	AccountClass     string
	BaseKind         string
	IsBuiltin        bool
	IsUserAssignable bool
	DisplayKey       string
	SortOrder        int
}

func NewAccountRepository(database *sql.DB) *AccountRepository {
	return &AccountRepository{database: database}
}

// accountListClassRank and accountListSortName are the SQL forms of the
// account-list order; AccountListPositionOf is their Go mirror. The keyset
// predicate compares the very same expressions the ORDER BY sorts on, so the
// cursor and the order cannot disagree on collation or NULL handling.
const (
	accountListClassRank = `CASE av.account_class
				WHEN 'asset' THEN 1
				WHEN 'liability' THEN 2
				WHEN 'equity' THEN 3
				WHEN 'income' THEN 4
				WHEN 'expense' THEN 5
				ELSE 99
			END`
	accountListSortName = `COALESCE(av.name, a.system_role, '')`
)

var accountListClassRanks = map[string]int{
	"asset":     1,
	"liability": 2,
	"equity":    3,
	"income":    4,
	"expense":   5,
}

// AccountListPositionOf returns the record's place in the account-list order.
func AccountListPositionOf(record AccountRecord) AccountListPosition {
	rank, ok := accountListClassRanks[record.AccountClass]
	if !ok {
		rank = 99
	}
	sortName := ""
	switch {
	case record.Name.Valid:
		sortName = record.Name.String
	case record.SystemRole.Valid:
		sortName = record.SystemRole.String
	}
	return AccountListPosition{ClassRank: rank, SortName: sortName, ID: record.ID}
}

func (r *AccountRepository) ListAccounts(ctx context.Context, params ListAccountsParams) (AccountListPage, error) {
	if params.Limit <= 0 {
		return AccountListPage{}, fmt.Errorf("list accounts: limit must be positive")
	}

	where := []string{"a.book_id = ?"}
	args := []any{params.BookID}

	if params.Status != "" {
		where = append(where, "av.status = ?")
		args = append(args, params.Status)
	} else if !params.IncludeArchived {
		where = append(where, "av.status != 'archived'")
	}

	if params.AccountClass != "" {
		where = append(where, "av.account_class = ?")
		args = append(args, params.AccountClass)
	}

	if !params.IncludeSystem {
		where = append(where, "a.system_role IS NULL")
	}

	query := strings.TrimSpace(params.Query)
	if query != "" {
		where = append(where, "(av.name LIKE ? OR av.code LIKE ? OR av.account_kind LIKE ? OR a.system_role LIKE ?)")
		like := "%" + query + "%"
		args = append(args, like, like, like, like)
	}

	if after := params.After; after != nil {
		where = append(where, "("+accountListClassRank+" > ? OR ("+accountListClassRank+" = ? AND ("+
			accountListSortName+" > ? OR ("+accountListSortName+" = ? AND a.id > ?))))")
		args = append(args, after.ClassRank, after.ClassRank, after.SortName, after.SortName, after.ID)
	}

	// One extra row tells whether another page follows without a count.
	args = append(args, params.Limit+1)

	rows, err := r.database.QueryContext(ctx, currentAccountSelect(ctx, "AND "+strings.Join(where, " AND ")+`
		ORDER BY
			`+accountListClassRank+`,
			`+accountListSortName+`,
			a.id
		LIMIT ?
	`), args...)
	if err != nil {
		return AccountListPage{}, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	records, err := scanAccountRecords(rows)
	if err != nil {
		return AccountListPage{}, err
	}

	page := AccountListPage{Records: records}
	if len(records) > params.Limit {
		page.Records = records[:params.Limit]
		next := AccountListPositionOf(page.Records[params.Limit-1])
		page.Next = &next
	}
	return page, nil
}

// PostingAccountSummary holds the account fields needed to enrich posting responses.
type PostingAccountSummary struct {
	ID           int64
	Name         sql.NullString
	Code         sql.NullString
	SystemRole   sql.NullString
	AccountClass string
	AccountKind  string
	BaseKind     string         // account_kinds.base_kind — the kind's family
	BuiltinKey   sql.NullString // json_extract(metadata_json, '$.category.builtin_key')
}

// AccountsByIDs returns a PostingAccountSummary keyed by account ID for the given IDs.
// Missing IDs are absent from the map (not an error). Callers must deduplicate IDs first.
func (r *AccountRepository) AccountsByIDs(ctx context.Context, bookID int64, ids []int64) (map[int64]PostingAccountSummary, error) {
	if len(ids) == 0 {
		return map[int64]PostingAccountSummary{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, bookID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := `
		SELECT
			a.id,
			av.name,
			av.code,
			a.system_role,
			av.account_class,
			av.account_kind,
			ak.base_kind,
			json_extract(av.metadata_json, '$.category.builtin_key')
		FROM accounts a
		JOIN ` + CurrentReferenceVersions(ctx, "account") + ` av ON av.account_id = a.id
		JOIN account_kinds ak ON ak.code = av.account_kind AND ak.account_class = av.account_class
		WHERE a.book_id = ?
			AND a.id IN (` + strings.Join(placeholders, ", ") + `)`

	rows, err := r.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("accounts by ids: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]PostingAccountSummary, len(ids))
	for rows.Next() {
		var s PostingAccountSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Code, &s.SystemRole, &s.AccountClass, &s.AccountKind, &s.BaseKind, &s.BuiltinKey); err != nil {
			return nil, fmt.Errorf("scan posting account summary: %w", err)
		}
		result[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting account summaries: %w", err)
	}

	return result, nil
}

func (r *AccountRepository) CurrentAccountByID(ctx context.Context, bookID int64, accountID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(r.database.QueryRowContext(ctx, currentAccountSelect(ctx, `
		AND a.book_id = ? AND a.id = ?
	`), bookID, accountID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func (r *AccountRepository) ListAccountVersions(ctx context.Context, bookID int64, accountID int64) ([]AccountRecord, error) {
	rows, err := r.database.QueryContext(ctx, accountSelect("account_versions", `
		WHERE a.book_id = ? AND a.id = ?
		ORDER BY av.version_seq DESC
	`), bookID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list account versions: %w", err)
	}
	defer rows.Close()

	records, err := scanAccountRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrNotFound
	}

	return records, nil
}

func (r *AccountRepository) CreateAccount(ctx context.Context, params CreateAccountParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin create account transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return AccountRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.CreatedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        params.ChangeReason,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, NULL, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, params.CreatedAt, params.CreatedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert account: %w", err)
	}

	accountID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read account id: %w", err)
	}

	spec, err := r.accountSpecWithEnsuredCurrency(ctx, tx, params.Spec, params, auditEventID)
	if err != nil {
		return AccountRecord{}, err
	}

	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          accountID,
		CreatedAt:          params.CreatedAt,
		CreatedByUserID:    params.CreatedByUserID,
		VersionSeq:         1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.CreatedByUserID,
		ChangeReason:       params.ChangeReason,
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec:               spec,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	if err := tx.Commit(); err != nil {
		return AccountRecord{}, fmt.Errorf("commit create account transaction: %w", err)
	}
	committed = true

	return record, nil
}

func (r *AccountRepository) UpdateAccount(ctx context.Context, params UpdateAccountParams) (AccountRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("begin update account transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	current, err := currentAccountByID(ctx, tx, params.BookID, params.AccountID)
	if err != nil {
		return AccountRecord{}, err
	}
	if params.RequireNoPostingsForStructure || params.RequireNoPostingsForCommodity {
		hasPostings, err := accountHasPostings(ctx, tx, params.BookID, params.AccountID)
		if err != nil {
			return AccountRecord{}, err
		}
		if hasPostings {
			if params.RequireNoPostingsForCommodity {
				return AccountRecord{}, ErrAccountCurrencyLocked
			}
			return AccountRecord{}, ErrAccountStructureLocked
		}
	}

	status := params.Status
	if status == "" {
		status = current.Status
	}
	next := params.Spec
	latest, err := latestAccountVersionDateTx(ctx, tx, params.AccountID)
	if err != nil {
		return AccountRecord{}, err
	}
	if params.EffectiveFrom < latest {
		// A version dated before the latest one governs only the days up to
		// it, so it cannot be what the account is today (#293). Lifecycle and
		// structure therefore take effect on or after the latest version; an
		// earlier-dated version is a correction of descriptive fields and
		// keeps the structure and lifecycle the account had on its date.
		if params.Status != "" {
			return AccountRecord{}, AccountVersionBackdatedError{LatestEffectiveFrom: latest}
		}
		asOf, err := accountVersionAsOfTx(ctx, tx, params.BookID, params.AccountID, params.EffectiveFrom)
		if errors.Is(err, ErrNotFound) {
			return AccountRecord{}, AccountVersionBackdatedError{LatestEffectiveFrom: latest}
		}
		if err != nil {
			return AccountRecord{}, err
		}
		if accountStructureDiffers(asOf, next) {
			return AccountRecord{}, AccountVersionBackdatedError{LatestEffectiveFrom: latest}
		}
		status = asOf.Status
		next.ClosedOn = asOf.ClosedOn
	}
	if err := requireArchiveConsistencyTx(ctx, tx, params.BookID, params.AccountID, status, next); err != nil {
		return AccountRecord{}, err
	}

	changeReason := strings.TrimSpace(params.ChangeReason)
	if changeReason == "" {
		changeReason = "updated account"
	}
	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.RecordedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        changeReason,
	})
	if err != nil {
		return AccountRecord{}, err
	}

	spec, err := r.accountSpecWithEnsuredCurrency(ctx, tx, next, CreateAccountParams{
		BookID:          params.BookID,
		CreatedByUserID: params.ChangedByUserID,
		AuthSessionID:   params.AuthSessionID,
		RequestID:       params.RequestID,
		OriginType:      params.OriginType,
		Operation:       params.Operation,
		CreatedAt:       params.RecordedAt,
		EffectiveFrom:   params.EffectiveFrom,
	}, auditEventID)
	if err != nil {
		return AccountRecord{}, err
	}

	record, err := insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          params.AccountID,
		CreatedAt:          current.CreatedAt,
		CreatedByUserID:    current.CreatedByUserID,
		VersionSeq:         current.VersionSeq + 1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.RecordedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       changeReason,
		ChangeAuditEventID: auditEventID,
		Status:             status,
		Spec:               spec,
	})
	if err != nil {
		return AccountRecord{}, err
	}
	// Asked inside the write, after the version exists, so the answer covers
	// postings committed concurrently and uses the same as-of pick the
	// posting trigger will (#292).
	if err := requirePostingsStayEligibleTx(ctx, tx, params.AccountID, record.VersionID); err != nil {
		return AccountRecord{}, err
	}
	if spec.ParentAccountID.Valid {
		if err := requireNoDatedParentCycleTx(ctx, tx, params.BookID, params.AccountID, params.EffectiveFrom); err != nil {
			return AccountRecord{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return AccountRecord{}, fmt.Errorf("commit update account transaction: %w", err)
	}
	committed = true

	return record, nil
}

// requireArchiveConsistencyTx keeps "no account in use under an archived
// parent or institution" true at commit (#294). Asked inside the write so a
// concurrent archive or restore of the other side cannot slip between the
// service's check and this version.
func requireArchiveConsistencyTx(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64, status string, spec AccountSpec) error {
	if status == "archived" {
		var children int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(1)
			FROM accounts a
			JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id
			WHERE a.book_id = ? AND av.parent_account_id = ? AND av.status <> 'archived'
		`, bookID, accountID).Scan(&children); err != nil {
			return fmt.Errorf("check account children: %w", err)
		}
		if children > 0 {
			return ErrAccountHasUnarchivedChildren
		}
		return nil
	}
	if spec.ParentAccountID.Valid {
		var parentStatus, parentClass string
		var parentIsSystem, parentIsCategory bool
		err := tx.QueryRowContext(ctx, `
			SELECT av.status, av.account_class, a.system_role IS NOT NULL,
				av.account_class IN ('income', 'expense')
					AND av.account_kind = av.account_class
					AND COALESCE(json_extract(av.metadata_json, '$.category.type'), '') = av.account_class
			FROM accounts a
			JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id
			WHERE a.book_id = ? AND a.id = ?
		`, bookID, spec.ParentAccountID.Int64).Scan(&parentStatus, &parentClass, &parentIsSystem, &parentIsCategory)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAccountParentNotAllowed
		}
		if err != nil {
			return fmt.Errorf("check account parent: %w", err)
		}
		if parentStatus == "archived" {
			return ErrAccountParentArchived
		}
		// The service checked these before the write; a concurrent change to
		// the parent could still land in between.
		if parentClass != spec.AccountClass || parentIsSystem || parentIsCategory {
			return ErrAccountParentNotAllowed
		}
	}
	if spec.InstitutionID.Valid {
		var institutionStatus string
		if err := tx.QueryRowContext(ctx, `
			SELECT iv.status
			FROM institutions i
			JOIN `+CurrentReferenceVersions(ctx, "institution")+` iv ON iv.institution_id = i.id
			WHERE i.book_id = ? AND i.id = ?
		`, bookID, spec.InstitutionID.Int64).Scan(&institutionStatus); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check account institution status: %w", err)
		}
		if institutionStatus == "archived" {
			return ErrAccountInstitutionArchived
		}
	}
	return nil
}

func latestAccountVersionDateTx(ctx context.Context, tx *sql.Tx, accountID int64) (string, error) {
	var latest sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MAX(effective_from) FROM account_versions WHERE account_id = ?`, accountID).Scan(&latest); err != nil {
		return "", fmt.Errorf("read latest account version date: %w", err)
	}
	return latest.String, nil
}

func accountVersionAsOfTx(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64, date string) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, accountSelect("account_versions", `
		WHERE a.book_id = ? AND a.id = ?
			AND av.id = (
				SELECT asof_av.id FROM account_versions asof_av
				WHERE asof_av.account_id = a.id AND asof_av.effective_from <= ?
				ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC LIMIT 1
			)
	`), bookID, accountID, date), &record); err != nil {
		return AccountRecord{}, err
	}
	return record, nil
}

// accountStructureDiffers compares the structural fields an earlier-dated
// correction must keep (#293) — the same fields the posting lock covers.
func accountStructureDiffers(version AccountRecord, spec AccountSpec) bool {
	return spec.EnsureDefaultCurrency != nil ||
		version.AccountClass != spec.AccountClass ||
		version.AccountKind != spec.AccountKind ||
		version.ParentAccountID != spec.ParentAccountID ||
		version.InstitutionID != spec.InstitutionID ||
		version.DefaultCommodityID != spec.DefaultCommodityID ||
		version.QuantityScaleOverride != spec.QuantityScaleOverride ||
		version.AllowsPostings != spec.AllowsPostings ||
		version.OpenedOn != spec.OpenedOn ||
		version.CostBasisMethod.String != spec.CostBasisMethod
}

// requireNoDatedParentCycleTx refuses a version that makes the account its
// own ancestor on any date it governs (#293). Parent links are versioned and
// reports resolve the tree per date, so checking today's tree is not enough:
// another account's history can close the loop on an earlier date. The dates
// to check are the version's own date and every later date on which any
// account's version changes; between them the tree is constant. The new
// version is already inserted, so the as-of pick includes it.
func requireNoDatedParentCycleTx(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64, effectiveFrom string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT ?
		UNION
		SELECT DISTINCT av.effective_from
		FROM account_versions av
		JOIN accounts a ON a.id = av.account_id
		WHERE a.book_id = ? AND av.effective_from > ?
	`, effectiveFrom, bookID, effectiveFrom)
	if err != nil {
		return fmt.Errorf("read account tree dates: %w", err)
	}
	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			rows.Close()
			return fmt.Errorf("scan account tree date: %w", err)
		}
		dates = append(dates, date)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close account tree dates: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate account tree dates: %w", err)
	}

	for _, date := range dates {
		var loops int
		// The depth bound only stops the walk on a loop elsewhere in the
		// tree; a loop through this account is found on its first return.
		if err := tx.QueryRowContext(ctx, `
			WITH RECURSIVE chain(id, depth) AS (
				SELECT (
					SELECT av.parent_account_id FROM account_versions av
					WHERE av.account_id = ? AND av.effective_from <= ?
					ORDER BY av.effective_from DESC, av.version_seq DESC LIMIT 1
				), 1
				UNION ALL
				SELECT (
					SELECT av.parent_account_id FROM account_versions av
					WHERE av.account_id = chain.id AND av.effective_from <= ?
					ORDER BY av.effective_from DESC, av.version_seq DESC LIMIT 1
				), chain.depth + 1
				FROM chain
				WHERE chain.id IS NOT NULL AND chain.id <> ? AND chain.depth < 1000
			)
			SELECT COUNT(1) FROM chain WHERE id = ?
		`, accountID, date, date, accountID, accountID).Scan(&loops); err != nil {
			return fmt.Errorf("check account parent cycle on %s: %w", date, err)
		}
		if loops > 0 {
			return ErrAccountParentCycle
		}
	}
	return nil
}

// requirePostingsStayEligibleTx refuses a new account version that would
// govern an existing posting and not accept it. Only postings the new version
// governs are examined: a posting already outside an older version's dates is
// not this write's doing, which keeps a repair (such as a reopen from the
// closing date) possible. Voided and soft-deleted transactions are skipped;
// nothing rewrites their postings. The eligibility test mirrors the
// posting_versions_account_version_valid trigger.
func requirePostingsStayEligibleTx(ctx context.Context, tx *sql.Tx, accountID int64, versionID int64) error {
	var earliest sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT MIN(je.entry_date)
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN account_versions av ON av.id = ?
		WHERE pv.account_id = ?
			AND tv.status <> 'voided'
			AND t.deleted_at IS NULL
			AND (
				SELECT asof_av.id FROM account_versions asof_av
				WHERE asof_av.account_id = pv.account_id AND asof_av.effective_from <= je.entry_date
				ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC LIMIT 1
			) = av.id
			AND NOT (
				(
					av.status = 'active'
					OR (av.status = 'closed' AND av.closed_on IS NOT NULL AND je.entry_date <= av.closed_on)
				)
				AND av.allows_postings = 1
				AND (av.opened_on IS NULL OR je.entry_date >= av.opened_on)
				AND (av.closed_on IS NULL OR je.entry_date <= av.closed_on)
				AND (av.default_commodity_id IS NULL OR av.default_commodity_id = pv.commodity_id)
				AND (av.quantity_scale_override IS NULL OR pv.quantity_scale <= av.quantity_scale_override)
			)
	`, versionID, accountID).Scan(&earliest); err != nil {
		return fmt.Errorf("check account postings stay eligible: %w", err)
	}
	if earliest.Valid {
		return AccountPostingsOutsideDatesError{EntryDate: earliest.String}
	}
	return nil
}

func (r *AccountRepository) DeleteUnusedAccount(ctx context.Context, params DeleteAccountParams) error {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete account transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	current, err := currentAccountByID(ctx, tx, params.BookID, params.AccountID)
	if err != nil {
		return err
	}
	if current.IsSystem {
		return ErrAccountDeleteNotPermitted
	}

	var referenceCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(1) FROM posting_versions WHERE account_id = ?)
			+ (SELECT COUNT(1) FROM account_versions WHERE parent_account_id = ?)
			+ (SELECT COUNT(1) FROM payee_versions WHERE default_account_id = ?)
			+ (SELECT COUNT(1) FROM payee_versions WHERE default_category_account_id = ?)
			+ (SELECT COUNT(1) FROM reconciliation_sessions WHERE account_id = ?)
			+ (SELECT COUNT(1) FROM reconciliation_session_postings WHERE account_id = ?)
			+ (SELECT COUNT(1) FROM reconciliation_checkpoints WHERE account_id = ?)
			+ (SELECT COUNT(1) FROM reconciliation_checkpoint_postings WHERE account_id = ?)
	`, params.AccountID, params.AccountID, params.AccountID, params.AccountID, params.AccountID, params.AccountID, params.AccountID, params.AccountID).Scan(&referenceCount); err != nil {
		return fmt.Errorf("read account reference count: %w", err)
	}
	if referenceCount > 0 {
		return ErrAccountDeleteNotPermitted
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM account_versions
		WHERE account_id = ?
	`, params.AccountID)
	if err != nil {
		return mapAccountDeleteError(fmt.Errorf("delete account versions: %w", err))
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted account version rows: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	result, err = tx.ExecContext(ctx, `
		DELETE FROM accounts
		WHERE book_id = ?
			AND id = ?
			AND system_role IS NULL
	`, params.BookID, params.AccountID)
	if err != nil {
		return mapAccountDeleteError(fmt.Errorf("delete account: %w", err))
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted account rows: %w", err)
	}
	if rowsAffected != 1 {
		return ErrAccountDeleteNotPermitted
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete account transaction: %w", err)
	}
	committed = true

	return nil
}

func (r *AccountRepository) EnsureSystemAccounts(ctx context.Context, params EnsureSystemAccountsParams) (EnsureSystemAccountsRecord, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("begin system accounts setup transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackTx(ctx, tx)
		}
	}()

	var completedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT completed_at
		FROM setup_steps
		WHERE step_key = 'system_accounts'
	`).Scan(&completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EnsureSystemAccountsRecord{}, ErrNotFound
		}
		return EnsureSystemAccountsRecord{}, fmt.Errorf("read system accounts setup step: %w", err)
	}
	if completedAt.Valid {
		return EnsureSystemAccountsRecord{}, ErrSystemAccountsSetupComplete
	}

	if _, err := readBookForUpdate(ctx, tx, params.BookID); err != nil {
		return EnsureSystemAccountsRecord{}, err
	}

	auditEventID, err := insertAuditEvent(ctx, tx, AuditEventParams{
		BookID:        params.BookID,
		ActorUserID:   params.ChangedByUserID,
		AuthSessionID: params.AuthSessionID,
		OccurredAt:    params.CreatedAt,
		RequestID:     params.RequestID,
		OriginType:    params.OriginType,
		Operation:     params.Operation,
		Reason:        "seeded system and starter accounts",
	})
	if err != nil {
		return EnsureSystemAccountsRecord{}, err
	}

	records := make([]AccountRecord, 0, len(params.Specs)+len(params.StarterCashSpecs))
	for _, spec := range params.Specs {
		record, err := r.ensureSystemAccount(ctx, tx, params, spec, auditEventID)
		if err != nil {
			return EnsureSystemAccountsRecord{}, err
		}
		records = append(records, record)
	}
	for _, spec := range params.StarterCashSpecs {
		record, err := r.ensureStarterCashAccount(ctx, tx, params, spec, auditEventID)
		if err != nil {
			return EnsureSystemAccountsRecord{}, err
		}
		records = append(records, record)
	}
	// The pre-release investment contract resolves an unspecified ordinary
	// commission through the book's versioned clearing-included fallback.
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO investment_fee_policies
			(book_id, account_id, charge_kind, created_at, created_audit_event_id)
		VALUES (?, NULL, 'commission', ?, ?)
	`, params.BookID, params.CreatedAt, auditEventID)
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("seed commission treatment policy: %w", err)
	}
	var policyID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM investment_fee_policies
		WHERE book_id = ? AND account_id IS NULL AND charge_kind = 'commission'
	`, params.BookID).Scan(&policyID); err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("read commission treatment policy id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO investment_fee_policy_versions
			(policy_id, version_seq, effective_from, treatment, recorded_at, audit_event_id)
		VALUES (?, 1, '0001-01-01', 'clearing_included', ?, ?)
	`, policyID, params.CreatedAt, auditEventID); err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("seed commission treatment version: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE setup_steps
		SET completed_at = ?, completed_audit_event_id = ?
		WHERE step_key = 'system_accounts' AND completed_at IS NULL
	`, params.CreatedAt, auditEventID)
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("mark system accounts setup complete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("read system accounts setup rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("mark system accounts setup complete: updated %d rows", rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return EnsureSystemAccountsRecord{}, fmt.Errorf("commit system accounts setup transaction: %w", err)
	}
	committed = true

	return EnsureSystemAccountsRecord{Accounts: records}, nil
}

func (r *AccountRepository) accountSpecWithEnsuredCurrency(ctx context.Context, tx *sql.Tx, spec AccountSpec, params CreateAccountParams, auditEventID int64) (AccountSpec, error) {
	if spec.EnsureDefaultCurrency == nil {
		return spec, nil
	}

	record, err := ensureCurrency(ctx, tx, CreateCurrencyParams{
		BookID:          params.BookID,
		CreatedByUserID: params.CreatedByUserID,
		AuthSessionID:   params.AuthSessionID,
		RequestID:       params.RequestID,
		OriginType:      params.OriginType,
		Operation:       params.Operation,
		Spec:            *spec.EnsureDefaultCurrency,
		CreatedAt:       params.CreatedAt,
		EffectiveFrom:   params.EffectiveFrom,
	}, auditEventID)
	if err != nil {
		return AccountSpec{}, err
	}

	spec.DefaultCommodityID = sql.NullInt64{Int64: record.ID, Valid: true}
	spec.EnsureDefaultCurrency = nil
	return spec, nil
}

func (r *AccountRepository) WouldCreateCycle(ctx context.Context, bookID int64, accountID int64, parentAccountID int64) (bool, error) {
	if accountID <= 0 || parentAccountID <= 0 {
		return false, nil
	}

	rows, err := r.database.QueryContext(ctx, `
		WITH RECURSIVE ancestors(id, parent_id) AS (
			SELECT a.id, av.parent_account_id
			FROM accounts a
			JOIN `+CurrentReferenceVersions(ctx, "account")+` av ON av.account_id = a.id
			WHERE a.book_id = ?
				AND a.id = ?
			UNION ALL
			SELECT parent.id, parent_version.parent_account_id
			FROM accounts parent
			JOIN `+CurrentReferenceVersions(ctx, "account")+` parent_version ON parent_version.account_id = parent.id
			JOIN ancestors ON ancestors.parent_id = parent.id
			WHERE parent.book_id = ?
		)
		SELECT id
		FROM ancestors
	`, bookID, parentAccountID, bookID)
	if err != nil {
		return false, fmt.Errorf("read account ancestors: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ancestorID int64
		if err := rows.Scan(&ancestorID); err != nil {
			return false, fmt.Errorf("scan account ancestor: %w", err)
		}
		if ancestorID == accountID {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate account ancestors: %w", err)
	}

	return false, nil
}

func (r *AccountRepository) CurrentCommodityByID(ctx context.Context, bookID int64, commodityID int64) (AccountCommodityRecord, error) {
	var record AccountCommodityRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT c.id, c.book_id, c.code, c.kind, cv.status, cv.max_quantity_scale
		FROM commodities c
		JOIN `+CurrentReferenceVersions(ctx, "commodity")+` cv ON cv.commodity_id = c.id
		WHERE 1 = 1
			AND c.book_id = ?
			AND c.id = ?
	`, bookID, commodityID).Scan(&record.ID, &record.BookID, &record.Code, &record.Kind, &record.Status, &record.MaxQuantityScale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountCommodityRecord{}, ErrNotFound
		}
		return AccountCommodityRecord{}, fmt.Errorf("read account commodity: %w", err)
	}

	return record, nil
}

func (r *AccountRepository) CurrentCurrencyByCode(ctx context.Context, bookID int64, code string) (AccountCommodityRecord, error) {
	var record AccountCommodityRecord
	if err := r.database.QueryRowContext(ctx, `
		SELECT c.id, c.book_id, c.code, c.kind, cv.status, cv.max_quantity_scale
		FROM commodities c
		JOIN `+CurrentReferenceVersions(ctx, "commodity")+` cv ON cv.commodity_id = c.id
		WHERE 1 = 1
			AND c.book_id = ?
			AND c.kind = 'currency'
			AND c.code = ?
	`, bookID, code).Scan(&record.ID, &record.BookID, &record.Code, &record.Kind, &record.Status, &record.MaxQuantityScale); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountCommodityRecord{}, ErrNotFound
		}
		return AccountCommodityRecord{}, fmt.Errorf("read account currency by code: %w", err)
	}

	return record, nil
}

func (r *AccountRepository) AccountHasPostings(ctx context.Context, bookID int64, accountID int64) (bool, error) {
	return accountHasPostings(ctx, r.database, bookID, accountID)
}

// rowQueryer is satisfied by both *sql.DB and *sql.Tx, so a question can be
// asked either outside a write or inside one without a second copy of the query.
type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func accountHasPostings(ctx context.Context, queryer rowQueryer, bookID int64, accountID int64) (bool, error) {
	var exists int
	if err := queryer.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM posting_versions
			WHERE book_id = ?
				AND account_id = ?
		)
	`, bookID, accountID).Scan(&exists); err != nil {
		return false, fmt.Errorf("read account posting existence: %w", err)
	}

	return exists == 1, nil
}

func (r *AccountRepository) AccountKindByCode(ctx context.Context, code string) (AccountKindRecord, error) {
	var record AccountKindRecord
	var isBuiltin int
	var isUserAssignable int
	if err := r.database.QueryRowContext(ctx, `
		SELECT
			code,
			account_class,
			base_kind,
			is_builtin,
			is_user_assignable,
			display_key,
			sort_order
		FROM account_kinds
		WHERE code = ?
	`, code).Scan(
		&record.Code,
		&record.AccountClass,
		&record.BaseKind,
		&isBuiltin,
		&isUserAssignable,
		&record.DisplayKey,
		&record.SortOrder,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountKindRecord{}, ErrNotFound
		}
		return AccountKindRecord{}, fmt.Errorf("read account kind: %w", err)
	}

	record.IsBuiltin = isBuiltin == 1
	record.IsUserAssignable = isUserAssignable == 1
	return record, nil
}

func (r *AccountRepository) ListActiveCurrencyStarterSpecs(ctx context.Context, bookID int64) ([]StarterCashAccountSpec, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT c.id, c.code
		FROM commodities c
		JOIN `+CurrentReferenceVersions(ctx, "commodity")+` cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
			AND c.kind = 'currency'
			AND cv.status = 'active'
		ORDER BY c.code
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("list active currency starter specs: %w", err)
	}
	defer rows.Close()

	var specs []StarterCashAccountSpec
	for rows.Next() {
		var spec StarterCashAccountSpec
		if err := rows.Scan(&spec.CommodityID, &spec.CurrencyCode); err != nil {
			return nil, fmt.Errorf("scan active currency starter spec: %w", err)
		}
		specs = append(specs, spec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active currency starter specs: %w", err)
	}

	return specs, nil
}

func (r *AccountRepository) ensureSystemAccount(ctx context.Context, tx *sql.Tx, params EnsureSystemAccountsParams, systemSpec SystemAccountSpec, auditEventID int64) (AccountRecord, error) {
	record, err := currentSystemAccountByRole(ctx, tx, params.BookID, systemSpec.Role)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, systemSpec.Role, params.CreatedAt, params.ChangedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert system account: %w", err)
	}

	accountID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read system account id: %w", err)
	}

	return insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          accountID,
		CreatedAt:          params.CreatedAt,
		CreatedByUserID:    params.ChangedByUserID,
		VersionSeq:         1,
		EffectiveFrom:      params.EffectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       "seeded system account",
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec: AccountSpec{
			Code: systemSpec.Role,
			// System account display names are derived from system_role at the
			// frontend localization boundary; do not seed English names here.
			AccountClass:   systemSpec.AccountClass,
			AccountKind:    systemSpec.AccountKind,
			AllowsPostings: true,
			MetadataJSON:   `{"source":"system_account_seed"}`,
			OpenedOn:       params.EffectiveFrom,
		},
	})
}

func (r *AccountRepository) ensureStarterCashAccount(ctx context.Context, tx *sql.Tx, params EnsureSystemAccountsParams, starterSpec StarterCashAccountSpec, auditEventID int64) (AccountRecord, error) {
	record, err := currentStarterCashAccountByCommodityID(ctx, tx, params.BookID, starterSpec.CommodityID)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return AccountRecord{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (book_id, system_role, created_at, created_by_user_id, created_request_id, created_audit_event_id)
		VALUES (?, NULL, ?, ?, NULLIF(?, ''), ?)
	`, params.BookID, params.CreatedAt, params.ChangedByUserID, params.RequestID, auditEventID)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert starter cash account: %w", err)
	}

	accountID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read starter cash account id: %w", err)
	}

	effectiveFrom := params.StarterEffectiveFrom
	if effectiveFrom == "" {
		effectiveFrom = params.CreatedAt[:10]
	}
	metadataJSON, err := starterCashMetadataJSON(starterSpec.CurrencyCode)
	if err != nil {
		return AccountRecord{}, err
	}

	return insertAccountVersion(ctx, tx, insertAccountVersionParams{
		BookID:             params.BookID,
		AccountID:          accountID,
		CreatedAt:          params.CreatedAt,
		CreatedByUserID:    params.ChangedByUserID,
		VersionSeq:         1,
		EffectiveFrom:      effectiveFrom,
		RecordedAt:         params.CreatedAt,
		ChangedByUserID:    params.ChangedByUserID,
		ChangeReason:       "seeded starter cash account",
		ChangeAuditEventID: auditEventID,
		Status:             "active",
		Spec: AccountSpec{
			Code:               "cash:" + starterSpec.CurrencyCode,
			AccountClass:       "asset",
			AccountKind:        "cash",
			DefaultCommodityID: sql.NullInt64{Int64: starterSpec.CommodityID, Valid: true},
			AllowsPostings:     true,
			MetadataJSON:       metadataJSON,
			OpenedOn:           effectiveFrom,
		},
	})
}

func starterCashMetadataJSON(currencyCode string) (string, error) {
	payload := struct {
		Source       string `json:"source"`
		StarterKey   string `json:"starter_key"`
		CurrencyCode string `json:"currency_code"`
	}{
		Source:       "starter_account_seed",
		StarterKey:   "cash",
		CurrencyCode: currencyCode,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode starter cash account metadata: %w", err)
	}

	return string(encoded), nil
}

type insertAccountVersionParams struct {
	BookID             int64
	AccountID          int64
	CreatedAt          string
	CreatedByUserID    int64
	VersionSeq         int64
	EffectiveFrom      string
	RecordedAt         string
	ChangedByUserID    int64
	ChangeReason       string
	ChangeAuditEventID int64
	Status             string
	Spec               AccountSpec
}

func insertAccountVersion(ctx context.Context, tx *sql.Tx, params insertAccountVersionParams) (AccountRecord, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO account_versions (
			account_id,
			version_seq,
			effective_from,
			recorded_at,
			changed_by_user_id,
			change_reason,
			status,
			opened_on,
			closed_on,
			code,
			name,
			account_class,
			account_kind,
			parent_account_id,
			institution_id,
			country_code,
			default_commodity_id,
			quantity_scale_override,
			allows_postings,
			number_last4,
			external_ref_hint,
			comment_markdown,
			metadata_json,
			change_audit_event_id,
			cost_basis_method
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''))
	`, params.AccountID, params.VersionSeq, params.EffectiveFrom, params.RecordedAt, params.ChangedByUserID, params.ChangeReason, params.Status, params.Spec.OpenedOn, nullableStringValue(params.Spec.ClosedOn), params.Spec.Code, params.Spec.Name, params.Spec.AccountClass, params.Spec.AccountKind, nullableInt64Value(params.Spec.ParentAccountID), nullableInt64Value(params.Spec.InstitutionID), params.Spec.CountryCode, nullableInt64Value(params.Spec.DefaultCommodityID), nullableInt64Value(params.Spec.QuantityScaleOverride), boolToInt(params.Spec.AllowsPostings), params.Spec.NumberLast4, params.Spec.ExternalRefHint, params.Spec.CommentMarkdown, params.Spec.MetadataJSON, params.ChangeAuditEventID, params.Spec.CostBasisMethod)
	if err != nil {
		return AccountRecord{}, fmt.Errorf("insert account version: %w", err)
	}

	versionID, err := result.LastInsertId()
	if err != nil {
		return AccountRecord{}, fmt.Errorf("read account version id: %w", err)
	}

	record, err := accountVersionByVersionID(ctx, tx, params.BookID, versionID)
	if err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func accountVersionByVersionID(ctx context.Context, tx *sql.Tx, bookID int64, versionID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, accountSelect("account_versions", `
		WHERE a.book_id = ? AND av.id = ?
	`), bookID, versionID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentAccountByID(ctx context.Context, tx *sql.Tx, bookID int64, accountID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(ctx, `
		AND a.book_id = ? AND a.id = ?
	`), bookID, accountID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentSystemAccountByRole(ctx context.Context, tx *sql.Tx, bookID int64, role string) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(ctx, `
		AND a.book_id = ? AND a.system_role = ?
	`), bookID, role), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentStarterCashAccountByCommodityID(ctx context.Context, tx *sql.Tx, bookID int64, commodityID int64) (AccountRecord, error) {
	var record AccountRecord
	if err := scanAccountRecord(tx.QueryRowContext(ctx, currentAccountSelect(ctx, `
		AND a.book_id = ?
		AND a.system_role IS NULL
		AND av.default_commodity_id = ?
		AND json_extract(av.metadata_json, '$.source') = 'starter_account_seed'
		AND json_extract(av.metadata_json, '$.starter_key') = 'cash'
	`), bookID, commodityID), &record); err != nil {
		return AccountRecord{}, err
	}

	return record, nil
}

func currentAccountSelect(ctx context.Context, extraConditions string) string {
	return accountSelect(CurrentReferenceVersions(ctx, "account"), `
		WHERE 1 = 1
	`+extraConditions)
}

func accountSelect(versionSource string, whereClause string) string {
	return `
		SELECT
			a.id,
			a.book_id,
			a.system_role,
			a.created_at,
			a.created_by_user_id,
			av.id,
			av.version_seq,
			av.effective_from,
			av.recorded_at,
			av.changed_by_user_id,
			av.change_reason,
			av.status,
			av.opened_on,
			av.closed_on,
			av.code,
			av.name,
			av.account_class,
			av.account_kind,
			av.parent_account_id,
			av.institution_id,
			av.country_code,
			av.default_commodity_id,
			av.quantity_scale_override,
			av.allows_postings,
			EXISTS (
				SELECT 1
				FROM posting_versions pv
				WHERE pv.account_id = a.id
			),
			av.number_last4,
			av.external_ref_hint,
			av.comment_markdown,
			av.metadata_json,
			av.cost_basis_method
		FROM accounts a
		JOIN ` + versionSource + ` av ON av.account_id = a.id
	` + whereClause
}

func scanAccountRecord(row rowScanner, record *AccountRecord) error {
	var allowsPostings int
	if err := row.Scan(
		&record.ID,
		&record.BookID,
		&record.SystemRole,
		&record.CreatedAt,
		&record.CreatedByUserID,
		&record.VersionID,
		&record.VersionSeq,
		&record.EffectiveFrom,
		&record.RecordedAt,
		&record.ChangedByUserID,
		&record.ChangeReason,
		&record.Status,
		&record.OpenedOn,
		&record.ClosedOn,
		&record.Code,
		&record.Name,
		&record.AccountClass,
		&record.AccountKind,
		&record.ParentAccountID,
		&record.InstitutionID,
		&record.CountryCode,
		&record.DefaultCommodityID,
		&record.QuantityScaleOverride,
		&allowsPostings,
		&record.HasActivity,
		&record.NumberLast4,
		&record.ExternalRefHint,
		&record.CommentMarkdown,
		&record.MetadataJSON,
		&record.CostBasisMethod,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("scan account: %w", err)
	}

	record.IsSystem = record.SystemRole.Valid
	record.AllowsPostings = allowsPostings == 1
	return nil
}

func scanAccountRecords(rows *sql.Rows) ([]AccountRecord, error) {
	var records []AccountRecord
	for rows.Next() {
		var record AccountRecord
		if err := scanAccountRecord(rows, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	return records, nil
}

func mapAccountDeleteError(err error) error {
	message := err.Error()
	if strings.Contains(message, "account_versions rows are append-only") ||
		strings.Contains(message, "account is referenced by allocation history") ||
		isSQLiteForeignKeyConstraint(err) {
		return fmt.Errorf("%w: %v", ErrAccountDeleteNotPermitted, err)
	}

	return err
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64Value(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableStringValue(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
