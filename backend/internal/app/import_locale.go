package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// This file holds the locale-sensitive parsing shared by the file-import
// adapters: how a date string maps to Y/M/D, and which character in a number
// is the decimal separator. Both are ambiguous in European exports
// ("01/02/2026" is 1 February, "1,50" is one and a half), so the adapters
// resolve them once per file — from an import profile when the user has set
// one, otherwise by detecting the layout across the whole file.

// dateOrder is the field order of a numeric date such as "01/02/06".
type dateOrder int

const (
	// dateOrderAuto defers to per-file detection; it is never used to parse
	// a single date on its own (parseFlexibleDate falls back to MDY).
	dateOrderAuto dateOrder = iota
	dateOrderMDY
	dateOrderDMY
	dateOrderYMD
)

func (o dateOrder) String() string {
	switch o {
	case dateOrderMDY:
		return "MDY"
	case dateOrderDMY:
		return "DMY"
	case dateOrderYMD:
		return "YMD"
	default:
		return "auto"
	}
}

// parseDateOrder reads a profile's date_layout setting. Both the short tokens
// ("DMY") and the human patterns users expect to type ("DD/MM/YYYY") are
// accepted.
func parseDateOrder(raw string) (dateOrder, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	if normalized == "" || normalized == "AUTO" {
		return dateOrderAuto, nil
	}
	// Reduce a pattern like "DD/MM/YYYY" or "D.M.YY" to its field order.
	var order strings.Builder
	var last byte
	for i := 0; i < len(normalized); i++ {
		c := normalized[i]
		if c != 'D' && c != 'M' && c != 'Y' {
			last = 0
			continue
		}
		if c != last {
			order.WriteByte(c)
		}
		last = c
	}
	switch order.String() {
	case "MDY":
		return dateOrderMDY, nil
	case "DMY":
		return dateOrderDMY, nil
	case "YMD":
		return dateOrderYMD, nil
	default:
		return dateOrderAuto, fmt.Errorf("unrecognized date layout %q (use DMY, MDY, YMD or a pattern like DD/MM/YYYY)", raw)
	}
}

var monthAbbreviations = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

// splitDateParts splits a date on any separator QIF exporters use:
// "01/15/06", "15-01-2006", "15.01.2006", "1/ 5'06" (Quicken).
func splitDateParts(raw string) ([]string, bool) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || r == '\'' || r == ' ' || r == '\t'
	})
	if len(fields) != 3 {
		return nil, false
	}
	return fields, true
}

// dateAmbiguity reports what a single date string proves about the file's
// field order: an EU-only date has a first field > 12, a US-only date has a
// second field > 12, and everything else is ambiguous.
func dateAmbiguity(raw string) (order dateOrder, decisive bool) {
	parts, ok := splitDateParts(strings.TrimSpace(raw))
	if !ok {
		return dateOrderAuto, false
	}
	if len(parts[0]) == 4 {
		return dateOrderYMD, true
	}
	first, err1 := strconv.Atoi(parts[0])
	second, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		// A month name (e.g. "15-Jan-06") is unambiguous by construction and
		// tells us nothing about the numeric layout.
		return dateOrderAuto, false
	}
	switch {
	case first > 12 && second <= 12:
		return dateOrderDMY, true
	case second > 12 && first <= 12:
		return dateOrderMDY, true
	default:
		return dateOrderAuto, false
	}
}

// isOrderAmbiguousDate reports whether a date reads as a real date under both
// field orders — "01/02/2026" does, "15/02/2026" and "not-a-date" do not.
func isOrderAmbiguousDate(raw string) bool {
	mdy, errMDY := parseFlexibleDate(raw, dateOrderMDY)
	dmy, errDMY := parseFlexibleDate(raw, dateOrderDMY)
	return errMDY == nil && errDMY == nil && mdy != dmy
}

// dateOrderDetection is the outcome of scanning every date in one file.
type dateOrderDetection struct {
	// Order is the layout to parse the file with — the detected one, or the
	// MDY default when nothing in the file settles it.
	Order dateOrder
	// Decisive is true when at least one row proved the layout.
	Decisive bool
	// Ambiguous counts rows that could be read either way. Combined with
	// !Decisive it means the file is genuinely undecidable and the caller
	// should warn the user to set a profile date layout.
	Ambiguous int
	// Conflict is true when different rows imply different layouts, which
	// means the file is malformed or mixes exports.
	Conflict bool
}

