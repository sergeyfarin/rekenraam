// Package recur enumerates the calendar dates of a recurring schedule.
//
// It is deliberately pure: no clock, no context, no database, no book. The
// generator (app.RecurringService) supplies the window and decides what to do
// with the dates; R10's projections call the same function for the future it
// has not materialized. One implementation means one set of month-clamping
// tests, which is the whole reason this is a package and not a method.
//
// Dates are ISO YYYY-MM-DD strings on the boundary, matching every other
// calendar date in this codebase (docs/conventions.md § Data). Internally the
// arithmetic runs on time.Time pinned to UTC — safe precisely because nothing
// here is a wall-clock instant, so no daylight-saving transition can shorten a
// day out from under it.
package recur

import (
	"errors"
	"fmt"
	"time"
)

// Frequency is the recurrence unit. IntervalCount multiplies it.
type Frequency string

const (
	Daily   Frequency = "daily"
	Weekly  Frequency = "weekly"
	Monthly Frequency = "monthly"
	Yearly  Frequency = "yearly"
)

// MaxOccurrencesPerWindow bounds a single enumeration. A caller asking for a
// wider window must split it into bounded windows, as the generator does for
// downtime catch-up. User-controlled input must not cause unbounded allocation.
const MaxOccurrencesPerWindow = 4000

var (
	ErrFrequencyInvalid  = errors.New("recur: frequency is invalid")
	ErrIntervalInvalid   = errors.New("recur: interval must be at least 1")
	ErrWeekdayMissing    = errors.New("recur: weekly schedule needs a weekday")
	ErrDayOfMonthInvalid = errors.New("recur: monthly or yearly schedule needs a day of month or the last-day flag, not both")
	ErrMonthInvalid      = errors.New("recur: yearly schedule needs a month between 1 and 12")
	ErrDateInvalid       = errors.New("recur: date is not an ISO YYYY-MM-DD calendar date")
	ErrWindowTooWide     = errors.New("recur: window would produce more occurrences than the cap")
)

// ScheduleSpec is a template's recurrence rule. It is a deliberate subset of
// iCalendar's RRULE: the fields below cover every schedule the daily-driver
// persona actually keeps (rent, salary, subscriptions, standing transfers).
// Extending the subset is the supported way to grow; adopting RRULE as a wire
// format is not (docs/plans/recurring-transactions-plan.md, out of scope).
type ScheduleSpec struct {
	Frequency     Frequency
	IntervalCount int

	// ByWeekday selects the weekday for Weekly schedules. Ignored otherwise.
	ByWeekday *time.Weekday

	// DayOfMonth is the nominal day for Monthly and Yearly schedules, 1-31.
	// A nominal day past the end of a short month clamps to that month's last
	// day; the series re-anchors on the nominal day the following month, so
	// "the 31st" is 31 Jan, 28 Feb, 31 Mar and never drifts to the 28th.
	DayOfMonth int

	// LastDayOfMonth means "the last day, whatever it is". Mutually exclusive
	// with DayOfMonth.
	LastDayOfMonth bool

	// MonthOfYear selects the month for Yearly schedules, 1-12.
	MonthOfYear int

	// StartsOn is the phase anchor. It is not necessarily an occurrence: a
	// weekly schedule anchored on a Tuesday with ByWeekday=Friday first occurs
	// on the following Friday.
	StartsOn string

	// EndsOn is the last date the series may produce. Empty means open-ended.
	EndsOn string

	// MaxOccurrences caps the series counted from StartsOn — the anchor, not
	// the requested window. Zero means unlimited.
	MaxOccurrences int
}

