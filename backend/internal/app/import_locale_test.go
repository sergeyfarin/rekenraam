package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Date field order (T-35) ---

func TestParseDateOrder(t *testing.T) {
	tests := []struct {
		input string
		want  dateOrder
	}{
		{"", dateOrderAuto},
		{"auto", dateOrderAuto},
		{"DMY", dateOrderDMY},
		{"mdy", dateOrderMDY},
		{"YMD", dateOrderYMD},
		{"DD/MM/YYYY", dateOrderDMY},
		{"MM/DD/YY", dateOrderMDY},
		{"D.M.YYYY", dateOrderDMY},
		{"YYYY-MM-DD", dateOrderYMD},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseDateOrder(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseDateOrder_UnrecognizedReturnsError(t *testing.T) {
	_, err := parseDateOrder("DD/MM/DD")
	require.Error(t, err)
}

func TestDetectDateOrder(t *testing.T) {
	t.Run("one day above 12 settles the whole file as EU", func(t *testing.T) {
		got := detectDateOrder([]string{"01/02/2026", "15/02/2026", "03/04/2026"})
		assert.Equal(t, dateOrderDMY, got.Order)
		assert.True(t, got.Decisive)
		assert.False(t, got.Conflict)
	})

	t.Run("one month-second date settles the whole file as US", func(t *testing.T) {
		got := detectDateOrder([]string{"01/02/2026", "02/15/2026"})
		assert.Equal(t, dateOrderMDY, got.Order)
		assert.True(t, got.Decisive)
	})

	t.Run("all-ambiguous files fall back to US and report it", func(t *testing.T) {
		got := detectDateOrder([]string{"01/02/2026", "03/04/2026"})
		assert.Equal(t, dateOrderMDY, got.Order)
		assert.False(t, got.Decisive)
		assert.Equal(t, 2, got.Ambiguous)
	})

	t.Run("contradicting rows are flagged", func(t *testing.T) {
		got := detectDateOrder([]string{"15/02/2026", "02/15/2026"})
		assert.True(t, got.Conflict)
	})

	t.Run("ISO dates do not vote", func(t *testing.T) {
		got := detectDateOrder([]string{"2026-02-15", "2026-03-01"})
		assert.Equal(t, dateOrderMDY, got.Order)
		assert.False(t, got.Decisive)
		assert.Zero(t, got.Ambiguous, "ISO dates are not ambiguous, so they must not trigger the warning")
	})
}

func TestParseFlexibleDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		order dateOrder
		want  string
	}{
		{"EU order", "01/02/2026", dateOrderDMY, "2026-02-01"},
		{"US order", "01/02/2026", dateOrderMDY, "2026-01-02"},
		{"EU two-digit year", "01/02/06", dateOrderDMY, "2006-02-01"},
		{"EU dotted", "01.02.2026", dateOrderDMY, "2026-02-01"},
		{"unambiguous day beats the order hint", "15/02/2026", dateOrderMDY, "2026-02-15"},
		{"ISO passthrough", "2026-02-15", dateOrderDMY, "2026-02-15"},
		{"four-digit first field is a year", "2026/02/15", dateOrderDMY, "2026-02-15"},
		{"month name", "15-Jan-06", dateOrderMDY, "2006-01-15"},
		{"month name first", "Jan 15 2006", dateOrderDMY, "2006-01-15"},
		{"Quicken apostrophe year", "1/ 5'06", dateOrderMDY, "2006-01-05"},
		{"auto resolves from the date itself", "15/02/2026", dateOrderAuto, "2026-02-15"},
		{"auto falls back to US", "01/02/2026", dateOrderAuto, "2026-01-02"},
		{"two-digit year pivot", "01/02/70", dateOrderMDY, "1970-01-02"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFlexibleDate(tc.input, tc.order)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseFlexibleDate_RejectsImpossibleDates(t *testing.T) {
	for _, input := range []string{"", "not-a-date", "31/02/2026", "13/13/2026", "2026-02-31", "1/2"} {
		t.Run(input, func(t *testing.T) {
			_, err := parseFlexibleDate(input, dateOrderAuto)
			require.Error(t, err)
		})
	}
}

// --- Decimal separator (T-36) ---

func TestCanonicalDecimal_Detected(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1,50", "1.50"},          // T-36: a decimal comma, not 150
		{"-1.234,56", "-1234.56"}, // EU grouping + decimal comma
		{"1,234.56", "1234.56"},   // US grouping + decimal point
		{"1,234,567.89", "1234567.89"},
		{"1.234.567,89", "1234567.89"},
		{"1,234", "1234"},  // a lone three-digit group reads as thousands
		{"1.234", "1.234"}, // a lone period stays a decimal point
		{"-42.50", "-42.50"},
		{"+100,5", "100.5"},
		{"1 234,56", "1234.56"},    // space-grouped (EU)
		{"1 234,56", "1234.56"},    // NBSP-grouped
		{"1'234.56", "1234.56"},    // apostrophe-grouped (CH)
		{"(1.234,56)", "-1234.56"}, // accounting negative
		{",50", "0.50"},
		{"0,00", "0.00"},
		{"500", "500"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := canonicalDecimal(tc.input, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Per value, "1.234" is genuinely undecidable and reads as a decimal point.
// Across a whole file it usually is decidable, and reading a German export's
// "1.234" as 1.234 instead of 1234 is the same silent corruption T-36 was
// about — the announcement's target audience, on the announcement's demo path.
func TestDetectDecimalSeparatorAcrossValues(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		want     rune
		decisive bool
		conflict bool
	}{
		{
			name:     "one decimal comma settles the file",
			values:   []string{"1.234", "56,78", "9.000"},
			want:     ',',
			decisive: true,
		},
		{
			name:     "one decimal point settles the file",
			values:   []string{"1,234", "56.78", "9,000"},
			want:     '.',
			decisive: true,
		},
		{
			name:     "both separators in one value settle it",
			values:   []string{"1.234,56", "9.000"},
			want:     ',',
			decisive: true,
		},
		{
			name:     "repeated commas are grouping, so the decimal is a point",
			values:   []string{"1,234,567", "890"},
			want:     '.',
			decisive: true,
		},
		{
			name:   "nothing decisive leaves it to the per-value reading",
			values: []string{"1.234", "9.000"},
			want:   0,
		},
		{
			// Picking a winner here would depend on map iteration order, so the
			// same file could parse two ways on two runs. Undecided is the only
			// honest and repeatable answer.
			name:     "an even disagreement stays undecided rather than coin-flipping",
			values:   []string{"1.234,56", "7,890.12"},
			want:     0,
			conflict: true,
		},
		{
			name:     "a clear majority wins and is still flagged",
			values:   []string{"1,50", "2,75", "3,90", "7,890.12"},
			want:     ',',
			decisive: true,
			conflict: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectDecimalSeparatorAcrossValues(test.values)
			assert.Equal(t, string(test.want), string(got.Separator))
			assert.Equal(t, test.decisive, got.Decisive)
			assert.Equal(t, test.conflict, got.Conflict)
		})
	}
}

func TestCanonicalDecimal_ExplicitSeparatorOverridesDetection(t *testing.T) {
	// "1,234" is read as a thousands group by default; a profile that declares
	// a decimal comma must win.
	got, err := canonicalDecimal("1,234", ',')
	require.NoError(t, err)
	assert.Equal(t, "1.234", got)

	got, err = canonicalDecimal("1.234", '.')
	require.NoError(t, err)
	assert.Equal(t, "1.234", got)

	got, err = canonicalDecimal("1.234", ',')
	require.NoError(t, err)
	assert.Equal(t, "1234", got, "with a declared decimal comma, a period can only be grouping")
}

func TestCanonicalDecimal_RejectsNonNumbers(t *testing.T) {
	for _, input := range []string{"", "  ", "abc", "1.2.3,4,5", "12x.34", "-"} {
		t.Run(input, func(t *testing.T) {
			_, err := canonicalDecimal(input, 0)
			require.Error(t, err)
		})
	}
}
