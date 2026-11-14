package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReferenceVersionBackdatedError refuses changes that cannot take effect in
// the requested historical window (#552). Refusals precede audit insertion.
type ReferenceVersionBackdatedError struct {
	LatestEffectiveFrom string
}

func (e ReferenceVersionBackdatedError) Error() string {
	return fmt.Sprintf("lifecycle or structural changes must take effect on or after %s", e.LatestEffectiveFrom)
}

// ReferenceVersionBeforeFirstError refuses a write dated before the entity's
// first version: no version is in effect on that date to correct. Unlike the
// lifecycle refusal, any date on or after the first version is acceptable for
// a descriptive correction, so it names the first date, not the latest.
type ReferenceVersionBeforeFirstError struct {
	FirstEffectiveFrom string
}

func (e ReferenceVersionBeforeFirstError) Error() string {
	return fmt.Sprintf("the record starts on %s", e.FirstEffectiveFrom)
}

// Reference rules a dated write can break on a day other than today. The
// values are stable API identifiers (conflict_rule); the client explains each.
const (
	ReferenceRuleCategoryNameTaken        = "category_name_taken"
	ReferenceRulePayeeNameTaken           = "payee_name_taken"
	ReferenceRuleCategoryParentArchived   = "category_parent_archived"
	ReferenceRuleCategoryParentCycle      = "category_parent_cycle"
	ReferenceRuleCategoryActiveChild      = "category_active_child"
	ReferenceRuleInstitutionLinkedAccount = "institution_linked_account"
)

// ReferenceRuleConflictError refuses a write that breaks a rule involving
// another entity on a date the new version governs (#552). That date may lie
// between the requested date and today, where today's state does not show
// the cause, so the refusal names the rule, the first failing date and the
// other entity. Err is the rule's own sentinel, so errors.Is keeps working.
type ReferenceRuleConflictError struct {
	Rule string
	Date string
	// Name is the other entity's name on Date; BuiltinKey identifies an
	// unnamed built-in category, which the client labels in its own locale.
	Name       string
	BuiltinKey string
	Err        error
}

func (e ReferenceRuleConflictError) Error() string {
	return fmt.Sprintf("%v on %s", e.Err, e.Date)
}

func (e ReferenceRuleConflictError) Unwrap() error { return e.Err }

// ErrReferenceVersionChanged refuses a write whose omitted fields were filled
// from a version that is no longer the one in effect on the requested date: a
// version was added between the service's read and the write transaction.
var ErrReferenceVersionChanged = errors.New("the record changed while this edit was prepared; reload it and try again")

// referenceVersionTable names a versioned reference table and its entity key.
// Only the fixed values below are used, so the names are safe to splice.
type referenceVersionTable struct {
	table, key string
}

var (
	accountVersionTable     = referenceVersionTable{table: "account_versions", key: "account_id"}
	payeeVersionTable       = referenceVersionTable{table: "payee_versions", key: "payee_id"}
	institutionVersionTable = referenceVersionTable{table: "institution_versions", key: "institution_id"}
)

// referenceVersionBeforeFirst builds the before-first refusal for an entity
// whose as-of read on the requested date found no version.
func referenceVersionBeforeFirst(ctx context.Context, q rowQueryer, own referenceVersionTable, id int64) error {
	var first sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT MIN(effective_from) FROM `+own.table+` WHERE `+own.key+` = ?`, id).Scan(&first); err != nil {
		return fmt.Errorf("read first %s date: %w", own.table, err)
	}
	if !first.Valid {
		return ErrNotFound
	}
	return ReferenceVersionBeforeFirstError{FirstEffectiveFrom: first.String}
}

func latestReferenceVersionDateTx(ctx context.Context, tx *sql.Tx, own referenceVersionTable, id int64) (string, error) {
	var latest sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MAX(effective_from) FROM `+own.table+` WHERE `+own.key+` = ?`, id).Scan(&latest); err != nil {
		return "", fmt.Errorf("read latest %s date: %w", own.table, err)
	}
	return latest.String, nil
}

// referenceDateSource selects the effective_from dates of other versions that
// can make a rule fail: the versions that take effect with the state the rule
// refuses (a sibling taking the name, the parent changing, a child pointing
// here). The query selects a single effective_from column.
type referenceDateSource struct {
	query string
	args  []any
}

