package api

import (
	"errors"
	"net/http"

	"rekenraam/backend/internal/app"
)

// writeReferenceVersionError writes the shared reference-data versioning
// refusals (#552) and reports whether err was one of them. Handlers call it
// before their own cases: a dated rule conflict also unwraps to the plain
// service error (for example ErrCategoryExists), which would drop its date.
func writeReferenceVersionError(w http.ResponseWriter, err error) bool {
	if conflict, ok := errors.AsType[app.ReferenceRuleConflictError](err); ok {
		writeJSON(w, http.StatusConflict, errorResponse{Error: errorBody{
			Code:               "REFERENCE_DATED_CONFLICT",
			Message:            conflict.Error(),
			ConflictRule:       conflict.Rule,
			ConflictDate:       conflict.Date,
			ConflictName:       conflict.Name,
			ConflictBuiltinKey: conflict.BuiltinKey,
		}})
		return true
	}
	if backdated, ok := errors.AsType[app.ReferenceVersionBackdatedError](err); ok {
		writeJSON(w, http.StatusConflict, errorResponse{Error: errorBody{
			Code:         "REFERENCE_VERSION_BACKDATED",
			Message:      backdated.Error(),
			ConflictDate: backdated.LatestEffectiveFrom,
		}})
		return true
	}
	if beforeFirst, ok := errors.AsType[app.ReferenceVersionBeforeFirstError](err); ok {
		writeJSON(w, http.StatusConflict, errorResponse{Error: errorBody{
			Code:         "REFERENCE_VERSION_BEFORE_FIRST",
			Message:      beforeFirst.Error(),
			ConflictDate: beforeFirst.FirstEffectiveFrom,
		}})
		return true
	}
	if errors.Is(err, app.ErrReferenceVersionChanged) {
		writeAPIError(w, http.StatusConflict, "CONFLICT", err.Error())
		return true
	}
	return false
}
