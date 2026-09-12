package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"
)

func TestCSVAdapterTwoBankLayoutsUseProfileDataWithoutCodeChanges(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		contents   string
		wantDate   string
		wantAmount string
		wantPayee  string
	}{
		{
			name:     "European semicolon and decimal comma",
			config:   `{"delimiter":"semicolon","date_column":"Datum","payee_column":"Omschrijving","amount_column":"Bedrag","date_layout":"DMY","decimal_separator":","}`,
			contents: "Datum;Omschrijving;Bedrag\n28/08/2026;Bakker;\"-1.234,56\"\n",
			wantDate: "2026-08-28", wantAmount: "-1234.56", wantPayee: "Bakker",
		},
		{
			name:     "US comma with separate debit and credit",
			config:   `{"delimiter":"comma","date_column":"Date","payee_column":"Description","debit_column":"Debit","credit_column":"Credit","date_layout":"MDY","decimal_separator":"."}`,
			contents: "Date,Description,Debit,Credit\n08/28/2026,Coffee,(12.34),\n08/29/2026,Salary,,2500.00\n",
			wantDate: "2026-08-28", wantAmount: "-12.34", wantPayee: "Coffee",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := (&CSVAdapter{}).Parse(context.Background(), RawInput{Filename: "statement.csv", Bytes: []byte(test.contents)}, &ImportProfile{ID: 7, AdapterKind: "csv", ConfigJSON: test.config})
			require.NoError(t, err)
			require.NotEmpty(t, result.Rows)
			assert.Equal(t, test.wantDate, result.Rows[0].Date)
			assert.Equal(t, test.wantAmount, result.Rows[0].Amount)
			assert.Equal(t, test.wantPayee, result.Rows[0].PayeeHint)
			assert.Empty(t, result.Warnings)
		})
	}
}

// A German export with no decimal_separator on the profile. Read one row at a
// time, "1.234" is indistinguishable from 1.234 — but this file settles it:
// "56,78" can only be a decimal comma, which makes every period grouping.
// Getting this wrong turns 1234,00 into 1.234, a silent 1000x error on exactly
// the migration path the announcement leads with.
func TestCSVAdapterUsesTheWholeFileToSettleAnAmbiguousDecimalSeparator(t *testing.T) {
	config := `{"delimiter":"semicolon","date_column":"Datum","payee_column":"Beschreibung","amount_column":"Betrag","date_layout":"DMY"}`
	contents := "Datum;Beschreibung;Betrag\n28/08/2026;Miete;-1.234\n29/08/2026;Kaffee;-56,78\n30/08/2026;Gehalt;2.500\n"

	result, err := (&CSVAdapter{}).Parse(context.Background(),
		RawInput{Filename: "statement.csv", Bytes: []byte(contents)},
		&ImportProfile{ID: 9, AdapterKind: "csv", ConfigJSON: config})
	require.NoError(t, err)
	require.Len(t, result.Rows, 3)

	assert.Equal(t, "-1234", result.Rows[0].Amount, "a period in a decimal-comma file is grouping, not a decimal point")
	assert.Equal(t, "-56.78", result.Rows[1].Amount)
	assert.Equal(t, "2500", result.Rows[2].Amount)
	assert.Empty(t, result.Warnings)
}

// The same shape in reverse: a US file where a lone "1,234" must stay 1234.
func TestCSVAdapterKeepsUSGroupingWhenTheFileSettlesOnADecimalPoint(t *testing.T) {
	config := `{"delimiter":"comma","date_column":"Date","payee_column":"Description","amount_column":"Amount","date_layout":"MDY"}`
	contents := "Date,Description,Amount\n08/28/2026,Rent,\"-1,234\"\n08/29/2026,Coffee,-56.78\n"

	result, err := (&CSVAdapter{}).Parse(context.Background(),
		RawInput{Filename: "statement.csv", Bytes: []byte(contents)},
		&ImportProfile{ID: 10, AdapterKind: "csv", ConfigJSON: config})
	require.NoError(t, err)
	require.Len(t, result.Rows, 2)

	assert.Equal(t, "-1234", result.Rows[0].Amount)
	assert.Equal(t, "-56.78", result.Rows[1].Amount)
	assert.Empty(t, result.Warnings)
}