// Validate reports whether the spec is usable. The service calls it on save so
// an unusable template can never reach the generator.
func (spec ScheduleSpec) Validate() error {
	switch spec.Frequency {
	case Daily, Weekly, Monthly, Yearly:
	default:
		return fmt.Errorf("%w: %q", ErrFrequencyInvalid, spec.Frequency)
	}
	if spec.IntervalCount < 1 {
		return fmt.Errorf("%w: %d", ErrIntervalInvalid, spec.IntervalCount)
	}
	if _, err := parseDate(spec.StartsOn); err != nil {
		return fmt.Errorf("starts_on: %w", err)
	}
	if spec.EndsOn != "" {
		endsOn, err := parseDate(spec.EndsOn)
		if err != nil {
			return fmt.Errorf("ends_on: %w", err)
		}
		startsOn, _ := parseDate(spec.StartsOn)
		if endsOn.Before(startsOn) {
			return fmt.Errorf("%w: ends_on precedes starts_on", ErrDateInvalid)
		}
	}
	if spec.MaxOccurrences < 0 {
		return fmt.Errorf("%w: max occurrences is negative", ErrIntervalInvalid)
	}

	switch spec.Frequency {
	case Weekly:
		if spec.ByWeekday == nil {
			return ErrWeekdayMissing
		}
		if *spec.ByWeekday < time.Sunday || *spec.ByWeekday > time.Saturday {
			return fmt.Errorf("%w: %d", ErrWeekdayMissing, *spec.ByWeekday)
		}
	case Monthly:
		if err := validateMonthDay(spec); err != nil {
			return err
		}
	case Yearly:
		if err := validateMonthDay(spec); err != nil {
			return err
		}
		if spec.MonthOfYear < 1 || spec.MonthOfYear > 12 {
			return fmt.Errorf("%w: %d", ErrMonthInvalid, spec.MonthOfYear)
		}
	}
	return nil
}

func validateMonthDay(spec ScheduleSpec) error {
	if spec.LastDayOfMonth {
		if spec.DayOfMonth != 0 {
			return ErrDayOfMonthInvalid
		}
		return nil
	}
	if spec.DayOfMonth < 1 || spec.DayOfMonth > 31 {
		return fmt.Errorf("%w: %d", ErrDayOfMonthInvalid, spec.DayOfMonth)
	}
	return nil
}