// detectDateOrder resolves a file's date layout from all of its dates. One row
// with a day > 12 settles the whole file; QIF's historic US default (MDY) is
// the fallback when no row is decisive.
func detectDateOrder(dates []string) dateOrderDetection {
	result := dateOrderDetection{Order: dateOrderMDY}
	votes := map[dateOrder]int{}
	for _, raw := range dates {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		order, decisive := dateAmbiguity(raw)
		if !decisive {
			if isOrderAmbiguousDate(raw) {
				result.Ambiguous++
			}
			continue
		}
		votes[order]++
	}
	// ISO dates are self-describing and never force the numeric layout.
	delete(votes, dateOrderYMD)

	best := dateOrderAuto
	for order, count := range votes {
		if best == dateOrderAuto || count > votes[best] {
			best = order
		}
	}
	if best == dateOrderAuto {
		return result
	}
	result.Order = best
	result.Decisive = true
	result.Conflict = len(votes) > 1
	return result
}

// parseFlexibleDate normalizes one date to YYYY-MM-DD using the given field
// order. dateOrderAuto resolves per-date (day > 12 wins) and otherwise falls
// back to MDY.
func parseFlexibleDate(raw string, order dateOrder) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("date is empty")
	}

	// Already ISO: validate rather than re-derive.
	if len(raw) == 10 && raw[4] == '-' && raw[7] == '-' {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			return t.Format("2006-01-02"), nil
		}
		return "", fmt.Errorf("unrecognized date format")
	}

	parts, ok := splitDateParts(raw)
	if !ok {
		return "", fmt.Errorf("unrecognized date format")
	}

	var dayStr, monthStr, yearStr string
	switch {
	case monthFromName(parts[0]) > 0:
		monthStr, dayStr, yearStr = parts[0], parts[1], parts[2]
	case monthFromName(parts[1]) > 0:
		dayStr, monthStr, yearStr = parts[0], parts[1], parts[2]
	case len(parts[0]) == 4 || order == dateOrderYMD:
		yearStr, monthStr, dayStr = parts[0], parts[1], parts[2]
	default:
		effective := order
		if effective == dateOrderAuto {
			detected, decisive := dateAmbiguity(raw)
			if decisive && detected != dateOrderYMD {
				effective = detected
			} else {
				effective = dateOrderMDY
			}
		}
		if effective == dateOrderDMY {
			dayStr, monthStr, yearStr = parts[0], parts[1], parts[2]
		} else {
			monthStr, dayStr, yearStr = parts[0], parts[1], parts[2]
		}
		// A row that contradicts the chosen order — "15/02" under MDY — has
		// only one valid reading, so take it rather than rejecting the row.
		if first, err := strconv.Atoi(monthStr); err == nil && first > 12 {
			if second, err := strconv.Atoi(dayStr); err == nil && second <= 12 {
				dayStr, monthStr = monthStr, dayStr
			}
		}
	}

	month := monthFromName(monthStr)
	if month == 0 {
		parsed, err := strconv.Atoi(monthStr)
		if err != nil {
			return "", fmt.Errorf("unrecognized date format")
		}
		month = parsed
	}
	day, err := strconv.Atoi(dayStr)
	if err != nil {
		return "", fmt.Errorf("unrecognized date format")
	}
	year, err := parseYear(yearStr)
	if err != nil {
		return "", err
	}

	if month < 1 || month > 12 || day < 1 || day > 31 {
		return "", fmt.Errorf("unrecognized date format")
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		// Rolled over (e.g. 31 February): not a real date.
		return "", fmt.Errorf("unrecognized date format")
	}
	return t.Format("2006-01-02"), nil
}

func monthFromName(raw string) int {
	if len(raw) < 3 {
		return 0
	}
	return monthAbbreviations[strings.ToLower(raw[:3])]
}

// parseYear expands two-digit years on the same 69/70 pivot Go's "06" layout
// uses, so QIF files written for Quicken keep their historic interpretation.
func parseYear(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	year, err := strconv.Atoi(raw)
	if err != nil || year < 0 {
		return 0, fmt.Errorf("unrecognized date format")
	}
	if len(raw) <= 2 {
		if year >= 69 {
			return 1900 + year, nil
		}
		return 2000 + year, nil
	}
	return year, nil
}

