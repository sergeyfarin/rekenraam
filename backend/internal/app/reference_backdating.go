package app

import (
	"errors"

	"rekenraam/backend/internal/db"
)

var (
	ErrReferenceVersionBackdated = errors.New("reference version is backdated")
	// ErrReferenceVersionBeforeFirst: the requested date precedes the
	// record's first version, so there is no version to correct there.
	ErrReferenceVersionBeforeFirst = errors.New("reference version is dated before the record's first version")
	// ErrReferenceVersionChanged: a version was added between the read that
	// filled omitted fields and the write; the client reloads and retries.
	ErrReferenceVersionChanged = errors.New("the record changed while this edit was prepared; reload it and try again")
)

// ReferenceVersionBackdatedError is ErrReferenceVersionBackdated with the
// latest version's date: the earliest date a lifecycle or structural change
// may take effect, which the client shows in its refusal.
type ReferenceVersionBackdatedError struct {
	LatestEffectiveFrom string
	detail              string
}

func (e ReferenceVersionBackdatedError) Error() string {
	return ErrReferenceVersionBackdated.Error() + ": " + e.detail
}

func (e ReferenceVersionBackdatedError) Unwrap() error { return ErrReferenceVersionBackdated }

// ReferenceVersionBeforeFirstError is ErrReferenceVersionBeforeFirst with the
// first version's date: the earliest date any change may take effect.
type ReferenceVersionBeforeFirstError struct {
	FirstEffectiveFrom string
	detail             string
}

func (e ReferenceVersionBeforeFirstError) Error() string {
	return ErrReferenceVersionBeforeFirst.Error() + ": " + e.detail
}

func (e ReferenceVersionBeforeFirstError) Unwrap() error { return ErrReferenceVersionBeforeFirst }

// ReferenceRuleConflictError is a refusal by a rule that involves another
// entity, on a date the new version governs (#552). It unwraps to the rule's
// existing service error (ErrCategoryExists, ErrPayeeExists,
// ErrCategoryParentInvalid, ErrCategoryHasChildren, ErrInstitutionInUse).
type ReferenceRuleConflictError struct {
	Rule       string
	Date       string
	Name       string
	BuiltinKey string
	Err        error
}

func (e ReferenceRuleConflictError) Error() string {
	return e.Err.Error() + " on " + e.Date
}

func (e ReferenceRuleConflictError) Unwrap() error { return e.Err }

var referenceRuleErrors = map[string]error{
	db.ReferenceRuleCategoryNameTaken:        ErrCategoryExists,
	db.ReferenceRulePayeeNameTaken:           ErrPayeeExists,
	db.ReferenceRuleCategoryParentArchived:   ErrCategoryParentInvalid,
	db.ReferenceRuleCategoryParentCycle:      ErrCategoryParentInvalid,
	db.ReferenceRuleCategoryActiveChild:      ErrCategoryHasChildren,
	db.ReferenceRuleInstitutionLinkedAccount: ErrInstitutionInUse,
}

// referenceBackdatingError maps the shared reference-version refusals (#552).
// Callers try it before their own sentinel checks: a dated rule conflict also
// unwraps to the plain repository sentinel, which would lose its date.
func referenceBackdatingError(err error) (error, bool) {
	if backdated, ok := errors.AsType[db.ReferenceVersionBackdatedError](err); ok {
		return ReferenceVersionBackdatedError{LatestEffectiveFrom: backdated.LatestEffectiveFrom, detail: backdated.Error()}, true
	}
	if beforeFirst, ok := errors.AsType[db.ReferenceVersionBeforeFirstError](err); ok {
		return ReferenceVersionBeforeFirstError{FirstEffectiveFrom: beforeFirst.FirstEffectiveFrom, detail: beforeFirst.Error()}, true
	}
	if conflict, ok := errors.AsType[db.ReferenceRuleConflictError](err); ok {
		if mapped, known := referenceRuleErrors[conflict.Rule]; known {
			return ReferenceRuleConflictError{Rule: conflict.Rule, Date: conflict.Date, Name: conflict.Name, BuiltinKey: conflict.BuiltinKey, Err: mapped}, true
		}
	}
	if errors.Is(err, db.ErrReferenceVersionChanged) {
		return ErrReferenceVersionChanged, true
	}
	return nil, false
}
