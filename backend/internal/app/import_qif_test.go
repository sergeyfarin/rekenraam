package app

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQIF_SimpleBankTransaction(t *testing.T) {
	qif := `!Type:Bank
D01/15/06
T-42.50
PGrocery Store
MWeekly groceries
^
D01/20/06
T1500.00
PSalary
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)

	r0 := result.Rows[0]
	assert.Equal(t, "01/15/06", r0.Date)
	assert.Equal(t, "-42.50", r0.Amount)
	assert.Equal(t, "Grocery Store", r0.PayeeHint)
	assert.Equal(t, "Weekly groceries", r0.Memo)
	assert.Empty(t, r0.CategoryHint)
	assert.Empty(t, r0.TransferHint)

	r1 := result.Rows[1]
	assert.Equal(t, "1500.00", r1.Amount)
	assert.Equal(t, "Salary", r1.PayeeHint)
}

func TestParseQIF_TransferDetection(t *testing.T) {
	qif := `!Type:Bank
D02/01/06
T-500.00
PSavings Transfer
L[Savings Account]
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)

	r := result.Rows[0]
	assert.Equal(t, "Savings Account", r.TransferHint)
	assert.Empty(t, r.CategoryHint)
}

func TestParseQIF_Splits(t *testing.T) {
	qif := `!Type:Bank
D03/10/06
T-100.00
PTarget
SFood:Groceries
$-60.00
SHousehold
$-40.00
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)

	r := result.Rows[0]
	require.Len(t, r.Splits, 2)
	assert.Equal(t, "Food:Groceries", r.Splits[0].CategoryHint)
	assert.Equal(t, "-60.00", r.Splits[0].Amount)
	assert.Equal(t, "Household", r.Splits[1].CategoryHint)
	assert.Equal(t, "-40.00", r.Splits[1].Amount)
}

func TestParseQIF_InvestmentType_WarnsNeedsAttention(t *testing.T) {
	qif := `!Type:Invst
D04/01/06
T1000.00
YApple Inc
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "invest.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "investment")
}

func TestParseQIF_ThousandsCommaStripped(t *testing.T) {
	qif := `!Type:Bank
D05/01/06
T1,234,567.89
PCashout
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "1234567.89", result.Rows[0].Amount)
}

func TestParseQIF_FingerprintsAreUnique(t *testing.T) {
	// Two identical rows should have different fingerprints (via occurrence index).
	qif := `!Type:Bank
D06/01/06
T-5.00
PCoffee
^
D06/01/06
T-5.00
PCoffee
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)
	assert.NotEqual(t, result.Rows[0].DedupeFingerprint, result.Rows[1].DedupeFingerprint)
}

func TestParseQIF_DateRangeInMeta(t *testing.T) {
	qif := `!Type:Bank
D01/01/06
T-10.00
PA
^
D06/15/06
T-20.00
PB
^
D03/10/06
T-30.00
PC
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	assert.Equal(t, "01/01/06", result.Meta.DateFrom)
	assert.Equal(t, "06/15/06", result.Meta.DateTo)
}

func TestParseQIF_NoTrailingCaret(t *testing.T) {
	// File without trailing ^ still produces a row.
	qif := `!Type:Bank
D07/01/06
T-99.00
PStore
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "-99.00", result.Rows[0].Amount)
}

// --- QIF adapter Detect tests ---

func TestQIFAdapterDetect(t *testing.T) {
	adapter := &QIFAdapter{}

	assert.Equal(t, ConfidenceHigh, adapter.Detect(RawInput{Filename: "account.QIF"}))
	assert.Equal(t, ConfidenceHigh, adapter.Detect(RawInput{Filename: "export.qif"}))
	assert.Equal(t, ConfidenceMedium, adapter.Detect(RawInput{
		Filename: "file.txt",
		Bytes:    []byte("!Type:Bank\nD01/01/06\nT-10.00\n^\n"),
	}))
	assert.Equal(t, ConfidenceNone, adapter.Detect(RawInput{Filename: "data.csv", Bytes: []byte("date,amount\n")}))
}

// --- Date parsing tests ---

func TestParseQIFDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"01/15/06", "2006-01-15"},
		{"01/15/2006", "2006-01-15"},
		{"1/5/06", "2006-01-05"},
		{"2006-01-15", "2006-01-15"},
		{"15-Jan-06", "2006-01-15"},
		{"15-Jan-2006", "2006-01-15"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseQIFDate(tc.input)
			require.NoError(t, err, "input: %q", tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseQIFDate_InvalidReturnsError(t *testing.T) {
	_, err := parseQIFDate("not-a-date")
	require.Error(t, err)
}

// --- Decimal amount parsing tests ---

func TestParseDecimalAmount(t *testing.T) {
	tests := []struct {
		input     string
		wantCoeff string
		wantScale int
	}{
		{"-42.50", "-4250", 2},
		{"1500.00", "150000", 2},
		{"1,234,567.89", "123456789", 2},
		{"-500", "-500", 0},
		{"0.00", "0", 2},
		{"+100.5", "1005", 1},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			coeff, scale, err := parseDecimalAmount(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCoeff, string(coeff))
			assert.Equal(t, tc.wantScale, scale)
		})
	}
}

func TestParseDecimalAmount_EmptyReturnsError(t *testing.T) {
	_, _, err := parseDecimalAmount("")
	require.Error(t, err)
}