// canonicalDecimal rewrites a locale-formatted number ("1.234,56", "1 234,56",
// "1,234.56") into the canonical form the ledger parses ("-1234.56").
//
// decSep is the decimal separator from the import profile; pass 0 to detect it
// per value. Detection rules:
//   - both '.' and ',' present → the last one is the decimal separator
//   - repeated separators of one kind → those are thousands separators
//   - a single ',' → decimal separator unless it is followed by exactly three
//     digits, the shape of a US thousands group ("1,234")
//   - a single '.' → decimal separator (period-grouped thousands without any
//     comma, "1.234" meaning 1234, is indistinguishable from three decimals
//     and loses to the far more common reading)
//
// Set decimal_separator on the import profile when a file's own shape cannot
// settle it.
func canonicalDecimal(raw string, decSep rune) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("amount is empty")
	}

	// Drop currency symbols and the space/apostrophe thousands separators used
	// by EU and Swiss exports (including NBSP and narrow NBSP).
	value = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', ' ', ' ', '\'', '’':
			return -1
		}
		return r
	}, value)

	negative := false
	switch {
	case strings.HasPrefix(value, "-"):
		negative = true
		value = value[1:]
	case strings.HasPrefix(value, "+"):
		value = value[1:]
	}
	// Accounting-style negatives: "(1.234,56)".
	if strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		negative = !negative
		value = value[1 : len(value)-1]
	}
	if value == "" {
		return "", fmt.Errorf("amount is empty")
	}

	if decSep == 0 {
		decSep = detectDecimalSeparator(value)
	}

	var intPart, fracPart string
	if idx := strings.LastIndexByte(value, byte(decSep)); decSep != 0 && idx >= 0 {
		intPart, fracPart = value[:idx], value[idx+1:]
	} else {
		intPart = value
	}
	// Everything left in the integer part must be well-formed grouping.
	intPart, err := stripGrouping(intPart)
	if err != nil {
		return "", fmt.Errorf("amount %q is not a number", raw)
	}

	if !isDigits(intPart) || (fracPart != "" && !isDigits(fracPart)) {
		return "", fmt.Errorf("amount %q is not a number", raw)
	}
	if intPart == "" && fracPart == "" {
		return "", fmt.Errorf("amount %q is not a number", raw)
	}
	if intPart == "" {
		intPart = "0"
	}

	canonical := intPart
	if fracPart != "" {
		canonical += "." + fracPart
	}
	if negative {
		canonical = "-" + canonical
	}
	return canonical, nil
}

// stripGrouping removes the thousands separators from a number's integer part,
// rejecting anything that is not real grouping: mixed separators, or groups
// that are not three digits ("1.2.3").
func stripGrouping(intPart string) (string, error) {
	if !strings.ContainsAny(intPart, ",.") {
		return intPart, nil
	}
	if strings.Contains(intPart, ",") && strings.Contains(intPart, ".") {
		return "", fmt.Errorf("mixed thousands separators")
	}
	sep := ","
	if strings.Contains(intPart, ".") {
		sep = "."
	}
	groups := strings.Split(intPart, sep)
	for i, group := range groups {
		if !isDigits(group) || group == "" {
			return "", fmt.Errorf("malformed thousands group")
		}
		if i == 0 && len(group) > 3 {
			return "", fmt.Errorf("malformed leading group")
		}
		if i > 0 && len(group) != 3 {
			return "", fmt.Errorf("malformed thousands group")
		}
	}
	return strings.Join(groups, ""), nil
}

// detectDecimalSeparator picks the decimal separator of a single unsigned
// number, returning 0 when the value has no fractional part.
func detectDecimalSeparator(value string) rune {
	commas := strings.Count(value, ",")
	periods := strings.Count(value, ".")

	switch {
	case commas > 0 && periods > 0:
		if strings.LastIndexByte(value, ',') > strings.LastIndexByte(value, '.') {
			return ','
		}
		return '.'
	case commas > 1:
		return 0 // "1,234,567" — all grouping
	case commas == 1:
		if len(value)-strings.IndexByte(value, ',')-1 == 3 {
			return 0 // "1,234" — a US thousands group
		}
		return ','
	case periods > 1:
		return 0 // "1.234.567" — all grouping
	case periods == 1:
		return '.'
	default:
		return 0
	}
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
