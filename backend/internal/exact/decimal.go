package exact

import (
	"math/big"
	"strings"
)

// Decimal renders a coefficient and its scale as a plain decimal string:
// point separator, no group separators, sign as a leading '-'. It is the one
// place the backend turns exact storage into text, so an export, a log line,
// and a future file writer cannot drift from each other.
//
// It has a cross-language twin: formatLedgerAmount in
// frontend/src/lib/money/amount.ts applies the same rule in TypeScript, and
// neither side can import the other. Keep the two answering identically — the
// shared test-vector fixture that pins them arrives with the export bundle
// (docs/plans/data-portability-plan.md, slice 2), because that is when the
// second Go caller does.
//
// The rendering keeps the stored scale rather than trimming it: "1200" at
// scale 2 is "12.00", because the scale is part of what was recorded. A
// negative zero cannot appear — Coefficient is canonical, and canonical zero
// is unsigned.
func Decimal(value Coefficient, scale int) string {
	return decimalDigits(value.String(), scale)
}

// DecimalFromBig renders an accumulated value that has not been through the
// Coefficient gate.
//
// A sum can be legitimately wider than the 38 digits a stored coefficient may
// carry: 10^38 + 2.50 needs 40, and the ledger holds both operands quite
// legally. That ceiling exists for values the app stores and returns as
// coefficient strings; an exported decimal is text, and refusing to render it
// would fail a whole archive over a figure that is exact and perfectly
// readable. Values headed back into storage or JSON still go through
// ScaledInt.Coefficient, which refuses instead of wrapping.
func DecimalFromBig(value *big.Int, scale int) string {
	if value == nil {
		return decimalDigits("0", scale)
	}
	return decimalDigits(value.String(), scale)
}

func decimalDigits(digits string, scale int) string {
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
