package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// CSVAdapter maps ordinary bank exports through a saved, data-driven profile.
// Bank-specific layouts belong in profile data, never in this adapter.
type CSVAdapter struct{}

// CSVAnalysis is the canonical header view used to configure a CSV import.
// Parsing the final import repeats the same decoding path with the same raw
// file and requested encoding, so mapped column names cannot drift.
type CSVAnalysis struct {
	Headers            []string
	Delimiter          string
	TextEncoding       string
	EncodingSource     string
	EncodingConfidence int
}

func (a *CSVAdapter) Kind() string { return "csv" }

func (a *CSVAdapter) Detect(input RawInput) Confidence {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(input.Filename)), ".csv") {
		return ConfidenceHigh
	}
	if strings.Contains(strings.ToLower(input.ContentType), "csv") {
		return ConfidenceMedium
	}
	return ConfidenceNone
}

type csvProfileConfig struct {
	Delimiter         string   `json:"delimiter"`
	DateColumn        string   `json:"date_column"`
	PayeeColumn       string   `json:"payee_column"`
	MemoColumn        string   `json:"memo_column"`
	CategoryColumn    string   `json:"category_column"`
	ExternalRefColumn string   `json:"external_ref_column"`
	AmountColumn      string   `json:"amount_column"`
	DebitColumn       string   `json:"debit_column"`
	CreditColumn      string   `json:"credit_column"`
	DateLayout        string   `json:"date_layout"`
	DecimalSeparator  string   `json:"decimal_separator"`
	InvertAmount      bool     `json:"invert_amount"`
	SourceFilename    string   `json:"source_filename"`
	Headers           []string `json:"headers"`
}

func validateCSVProfileConfig(raw string) error {
	_, err := parseCSVProfileConfig(raw)
	return err
}

func parseCSVProfileConfig(raw string) (csvProfileConfig, error) {
	var config csvProfileConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return config, ValidationError{Message: "import profile config is not valid JSON"}
	}
	config.DateColumn = strings.TrimSpace(config.DateColumn)
	config.AmountColumn = strings.TrimSpace(config.AmountColumn)
	config.DebitColumn = strings.TrimSpace(config.DebitColumn)
	config.CreditColumn = strings.TrimSpace(config.CreditColumn)
	config.SourceFilename = strings.TrimSpace(config.SourceFilename)
	seenHeaders := make(map[string]bool, len(config.Headers))
	for index, header := range config.Headers {
		config.Headers[index] = strings.TrimSpace(header)
		if config.Headers[index] == "" || seenHeaders[config.Headers[index]] {
			return config, ValidationError{Message: "csv profile matching headers must be unique and non-empty"}
		}
		seenHeaders[config.Headers[index]] = true
	}
	if config.DateColumn == "" {
		return config, ValidationError{Message: "csv profile date_column is required"}
	}
	hasAmount := config.AmountColumn != ""
	hasDebitCredit := config.DebitColumn != "" && config.CreditColumn != ""
	if hasAmount == hasDebitCredit {
		return config, ValidationError{Message: "csv profile must map either amount_column or both debit_column and credit_column"}
	}
	if _, err := parseDateOrder(config.DateLayout); err != nil {
		return config, ValidationError{Message: err.Error()}
	}
	switch config.DecimalSeparator {
	case "", ".", ",":
	default:
		return config, ValidationError{Message: "csv profile decimal_separator must be \".\" or \",\""}
	}
	if _, err := csvDelimiter(config.Delimiter); err != nil {
		return config, err
	}
	return config, nil
}

func csvDelimiter(value string) (rune, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "comma", ",":
		return ',', nil
	case "semicolon", ";":
		return ';', nil
	case "tab", "\\t":
		return '\t', nil
	default:
		return 0, ValidationError{Message: "csv profile delimiter must be comma, semicolon, or tab"}
	}
}

