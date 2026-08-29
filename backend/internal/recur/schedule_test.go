package recur_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/recur"
)

func weekday(day time.Weekday) *time.Weekday { return &day }

// The clamping regression this package exists for: a nominal day past the end
// of a short month clamps to that month's last day, and the series re-anchors
// on the nominal day afterwards. Computing each date from the previous
// *produced* date is what makes "the 31st" collapse to the 28th forever after
// one February.
func TestMonthlyOnThe31stClampsAndReanchors(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Monthly,
		IntervalCount: 1,
		DayOfMonth:    31,
		StartsOn:      "2026-01-31",
	}

	dates, err := recur.Occurrences(spec, "2026-01-01", "2026-06-30")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-01-31",
		"2026-02-28",
		"2026-03-31",
		"2026-04-30",
		"2026-05-31",
		"2026-06-30",
	}, dates)

	leap := spec
	leap.StartsOn = "2028-01-31"
	leapDates, err := recur.Occurrences(leap, "2028-01-01", "2028-03-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2028-01-31", "2028-02-29", "2028-03-31"}, leapDates)
}

func TestYearlyOnLeapDayClampsInCommonYears(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Yearly,
		IntervalCount: 1,
		MonthOfYear:   2,
		DayOfMonth:    29,
		StartsOn:      "2024-02-29",
	}

	dates, err := recur.Occurrences(spec, "2024-01-01", "2028-12-31")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2024-02-29",
		"2025-02-28",
		"2026-02-28",
		"2027-02-28",
		"2028-02-29",
	}, dates)
}

// A clamped February must not shift the phase of every month after it: an
// every-two-months series anchored on an odd month stays on odd months.
func TestIntervalGreaterThanOneKeepsPhaseAcrossAClampedMonth(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Monthly,
		IntervalCount: 2,
		DayOfMonth:    31,
		StartsOn:      "2026-01-31",
	}

	dates, err := recur.Occurrences(spec, "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-01-31",
		"2026-03-31",
		"2026-05-31",
		"2026-07-31",
		"2026-09-30",
		"2026-11-30",
	}, dates)
}

func TestWeeklyStartsOnTheFirstMatchingWeekdayOnOrAfterStart(t *testing.T) {
	// 2026-08-31 is a Monday; the first Friday on or after it is 2026-09-04.
	spec := recur.ScheduleSpec{
		Frequency:     recur.Weekly,
		IntervalCount: 1,
		ByWeekday:     weekday(time.Friday),
		StartsOn:      "2026-08-31",
	}

	dates, err := recur.Occurrences(spec, "2026-08-01", "2026-09-30")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09-04", "2026-09-11", "2026-09-18", "2026-09-25"}, dates)

	// An anchor already on the matching weekday is itself the first occurrence.
	onTheDay := spec
	onTheDay.StartsOn = "2026-09-04"
	dates, err = recur.Occurrences(onTheDay, "2026-09-01", "2026-09-12")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09-04", "2026-09-11"}, dates)
}

// The anchor's own month has already passed the nominal day, so the series
// starts one whole interval later — and every later index stays one interval
// apart. An earlier revision special-cased only the first occurrence, which
// made index 0 and index 1 land on the same date.
func TestMonthlySkipsTheAnchorMonthWhenItsDayHasPassedWithoutDuplicating(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Monthly,
		IntervalCount: 3,
		DayOfMonth:    10,
		StartsOn:      "2026-01-15",
	}

	dates, err := recur.Occurrences(spec, "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-04-10", "2026-07-10", "2026-10-10"}, dates)
}

func TestLastDayOfMonthFollowsTheMonthLength(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:      recur.Monthly,
		IntervalCount:  1,
		LastDayOfMonth: true,
		StartsOn:       "2026-01-01",
	}

	dates, err := recur.Occurrences(spec, "2026-01-01", "2026-04-30")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30"}, dates)
}

// MaxOccurrences bounds the series, not the answer: a window that opens
// mid-series returns the occurrences that fall inside it, never a fresh count
// of five starting from the window.
func TestMaxOccurrencesCountsFromTheAnchorNotTheWindow(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:      recur.Daily,
		IntervalCount:  1,
		StartsOn:       "2026-01-01",
		MaxOccurrences: 5,
	}

	dates, err := recur.Occurrences(spec, "2026-01-03", "2026-12-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-03", "2026-01-04", "2026-01-05"}, dates)
}

func TestEndsOnAndMaxOccurrencesTakeWhicheverEndsFirst(t *testing.T) {
	base := recur.ScheduleSpec{
		Frequency:     recur.Daily,
		IntervalCount: 1,
		StartsOn:      "2026-01-01",
	}

	endDateWins := base
	endDateWins.EndsOn = "2026-01-04"
	endDateWins.MaxOccurrences = 10
	dates, err := recur.Occurrences(endDateWins, "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-01", "2026-01-02", "2026-01-03", "2026-01-04"}, dates)

	countWins := base
	countWins.EndsOn = "2026-01-31"
	countWins.MaxOccurrences = 3
	dates, err = recur.Occurrences(countWins, "2026-01-01", "2026-12-31")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-01", "2026-01-02", "2026-01-03"}, dates)
}