func TestCSVAdapterParse_LegacyEncodingUsesSameDecoderAsQIF(t *testing.T) {
	contents, err := charmap.Windows1250.NewEncoder().Bytes([]byte("Data;Opis;Kwota\n28/08/2026;Zażółć;-12,34\n"))
	require.NoError(t, err)
	profile := &ImportProfile{ID: 8, AdapterKind: "csv", ConfigJSON: `{"delimiter":"semicolon","date_column":"Data","payee_column":"Opis","amount_column":"Kwota","date_layout":"DMY","decimal_separator":","}`}

	result, err := (&CSVAdapter{}).Parse(context.Background(), RawInput{
		Filename: "statement.csv", Bytes: contents, TextEncoding: "windows-1250",
	}, profile)
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "Zażółć", result.Rows[0].PayeeHint)
	assert.Equal(t, "windows-1250", result.Meta.TextEncoding)
	assert.Equal(t, "selected", result.Meta.EncodingSource)
}

func TestAnalyzeCSVInput_DecodesHeadersAndDetectsDelimiter(t *testing.T) {
	contents, err := charmap.Windows1251.NewEncoder().Bytes([]byte("Дата;Описание;Сумма\n24.04.2021;Кафе;-42,49\n"))
	require.NoError(t, err)

	result, err := AnalyzeCSVInput(RawInput{Filename: "statement.csv", Bytes: contents}, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"Дата", "Описание", "Сумма"}, result.Headers)
	assert.Equal(t, "semicolon", result.Delimiter)
	assert.Equal(t, "windows-1251", result.TextEncoding)
	assert.Equal(t, "detected", result.EncodingSource)
	assert.GreaterOrEqual(t, result.EncodingConfidence, minimumAutomaticEncodingConfidence)
}

func TestAnalyzeCSVInput_UsesCSVQuotingRulesForCanonicalHeaders(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		delimiter string
		headers   []string
	}{
		{
			name:      "semicolon inside quoted header",
			contents:  "Datum;Omschrijving;\"Bedrag; EUR\"\n28/08/2026;Koffie;-3,50\n",
			delimiter: "semicolon",
			headers:   []string{"Datum", "Omschrijving", "Bedrag; EUR"},
		},
		{
			name:      "escaped quote in comma header",
			contents:  "Date,\"Payee \"\"name\"\"\",Amount\n2026-08-28,Cafe,-3.50\n",
			delimiter: "comma",
			headers:   []string{"Date", `Payee "name"`, "Amount"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := AnalyzeCSVInput(RawInput{Filename: "statement.csv", Bytes: []byte(test.contents)}, "")
			require.NoError(t, err)
			assert.Equal(t, test.delimiter, result.Delimiter)
			assert.Equal(t, test.headers, result.Headers)
		})
	}
}

func TestAnalyzeCSVInput_RejectsDuplicateHeaders(t *testing.T) {
	_, err := AnalyzeCSVInput(RawInput{Filename: "statement.csv", Bytes: []byte("Date,Amount,Amount\n")}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `header "Amount" appears more than once`)
}

func TestCSVAdapterRejectsProfileColumnMissingFromStatement(t *testing.T) {
	_, err := (&CSVAdapter{}).Parse(context.Background(), RawInput{Filename: "statement.csv", Bytes: []byte("Date,Value\n2026-08-28,-1.00\n")}, &ImportProfile{ID: 1, AdapterKind: "csv", ConfigJSON: `{"date_column":"Date","amount_column":"Amount","date_layout":"YMD","decimal_separator":"."}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `column "Amount" is not present`)
}

func TestCSVAdapterRejectsAmbiguousAmountMapping(t *testing.T) {
	for _, config := range []string{
		`{"date_column":"Date","date_layout":"YMD"}`,
		`{"date_column":"Date","amount_column":"Amount","debit_column":"Debit","credit_column":"Credit","date_layout":"YMD"}`,
	} {
		err := validateCSVProfileConfig(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either amount_column or both debit_column and credit_column")
	}
}

func TestCSVAdapterDetect(t *testing.T) {
	adapter := &CSVAdapter{}
	assert.Equal(t, ConfidenceHigh, adapter.Detect(RawInput{Filename: "statement.CSV"}))
	assert.Equal(t, ConfidenceMedium, adapter.Detect(RawInput{ContentType: "text/csv"}))
	assert.Equal(t, ConfidenceNone, adapter.Detect(RawInput{Filename: "statement.qif"}))
}