func (a *CSVAdapter) Parse(_ context.Context, input RawInput, profile *ImportProfile) (ParseResult, error) {
	if profile == nil {
		return ParseResult{}, ValidationError{Message: "csv imports require a saved mapping profile"}
	}
	config, err := parseCSVProfileConfig(profile.ConfigJSON)
	if err != nil {
		return ParseResult{}, err
	}
	delimiter, _ := csvDelimiter(config.Delimiter)
	decoded, err := decodeImportText(input.Bytes, input.TextEncoding)
	if err != nil {
		return ParseResult{}, err
	}
	reader := csv.NewReader(bytes.NewReader(decoded.Bytes))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return ParseResult{}, ValidationError{Message: fmt.Sprintf("csv file could not be read: %v", err)}
	}
	if len(records) < 2 {
		return ParseResult{}, ValidationError{Message: "csv file must contain a header and at least one data row"}
	}

	normalizedHeaders, err := normalizeCSVHeaders(records[0])
	if err != nil {
		return ParseResult{}, err
	}
	headers := make(map[string]int, len(normalizedHeaders))
	for i, header := range normalizedHeaders {
		headers[header] = i
	}
	for _, column := range requiredCSVColumns(config) {
		if _, ok := headers[column]; !ok {
			return ParseResult{}, ValidationError{Message: fmt.Sprintf("csv profile column %q is not present in this file", column)}
		}
	}

	order, _ := parseDateOrder(config.DateLayout)
	if order == dateOrderAuto {
		var dates []string
		for _, record := range records[1:] {
			dates = append(dates, csvCell(record, headers[config.DateColumn]))
		}
		order = detectDateOrder(dates).Order
	}
	result := ParseResult{Meta: SourceMeta{
		TextEncoding:       decoded.Encoding,
		EncodingSource:     decoded.Source,
		EncodingConfidence: decoded.Confidence,
	}}

	var decimalSeparator rune
	if config.DecimalSeparator != "" {
		decimalSeparator = []rune(config.DecimalSeparator)[0]
	} else {
		amounts := make([]string, 0, len(records)-1)
		for _, record := range records[1:] {
			if raw, err := csvAmount(record, headers, config); err == nil {
				amounts = append(amounts, raw)
			}
		}
		detection := detectDecimalSeparatorAcrossValues(amounts)
		decimalSeparator = detection.Separator
		if detection.Conflict {
			result.Warnings = append(result.Warnings, ParseWarning{Message: decimalSeparatorConflictWarning(detection)})
		}
	}
	occurrences := map[string]int{}
	for recordIndex, record := range records[1:] {
		if csvRecordEmpty(record) {
			continue
		}
		rowIndex := len(result.Rows)
		dateRaw := csvValue(record, headers, config.DateColumn)
		date, dateErr := parseFlexibleDate(dateRaw, order)
		amountRaw, amountErr := csvAmount(record, headers, config)
		amount := ""
		if amountErr == nil {
			amount, amountErr = canonicalDecimal(amountRaw, decimalSeparator)
		}
		if config.AmountColumn == "" && amountErr == nil && strings.TrimSpace(csvValue(record, headers, config.DebitColumn)) != "" {
			amount = negativeCanonicalAmount(amount)
		}
		if config.InvertAmount && amountErr == nil {
			amount = invertCanonicalAmount(amount)
		}
		if dateErr != nil {
			result.Warnings = append(result.Warnings, ParseWarning{RowIndex: rowIndex, Message: fmt.Sprintf("row %d has an unrecognized date %q; review before committing", recordIndex+2, dateRaw)})
			date = dateRaw
		}
		if amountErr != nil {
			result.Warnings = append(result.Warnings, ParseWarning{RowIndex: rowIndex, Message: fmt.Sprintf("row %d has an unrecognized amount; review before committing", recordIndex+2)})
			amount = amountRaw
		}
		rawFields := make(map[string]string, len(headers))
		for header, index := range headers {
			rawFields[header] = csvCell(record, index)
		}
		row := StagedRow{
			Date: date, Amount: amount,
			PayeeHint:    csvValue(record, headers, config.PayeeColumn),
			Memo:         csvValue(record, headers, config.MemoColumn),
			CategoryHint: csvValue(record, headers, config.CategoryColumn),
			ExternalRef:  csvValue(record, headers, config.ExternalRefColumn),
			Raw:          rawFields,
		}
		baseFingerprint := fmt.Sprintf("csv|profile:%d|%s|%s|%s|%s", profile.ID, row.Date, row.Amount, strings.ToLower(row.PayeeHint), row.Memo)
		if row.ExternalRef != "" {
			baseFingerprint = fmt.Sprintf("csv|profile:%d|external:%s", profile.ID, row.ExternalRef)
		}
		occurrence := occurrences[baseFingerprint]
		occurrences[baseFingerprint]++
		row.DedupeFingerprint = fmt.Sprintf("%s|occurrence:%d", baseFingerprint, occurrence)
		result.Rows = append(result.Rows, row)
		if dateErr == nil {
			if result.Meta.DateFrom == "" || date < result.Meta.DateFrom {
				result.Meta.DateFrom = date
			}
			if result.Meta.DateTo == "" || date > result.Meta.DateTo {
				result.Meta.DateTo = date
			}
		}
	}
	return result, nil
}

