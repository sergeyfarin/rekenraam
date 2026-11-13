package app

import (
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

var (
	ErrReferenceVersionBackdated = errors.New("reference version is backdated")
	// ErrReferenceVersionChanged: a version was added between the read that
	// filled omitted fields and the write; the client reloads and retries.
	ErrReferenceVersionChanged = errors.New("the record changed while this edit was prepared; reload it and try again")
)

// referenceBackdatingError maps the shared reference-version refusals (#552).
func referenceBackdatingError(err error) (error, bool) {
	if backdated, ok := errors.AsType[db.ReferenceVersionBackdatedError](err); ok {
		return fmt.Errorf("%w: %s", ErrReferenceVersionBackdated, backdated.Error()), true
	}
	if errors.Is(err, db.ErrReferenceVersionChanged) {
		return ErrReferenceVersionChanged, true
	}
	return nil, false
}