func TestWindowIsInclusiveOfBothBounds(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Daily,
		IntervalCount: 1,
		StartsOn:      "2026-01-01",
	}

	dates, err := recur.Occurrences(spec, "2026-01-05", "2026-01-07")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-05", "2026-01-06", "2026-01-07"}, dates)

	sameDay, err := recur.Occurrences(spec, "2026-01-05", "2026-01-05")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-01-05"}, sameDay)

	inverted, err := recur.Occurrences(spec, "2026-01-07", "2026-01-05")
	require.NoError(t, err)
	assert.Empty(t, inverted)
}

// A date-only series has no 23- or 25-hour day. This pins that the
// implementation never routes through wall-clock arithmetic, where the
// Amsterdam spring-forward transition on 2026-03-29 would drop or duplicate a
// date.
func TestDailyAcrossADaylightSavingBoundaryProducesEveryDate(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Daily,
		IntervalCount: 1,
		StartsOn:      "2026-03-27",
	}

	dates, err := recur.Occurrences(spec, "2026-03-27", "2026-03-31")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-03-27",
		"2026-03-28",
		"2026-03-29",
		"2026-03-30",
		"2026-03-31",
	}, dates)
}

func TestNextReturnsTheFirstOccurrenceStrictlyAfterTheGivenDate(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Monthly,
		IntervalCount: 1,
		DayOfMonth:    1,
		StartsOn:      "2026-01-01",
	}

	next, ok, err := recur.Next(spec, "2026-03-01")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "2026-04-01", next)

	ended := spec
	ended.EndsOn = "2026-03-01"
	_, ok, err = recur.Next(ended, "2026-03-01")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestValidateRejectsUnusableSchedules(t *testing.T) {
	base := recur.ScheduleSpec{
		Frequency:     recur.Monthly,
		IntervalCount: 1,
		DayOfMonth:    1,
		StartsOn:      "2026-01-01",
	}
	require.NoError(t, base.Validate())

	cases := map[string]func(recur.ScheduleSpec) recur.ScheduleSpec{
		"unknown frequency":            func(s recur.ScheduleSpec) recur.ScheduleSpec { s.Frequency = "fortnightly"; return s },
		"zero interval":                func(s recur.ScheduleSpec) recur.ScheduleSpec { s.IntervalCount = 0; return s },
		"weekly without a weekday":     func(s recur.ScheduleSpec) recur.ScheduleSpec { s.Frequency = recur.Weekly; return s },
		"monthly without a day":        func(s recur.ScheduleSpec) recur.ScheduleSpec { s.DayOfMonth = 0; return s },
		"day of month and last day":    func(s recur.ScheduleSpec) recur.ScheduleSpec { s.LastDayOfMonth = true; return s },
		"day of month out of range":    func(s recur.ScheduleSpec) recur.ScheduleSpec { s.DayOfMonth = 32; return s },
		"yearly without a month":       func(s recur.ScheduleSpec) recur.ScheduleSpec { s.Frequency = recur.Yearly; return s },
		"ends before it starts":        func(s recur.ScheduleSpec) recur.ScheduleSpec { s.EndsOn = "2025-12-31"; return s },
		"start date is not a date":     func(s recur.ScheduleSpec) recur.ScheduleSpec { s.StartsOn = "01/01/2026"; return s },
		"start date does not exist":    func(s recur.ScheduleSpec) recur.ScheduleSpec { s.StartsOn = "2026-02-30"; return s },
		"negative maximum occurrences": func(s recur.ScheduleSpec) recur.ScheduleSpec { s.MaxOccurrences = -1; return s },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			require.Error(t, mutate(base).Validate())
		})
	}
}

func TestOccurrencesRejectsAWindowThatIsNotACalendarDate(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Daily,
		IntervalCount: 1,
		StartsOn:      "2026-01-01",
	}

	_, err := recur.Occurrences(spec, "2026-02-30", "2026-03-31")
	require.ErrorIs(t, err, recur.ErrDateInvalid)

	_, err = recur.Occurrences(spec, "2026-01-01", "not-a-date")
	require.ErrorIs(t, err, recur.ErrDateInvalid)
}

// An unbounded daily series over a decade-wide window must refuse rather than
// grow a slice from user-controlled input.
func TestOccurrencesRefusesAWindowBeyondTheCap(t *testing.T) {
	spec := recur.ScheduleSpec{
		Frequency:     recur.Daily,
		IntervalCount: 1,
		StartsOn:      "2020-01-01",
	}

	_, err := recur.Occurrences(spec, "2020-01-01", "2040-01-01")
	require.ErrorIs(t, err, recur.ErrWindowTooWide)
}