// categoryNameDates: versions that give a category this name under this
// parent, the only versions that can take the name on some date.
func categoryNameDates(bookID int64, categoryType string, parentID sql.NullInt64, name string) referenceDateSource {
	return referenceDateSource{query: `
		SELECT av.effective_from FROM account_versions av JOIN accounts a ON a.id = av.account_id
		WHERE a.book_id = ? AND av.account_class = ? AND av.parent_account_id IS ? AND lower(av.name) = lower(?)`,
		args: []any{bookID, categoryType, parentID, name}}
}

// accountOwnDates: the versions of one account (or category), whose own
// state is what the rule reads.
func accountOwnDates(accountID int64) referenceDateSource {
	return referenceDateSource{query: `SELECT effective_from FROM account_versions WHERE account_id = ?`, args: []any{accountID}}
}

// categoryChildDates: versions that place a category under this parent.
func categoryChildDates(parentID int64) referenceDateSource {
	return referenceDateSource{query: `SELECT effective_from FROM account_versions WHERE parent_account_id = ?`, args: []any{parentID}}
}

// payeeNameDates: versions that give a payee this normalized name.
func payeeNameDates(bookID int64, normalizedName string) referenceDateSource {
	return referenceDateSource{query: `
		SELECT pv.effective_from FROM payee_versions pv JOIN payees p ON p.id = pv.payee_id
		WHERE p.book_id = ? AND pv.normalized_name = ?`,
		args: []any{bookID, normalizedName}}
}

// institutionAccountDates: versions that link an account to this institution.
func institutionAccountDates(institutionID int64) referenceDateSource {
	return referenceDateSource{query: `SELECT effective_from FROM account_versions WHERE institution_id = ?`, args: []any{institutionID}}
}

// referenceWindowDatesTx lists the dates on which a version of entityID dated
// effectiveFrom must satisfy a rule that depends on other entities (#552). The
// version governs from effectiveFrom until the entity's next later version, or
// open-ended (through today) when there is none. A rule that holds on
// effectiveFrom can only start failing on a date when another version takes
// effect with the state the rule refuses, so the version's own date plus each
// such date from the sources inside the window covers every day it governs.
// Checking only effectiveFrom, or only today, misses the rest of the window.
// Call it before the new version is inserted.
func referenceWindowDatesTx(ctx context.Context, tx *sql.Tx, own referenceVersionTable, entityID int64, effectiveFrom string, sources ...referenceDateSource) ([]string, error) {
	var end sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MIN(effective_from) FROM `+own.table+` WHERE `+own.key+` = ? AND effective_from > ?`, entityID, effectiveFrom).Scan(&end); err != nil {
		return nil, fmt.Errorf("read %s window end: %w", own.table, err)
	}
	query := `SELECT ?`
	args := []any{effectiveFrom}
	for _, source := range sources {
		query += ` UNION SELECT effective_from FROM (` + source.query + `) WHERE effective_from > ? AND (? IS NULL OR effective_from < ?)`
		args = append(args, source.args...)
		args = append(args, effectiveFrom, end, end)
	}
	rows, err := tx.QueryContext(ctx, query+` ORDER BY 1`, args...)
	if err != nil {
		return nil, fmt.Errorf("read %s window dates: %w", own.table, err)
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("scan %s window date: %w", own.table, err)
		}
		dates = append(dates, date)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s window dates: %w", own.table, err)
	}
	return dates, nil
}

// requireBaseVersionTx refuses the write when the version in effect on the
// requested date is not the one the caller filled omitted fields from. A zero
// base means the caller sent a complete snapshot and has nothing to go stale.
func requireBaseVersionTx(baseVersionID int64, asOfVersionID int64) error {
	if baseVersionID != 0 && baseVersionID != asOfVersionID {
		return ErrReferenceVersionChanged
	}
	return nil
}

// VersionBeforeFirst returns the before-first refusal for a category the
// service found today but not on the requested date, so the coded 409 comes
// before any validation of fields filled from today's version.
func (r *CategoryRepository) VersionBeforeFirst(ctx context.Context, categoryID int64) error {
	return referenceVersionBeforeFirst(ctx, r.database, accountVersionTable, categoryID)
}

// VersionBeforeFirst is the payee counterpart of
// CategoryRepository.VersionBeforeFirst.
func (r *PayeeRepository) VersionBeforeFirst(ctx context.Context, payeeID int64) error {
	return referenceVersionBeforeFirst(ctx, r.database, payeeVersionTable, payeeID)
}

// VersionBeforeFirst is the institution counterpart of
// CategoryRepository.VersionBeforeFirst.
func (r *InstitutionRepository) VersionBeforeFirst(ctx context.Context, institutionID int64) error {
	return referenceVersionBeforeFirst(ctx, r.database, institutionVersionTable, institutionID)
}