// Occurrences returns every date the schedule produces within the inclusive
// window [from, to], in ascending order.
func Occurrences(spec ScheduleSpec, from string, to string) ([]string, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	windowStart, err := parseDate(from)
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	windowEnd, err := parseDate(to)
	if err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}
	if windowEnd.Before(windowStart) {
		return []string{}, nil
	}

	dates := []string{}
	err = walk(spec, func(index int, date time.Time) (bool, error) {
		if date.After(windowEnd) {
			return false, nil
		}
		if !date.Before(windowStart) {
			if len(dates) >= MaxOccurrencesPerWindow {
				return false, fmt.Errorf("%w: %d", ErrWindowTooWide, MaxOccurrencesPerWindow)
			}
			dates = append(dates, date.Format(time.DateOnly))
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return dates, nil
}

// Next returns the first occurrence strictly after the given date, reporting
// false when the series ends first. The generator uses it for a template's
// "next due" column, where enumerating a whole window would be wasteful.
func Next(spec ScheduleSpec, after string) (string, bool, error) {
	if err := spec.Validate(); err != nil {
		return "", false, err
	}
	cutoff, err := parseDate(after)
	if err != nil {
		return "", false, fmt.Errorf("after: %w", err)
	}

	next := ""
	err = walk(spec, func(index int, date time.Time) (bool, error) {
		if !date.After(cutoff) {
			return true, nil
		}
		next = date.Format(time.DateOnly)
		return false, nil
	})
	if err != nil {
		return "", false, err
	}
	return next, next != "", nil
}

// walk generates the series from its origin and hands each date to visit,
// stopping when visit returns false or the series' own end condition is
// reached.
//
// It always starts at the origin rather than at the caller's window, because
// MaxOccurrences counts the series from its start and because a monthly series
// must re-anchor on the nominal day: computing the next date from the previous
// *produced* date is what makes "the 31st" collapse to the 28th after one
// February.
func walk(spec ScheduleSpec, visit func(index int, date time.Time) (bool, error)) error {
	anchor, err := parseDate(spec.StartsOn)
	if err != nil {
		return fmt.Errorf("starts_on: %w", err)
	}
	var seriesEnd time.Time
	hasSeriesEnd := spec.EndsOn != ""
	if hasSeriesEnd {
		seriesEnd, err = parseDate(spec.EndsOn)
		if err != nil {
			return fmt.Errorf("ends_on: %w", err)
		}
	}

	series := newSeries(spec, anchor)
	for index := 0; ; index++ {
		if spec.MaxOccurrences > 0 && index >= spec.MaxOccurrences {
			return nil
		}
		date := series.at(index)
		// API and ledger dates have exactly four year digits. Exhausting that
		// calendar is an ended series, not a malformed next_due_on value.
		if date.Year() > 9999 || date.Year() < 1 {
			return nil
		}
		if hasSeriesEnd && date.After(seriesEnd) {
			return nil
		}
		keepGoing, err := visit(index, date)
		if err != nil {
			return err
		}
		if !keepGoing {
			return nil
		}
	}
}

// series is a spec resolved against its anchor: origin is the series' own
// starting point, computed once, and at(index) is a pure offset from it.
//
// The origin is resolved once rather than per index on purpose. An earlier
// revision special-cased index 0 when the anchor's month had already passed
// its nominal day, which shifted only the first occurrence and left index 1
// landing on the same date — a duplicate the unique occurrence constraint
// would have caught in production instead of here.
type series struct {
	spec ScheduleSpec

	// origin is the first occurrence for daily and weekly schedules, and the
	// first of the first occurrence's month for monthly and yearly ones, where
	// the day still has to be clamped per month.
	origin time.Time
}

func newSeries(spec ScheduleSpec, anchor time.Time) series {
	switch spec.Frequency {
	case Daily:
		return series{spec: spec, origin: anchor}

	case Weekly:
		// The first occurrence is the first ByWeekday on or after the anchor.
		offset := (int(*spec.ByWeekday) - int(anchor.Weekday()) + 7) % 7
		return series{spec: spec, origin: anchor.AddDate(0, 0, offset)}

	case Monthly:
		// Month arithmetic runs on the first of the month so AddDate cannot
		// normalize 31 February into 3 March before the clamp is applied.
		origin := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
		if clampToMonth(origin, spec).Before(anchor) {
			// The anchor's own month has already passed its nominal day, so
			// the series starts one whole interval later. Shifting the origin
			// keeps the phase — every later index is still one interval apart.
			origin = origin.AddDate(0, spec.IntervalCount, 0)
		}
		return series{spec: spec, origin: origin}

	case Yearly:
		origin := time.Date(anchor.Year(), time.Month(spec.MonthOfYear), 1, 0, 0, 0, 0, time.UTC)
		if clampToMonth(origin, spec).Before(anchor) {
			origin = origin.AddDate(spec.IntervalCount, 0, 0)
		}
		return series{spec: spec, origin: origin}
	}
	// Unreachable: Validate rejects every other frequency before walk runs.
	return series{spec: spec, origin: anchor}
}

func (s series) at(index int) time.Time {
	step := index * s.spec.IntervalCount
	switch s.spec.Frequency {
	case Daily:
		return s.origin.AddDate(0, 0, step)
	case Weekly:
		return s.origin.AddDate(0, 0, step*7)
	case Monthly:
		return clampToMonth(s.origin.AddDate(0, step, 0), s.spec)
	case Yearly:
		return clampToMonth(s.origin.AddDate(step, 0, 0), s.spec)
	}
	return s.origin
}

// clampToMonth resolves the spec's day rule inside the given month.
func clampToMonth(firstOfMonth time.Time, spec ScheduleSpec) time.Time {
	lastDay := daysInMonth(firstOfMonth.Year(), firstOfMonth.Month())
	day := spec.DayOfMonth
	if spec.LastDayOfMonth || day > lastDay {
		day = lastDay
	}
	return time.Date(firstOfMonth.Year(), firstOfMonth.Month(), day, 0, 0, 0, 0, time.UTC)
}

func daysInMonth(year int, month time.Month) int {
	// The zeroth day of the next month is the last day of this one.
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrDateInvalid, value)
	}
	// Require the canonical ledger calendar, including the year boundary.
	if parsed.Year() < 1 || parsed.Year() > 9999 || parsed.Format(time.DateOnly) != value {
		return time.Time{}, fmt.Errorf("%w: %q", ErrDateInvalid, value)
	}
	return parsed, nil
}
