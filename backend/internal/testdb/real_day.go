package testdb

import "time"

// RealUTCToday reads the real UTC day, at midnight. A reference read the
// service forgot to pin falls back to SQLite's date('now'), which follows the
// real clock, not a test's frozen Go clock; a hard-coded instant stops
// exposing that fallback once the real date passes it (#601 review). A test
// reads this once and derives its frozen instant and every expected date from
// that one reading (UTCDayInstant, UTCDay): two clock reads put them on
// different days when UTC midnight falls between them.
func RealUTCToday() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// UTCDayInstant is clock (HH:MM:SS) on the UTC day offset days from today,
// in RFC 3339.
func UTCDayInstant(today time.Time, days int, clock string) string {
	return UTCDay(today, days) + "T" + clock + "Z"
}

// UTCDay is the UTC day offset days from today, as a calendar date.
func UTCDay(today time.Time, days int) string {
	return today.AddDate(0, 0, days).Format(time.DateOnly)
}
