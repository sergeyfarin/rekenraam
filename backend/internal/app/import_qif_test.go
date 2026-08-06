package app

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"rekenraam/backend/internal/db"

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
	assert.Equal(t, "2006-01-15", r0.Date, "dates are normalized to ISO at parse time")
	assert.Equal(t, "01/15/06", r0.Raw["D"], "the raw field keeps the file's own spelling")
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
	require.NotEmpty(t, result.Warnings)
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
	assert.Equal(t, "2006-01-01", result.Meta.DateFrom)
	assert.Equal(t, "2006-06-15", result.Meta.DateTo)
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

// --- EU locale handling (T-35 dates, T-36 decimal commas) ---

func TestParseQIF_EUDatesDetectedFromTheFile(t *testing.T) {
	// A single row with a day above 12 settles the layout for the ambiguous
	// rows around it: without this, 02/03/2026 silently became 3 February.
	qif := `!Type:Bank
D02/03/2026
T-42,50
PAlbert Heijn
^
D15/03/2026
T-19,99
PBol.com
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "ing.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)

	assert.Equal(t, "2026-03-02", result.Rows[0].Date, "the day-first layout proven by row 2 must apply to row 1")
	assert.Equal(t, "2026-03-15", result.Rows[1].Date)
	assert.Equal(t, "2026-03-02", result.Meta.DateFrom)
	assert.Equal(t, "2026-03-15", result.Meta.DateTo)
	assert.Empty(t, result.Warnings, "a file that proves its own layout needs no warning")
}

func TestParseQIF_DecimalCommaAmountsAreNotHundredfold(t *testing.T) {
	qif := `!Type:Bank
D15/03/2026
T-1,50
PKoffie
SEten
$-1,50
^
D16/03/2026
T-1.234,56
PHuur
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "ing.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)

	assert.Equal(t, "-1.50", result.Rows[0].Amount, "a decimal comma must not be stripped as a thousands separator")
	require.Len(t, result.Rows[0].Splits, 1)
	assert.Equal(t, "-1.50", result.Rows[0].Splits[0].Amount, "split amounts get the same treatment as the row amount")
	assert.Equal(t, "-1234.56", result.Rows[1].Amount)
	assert.Equal(t, "-1,50", result.Rows[0].Raw["T"], "the raw field keeps the file's own spelling")
}

func TestParseQIF_AmbiguousDatesWarnAndKeepTheUSDefault(t *testing.T) {
	qif := `!Type:Bank
D01/02/2026
T-10.00
PA
^
D03/04/2026
T-20.00
PB
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)
	assert.Equal(t, "2026-01-02", result.Rows[0].Date)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "ambiguous")
}

func TestParseQIF_UnparseableDateWarnsAndKeepsTheRow(t *testing.T) {
	qif := `!Type:Bank
Dnot-a-date
T-10.00
PA
^
`
	result, err := parseQIF(bufio.NewScanner(strings.NewReader(qif)), "bank.qif")
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "not-a-date", result.Rows[0].Date, "an unparseable date stays verbatim for the user to fix")
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0].Message, "unrecognized date")
}

func TestQIFAdapterParse_ProfileDateLayoutOverridesDetection(t *testing.T) {
	qif := []byte(`!Type:Bank
D01/02/2026
T-10,00
PA
^
`)
	adapter := &QIFAdapter{}
	profile := &ImportProfile{
		AdapterKind: "qif",
		ConfigJSON:  `{"date_layout":"DD/MM/YYYY"}`,
	}

	result, err := adapter.Parse(context.Background(), RawInput{Filename: "ing.qif", Bytes: qif}, profile)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "2026-02-01", result.Rows[0].Date, "the profile settles a date the file itself cannot")
	assert.Empty(t, result.Warnings, "an explicit profile layout leaves nothing ambiguous to warn about")
}

func TestQIFAdapterParse_ProfileDecimalSeparatorOverridesDetection(t *testing.T) {
	// "1,234" defaults to a thousands group; a profile declaring a decimal
	// comma must be honored.
	qif := []byte(`!Type:Bank
D2026-03-15
T-1,234
PA
^
`)
	adapter := &QIFAdapter{}
	profile := &ImportProfile{ConfigJSON: `{"decimal_separator":","}`}

	result, err := adapter.Parse(context.Background(), RawInput{Filename: "ing.qif", Bytes: qif}, profile)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "-1.234", result.Rows[0].Amount)
}

func TestQIFAdapterParse_InvalidProfileConfigIsRejected(t *testing.T) {
	adapter := &QIFAdapter{}
	input := RawInput{Filename: "ing.qif", Bytes: []byte("!Type:Bank\nD01/02/2026\nT-10,00\n^\n")}

	for _, config := range []string{`{not json`, `{"date_layout":"XYZ"}`, `{"decimal_separator":";"}`} {
		t.Run(config, func(t *testing.T) {
			_, err := adapter.Parse(context.Background(), input, &ImportProfile{ConfigJSON: config})
			require.Error(t, err)
			var validationErr ValidationError
			assert.ErrorAs(t, err, &validationErr)
		})
	}
}

func TestQIFAdapterParse_NilProfileFallsBackToDetection(t *testing.T) {
	adapter := &QIFAdapter{}
	input := RawInput{Filename: "ing.qif", Bytes: []byte("!Type:Bank\nD15/02/2026\nT-10,00\n^\n")}

	result, err := adapter.Parse(context.Background(), input, nil)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "2026-02-15", result.Rows[0].Date)
	assert.Equal(t, "-10.00", result.Rows[0].Amount)
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
		// A day above 12 can only be day-first, whatever the file's origin.
		{"15/01/2006", "2006-01-15"},
		{"15.01.2006", "2006-01-15"},
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

func TestBuildTransactionSpec_EUStagedRowKeepsItsDateAndAmount(t *testing.T) {
	// The adapter normalizes before staging, so the commit path sees ISO dates
	// and canonical amounts — but it must also survive a decimal-comma amount
	// reaching it directly (T-36).
	row := db.ImportStagedRowRecord{
		NormalizedJSON: `{"date": "2026-03-02", "amount": "-1.234,56"}`,
	}
	resolution := ImportRowResolution{AccountID: 3, CommodityID: 7, CategoryID: ptrInt64(9)}

	spec, err := buildTransactionSpec(row, resolution)
	require.NoError(t, err)
	assert.Equal(t, "2026-03-02", spec.TransactionDate)

	postings := spec.JournalEntries[0].Postings
	require.Len(t, postings, 2)
	assert.Equal(t, "-123456", string(postings[0].QuantityValue))
	assert.Equal(t, 2, postings[0].QuantityScale)
	assert.Equal(t, "123456", string(postings[1].QuantityValue))
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
		// T-36: a decimal comma is a decimal point, not a 100× thousands group.
		{"1,50", "150", 2},
		{"-1.234,56", "-123456", 2},
		{"1 234,56", "123456", 2},
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