// AnalyzeCSVInput decodes a CSV and returns the exact normalized headers that
// saved mappings and the final parser use. An empty delimiter selects the
// candidate that produces the most header fields outside CSV quoting rules.
func AnalyzeCSVInput(input RawInput, requestedDelimiter string) (CSVAnalysis, error) {
	decoded, err := decodeImportText(input.Bytes, input.TextEncoding)
	if err != nil {
		return CSVAnalysis{}, err
	}

	delimiterName := strings.ToLower(strings.TrimSpace(requestedDelimiter))
	var delimiter rune
	if delimiterName == "" || delimiterName == "auto" {
		delimiterName, delimiter, err = detectCSVDelimiter(decoded.Bytes)
	} else {
		delimiter, err = csvDelimiter(delimiterName)
		delimiterName = canonicalCSVDelimiterName(delimiter)
	}
	if err != nil {
		return CSVAnalysis{}, err
	}

	reader := csv.NewReader(bytes.NewReader(decoded.Bytes))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		return CSVAnalysis{}, ValidationError{Message: fmt.Sprintf("csv header could not be read: %v", err)}
	}
	headers, err = normalizeCSVHeaders(headers)
	if err != nil {
		return CSVAnalysis{}, err
	}
	if len(headers) < 2 {
		return CSVAnalysis{}, ValidationError{Message: "csv header must contain at least two columns"}
	}

	return CSVAnalysis{
		Headers:            headers,
		Delimiter:          delimiterName,
		TextEncoding:       decoded.Encoding,
		EncodingSource:     decoded.Source,
		EncodingConfidence: decoded.Confidence,
	}, nil
}

func detectCSVDelimiter(contents []byte) (string, rune, error) {
	bestName := ""
	var bestDelimiter rune
	bestFields := 0
	for _, candidate := range []string{"comma", "semicolon", "tab"} {
		delimiter, _ := csvDelimiter(candidate)
		reader := csv.NewReader(bytes.NewReader(contents))
		reader.Comma = delimiter
		reader.FieldsPerRecord = -1
		record, err := reader.Read()
		if err == nil && len(record) > bestFields {
			bestName = candidate
			bestDelimiter = delimiter
			bestFields = len(record)
		}
	}
	if bestFields == 0 {
		return "", 0, ValidationError{Message: "csv header could not be read"}
	}
	return bestName, bestDelimiter, nil
}

func canonicalCSVDelimiterName(delimiter rune) string {
	switch delimiter {
	case ';':
		return "semicolon"
	case '\t':
		return "tab"
	default:
		return "comma"
	}
}

func normalizeCSVHeaders(values []string) ([]string, error) {
	headers := make([]string, len(values))
	seen := make(map[string]bool, len(values))
	for index, value := range values {
		header := strings.TrimSpace(value)
		if header == "" {
			return nil, ValidationError{Message: "csv header names must not be empty"}
		}
		if seen[header] {
			return nil, ValidationError{Message: fmt.Sprintf("csv header %q appears more than once", header)}
		}
		headers[index] = header
		seen[header] = true
	}
	return headers, nil
}

func requiredCSVColumns(config csvProfileConfig) []string {
	columns := []string{config.DateColumn}
	for _, column := range []string{config.PayeeColumn, config.MemoColumn, config.CategoryColumn, config.ExternalRefColumn, config.AmountColumn, config.DebitColumn, config.CreditColumn} {
		if strings.TrimSpace(column) != "" {
			columns = append(columns, strings.TrimSpace(column))
		}
	}
	return columns
}

func csvAmount(record []string, headers map[string]int, config csvProfileConfig) (string, error) {
	if config.AmountColumn != "" {
		return csvValue(record, headers, config.AmountColumn), nil
	}
	debit := csvValue(record, headers, config.DebitColumn)
	credit := csvValue(record, headers, config.CreditColumn)
	if strings.TrimSpace(debit) != "" && strings.TrimSpace(credit) != "" {
		return "", fmt.Errorf("both debit and credit are set")
	}
	if strings.TrimSpace(debit) != "" {
		return strings.TrimSpace(debit), nil
	}
	if strings.TrimSpace(credit) != "" {
		return strings.TrimSpace(credit), nil
	}
	return "", io.EOF
}

func csvValue(record []string, headers map[string]int, column string) string {
	if strings.TrimSpace(column) == "" {
		return ""
	}
	return strings.TrimSpace(csvCell(record, headers[column]))
}

func csvCell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	return record[index]
}

func csvRecordEmpty(record []string) bool {
	for _, cell := range record {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func invertCanonicalAmount(value string) string {
	if strings.HasPrefix(value, "-") {
		return strings.TrimPrefix(value, "-")
	}
	if value == "0" || strings.Trim(value, "0.") == "" {
		return value
	}
	return "-" + value
}

func negativeCanonicalAmount(value string) string {
	if strings.HasPrefix(value, "-") || value == "0" || strings.Trim(value, "0.") == "" {
		return value
	}
	return "-" + value
}
