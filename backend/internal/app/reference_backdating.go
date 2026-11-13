package app

import (
	"errors"
	"fmt"

	"rekenraam/backend/internal/db"
)

var ErrReferenceVersionBackdated = errors.New("reference version is backdated")

func referenceBackdatingError(err error) (error, bool) {
	if backdated, ok := errors.AsType[db.ReferenceVersionBackdatedError](err); ok {
		return fmt.Errorf("%w: %s", ErrReferenceVersionBackdated, backdated.Error()), true
	}
	return nil, false
}
