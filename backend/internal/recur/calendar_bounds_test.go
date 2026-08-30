package recur_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/recur"
)

func TestNextStopsAtLastRepresentableCalendarDate(t *testing.T) {
	spec := recur.ScheduleSpec{Frequency: recur.Daily, IntervalCount: 1, StartsOn: "9999-12-31"}
	next, found, err := recur.Next(spec, "9999-12-31")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, next)
}

func TestRecurringScheduleRejectsYearZero(t *testing.T) {
	spec := recur.ScheduleSpec{Frequency: recur.Daily, IntervalCount: 1, StartsOn: "0000-01-01"}
	require.ErrorIs(t, spec.Validate(), recur.ErrDateInvalid)
}
