package exact

import "strings"

// Decimal renders a coefficient and its scale as a plain decimal string:
// point separator, no group separators, sign as a leading '-'. It is the one
// place the backend turns exact storage into text, so an export, a log line,
// and a future file writer cannot drift from each other.
//
// The rendering keeps the stored scale rather than trimming it: "1200" at
// scale 2 is "12.00", because the scale is part of what was recorded. A
// negative zero cannot appear — Coefficient is canonical, and canonical zero
// is unsigned.
func Decimal(value Coefficient, scale int) string {
	digits := value.String()
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = digits[1:]
	}

	var rendered string
	switch {
	case scale <= 0:
		rendered = digits
	default:
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		rendered = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}

	if negative {
		return "-" + rendered
	}
	return rendered
}
