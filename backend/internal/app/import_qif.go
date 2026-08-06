package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// QIFAdapter parses QIF files as exported by MS Money and Quicken.
type QIFAdapter struct{}

func (a *QIFAdapter) Kind() string { return "qif" }

func (a *QIFAdapter) Detect(input RawInput) Confidence {
	name := strings.ToLower(strings.TrimSpace(input.Filename))
	if strings.HasSuffix(name, ".qif") {
		return ConfidenceHigh
	}
	// Sniff for !Type: header
	peek := input.Bytes
	if len(peek) > 512 {
		peek = peek[:512]
	}
	if bytes.Contains(peek, []byte("!Type:")) || bytes.Contains(peek, []byte("!type:")) {
		return ConfidenceMedium
	}
	return ConfidenceNone
}

func (a *QIFAdapter) Parse(ctx context.Context, input RawInput, profile *ImportProfile) (ParseResult, error) {
	opts, err := qifOptionsFromProfile(profile)
	if err != nil {
		return ParseResult{}, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(input.Bytes))
	return parseQIFWithOptions(scanner, input.Filename, opts)
}

// qifOptions carries the locale decisions a QIF file cannot make for itself.
type qifOptions struct {
	// dateOrder is the profile's date_layout; dateOrderAuto means detect it
	// from the file.
	dateOrder dateOrder
	// decimalSeparator is the profile's decimal_separator; 0 means detect it
	// per amount.
	decimalSeparator rune
}

// qifProfileConfig is the subset of an import profile's config_json the QIF
// adapter reads.
type qifProfileConfig struct {
	DateLayout       string `json:"date_layout"`
	DecimalSeparator string `json:"decimal_separator"`
}

func qifOptionsFromProfile(profile *ImportProfile) (qifOptions, error) {
	var opts qifOptions
	if profile == nil || strings.TrimSpace(profile.ConfigJSON) == "" {
		return opts, nil
	}

	var config qifProfileConfig
	if err := json.Unmarshal([]byte(profile.ConfigJSON), &config); err != nil {
		return qifOptions{}, ValidationError{Message: "import profile config is not valid JSON"}
	}

	order, err := parseDateOrder(config.DateLayout)
	if err != nil {
		return qifOptions{}, ValidationError{Message: err.Error()}
	}
	opts.dateOrder = order

	switch strings.TrimSpace(config.DecimalSeparator) {
	case "":
	case ".":
		opts.decimalSeparator = '.'
	case ",":
		opts.decimalSeparator = ','
	default:
		return qifOptions{}, ValidationError{Message: "import profile decimal_separator must be \".\" or \",\""}
	}

	return opts, nil
}

// qifRecord accumulates fields for one QIF transaction record.
type qifRecord struct {
	date        string
	amount      string
	payee       string
	memo        string
	category    string
	externalRef string
	cleared     string
	splits      []qifSplit
	rawFields   map[string][]string
}

type qifSplit struct {
	category string
	memo     string
	amount   string
}

// parseQIF parses a QIF file, detecting its date and decimal layout from its
// own contents. Use parseQIFWithOptions to apply an import profile's overrides.
func parseQIF(scanner *bufio.Scanner, filename string) (ParseResult, error) {
	return parseQIFWithOptions(scanner, filename, qifOptions{})
}

func parseQIFWithOptions(scanner *bufio.Scanner, filename string, opts qifOptions) (ParseResult, error) {
	var result ParseResult
	var current *qifRecord
	var inSplit bool
	var currentSplit qifSplit
	var recordType string // "Bank", "Cash", "CCard", "Invst", etc.
	var records []qifParsedRecord
	var rowIndex int

	flush := func() {
		if current == nil {
			return
		}
		if inSplit && (currentSplit.amount != "" || currentSplit.category != "") {
			current.splits = append(current.splits, currentSplit)
			currentSplit = qifSplit{}
		}
		inSplit = false

		if recordType == "Invst" {
			// Investment rows: mark needs_attention, include in output with a warning.
			result.Warnings = append(result.Warnings, ParseWarning{
				RowIndex: rowIndex,
				Message:  "investment transaction type is partially supported; review before committing",
			})
		}

		records = append(records, qifParsedRecord{record: current, recordType: recordType})
		rowIndex++

		current = nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Strip BOM if present on first line.
		line = strings.TrimPrefix(line, "\xef\xbb\xbf")

		if len(line) == 0 {
			continue
		}

		code := string(line[0])
		value := ""
		if len(line) > 1 {
			value = line[1:]
		}

		switch code {
		case "!":
			// Header line: !Type:Bank, !Type:Cash, !Account, !Option:..., !Clear:...
			if strings.HasPrefix(strings.ToLower(line), "!type:") {
				recordType = strings.TrimPrefix(line[6:], "")
				recordType = strings.TrimSpace(recordType)
			}
			// !Account sections: ignore for now (account hint comes from user mapping)
			continue

		case "^":
			// End-of-record marker.
			flush()

		case "D":
			if current == nil {
				current = newQIFRecord()
			}
			current.date = strings.TrimSpace(value)
			current.rawFields["D"] = append(current.rawFields["D"], current.date)

		case "T", "U":
			// T = amount (signed); U = amount in USD (Quicken investment — same for us).
			if current == nil {
				current = newQIFRecord()
			}
			// Kept verbatim: a comma here may be a thousands separator or a
			// decimal comma, which only the whole-file normalization pass
			// below can tell apart.
			amt := strings.TrimSpace(value)
			current.amount = amt
			current.rawFields[code] = append(current.rawFields[code], amt)

		case "P":
			if current == nil {
				current = newQIFRecord()
			}
			current.payee = strings.TrimSpace(value)
			current.rawFields["P"] = append(current.rawFields["P"], current.payee)

		case "M":
			if current == nil {
				current = newQIFRecord()
			}
			if inSplit {
				currentSplit.memo = strings.TrimSpace(value)
			} else {
				current.memo = strings.TrimSpace(value)
			}
			current.rawFields["M"] = append(current.rawFields["M"], strings.TrimSpace(value))

		case "L":
			if current == nil {
				current = newQIFRecord()
			}
			current.category = strings.TrimSpace(value)
			current.rawFields["L"] = append(current.rawFields["L"], current.category)

		case "N":
			if current == nil {
				current = newQIFRecord()
			}
			current.externalRef = strings.TrimSpace(value)
			current.rawFields["N"] = append(current.rawFields["N"], current.externalRef)

		case "C":
			if current == nil {
				current = newQIFRecord()
			}
			current.cleared = strings.TrimSpace(value)
			current.rawFields["C"] = append(current.rawFields["C"], current.cleared)

		case "S":
			// Split category.
			if current == nil {
				current = newQIFRecord()
			}
			if inSplit && (currentSplit.amount != "" || currentSplit.category != "") {
				current.splits = append(current.splits, currentSplit)
				currentSplit = qifSplit{}
			}
			inSplit = true
			currentSplit.category = strings.TrimSpace(value)
			current.rawFields["S"] = append(current.rawFields["S"], currentSplit.category)

		case "E":
			// Split memo.
			if current != nil && inSplit {
				currentSplit.memo = strings.TrimSpace(value)
				current.rawFields["E"] = append(current.rawFields["E"], currentSplit.memo)
			}

		case "$":
			// Split amount.
			if current != nil && inSplit {
				amt := strings.TrimSpace(value)
				currentSplit.amount = amt
				current.rawFields["$"] = append(current.rawFields["$"], currentSplit.amount)
			}

		default:
			if current == nil {
				current = newQIFRecord()
			}
			current.rawFields[code] = append(current.rawFields[code], strings.TrimSpace(value))
		}
	}

	// Flush any trailing record (file may not end with ^).
	flush()

	if err := scanner.Err(); err != nil {
		return ParseResult{}, fmt.Errorf("scan qif: %w", err)
	}

	normalizeQIFRecords(&result, records, filename, opts)
	return result, nil
}

// qifParsedRecord is one record awaiting the whole-file normalization pass.
type qifParsedRecord struct {
	record     *qifRecord
	recordType string
}

// normalizeQIFRecords rewrites every record's date to ISO and every amount to
// canonical form, then builds the staged rows. It runs after the whole file has
// been read because that is the only point where the date field order is
// knowable: a single row with a day > 12 settles it for all the ambiguous ones.
func normalizeQIFRecords(result *ParseResult, records []qifParsedRecord, filename string, opts qifOptions) {
	order := opts.dateOrder
	if order == dateOrderAuto {
		dates := make([]string, 0, len(records))
		for _, r := range records {
			dates = append(dates, r.record.date)
		}
		detection := detectDateOrder(dates)
		order = detection.Order
		switch {
		case detection.Conflict:
			result.Warnings = append(result.Warnings, ParseWarning{
				Message: fmt.Sprintf("dates in this file disagree on their layout; parsed as %s — set a date layout on an import profile if that is wrong", order),
			})
		case !detection.Decisive && detection.Ambiguous > 0:
			result.Warnings = append(result.Warnings, ParseWarning{
				Message: fmt.Sprintf("every date in this file is ambiguous (no day above 12); parsed as %s — set a date layout on an import profile if that is wrong", order),
			})
		}
	}

	for i, parsed := range records {
		rec := parsed.record

		if rec.date != "" {
			iso, err := parseFlexibleDate(rec.date, order)
			if err != nil {
				result.Warnings = append(result.Warnings, ParseWarning{
					RowIndex: i,
					Message:  fmt.Sprintf("unrecognized date %q; review before committing", rec.date),
				})
			} else {
				rec.date = iso
			}
		}

		if rec.amount != "" {
			canonical, err := canonicalDecimal(rec.amount, opts.decimalSeparator)
			if err != nil {
				result.Warnings = append(result.Warnings, ParseWarning{
					RowIndex: i,
					Message:  fmt.Sprintf("unrecognized amount %q; review before committing", rec.amount),
				})
			} else {
				rec.amount = canonical
			}
		}

		for j, split := range rec.splits {
			if split.amount == "" {
				continue
			}
			canonical, err := canonicalDecimal(split.amount, opts.decimalSeparator)
			if err != nil {
				result.Warnings = append(result.Warnings, ParseWarning{
					RowIndex: i,
					Message:  fmt.Sprintf("unrecognized split amount %q; review before committing", split.amount),
				})
				continue
			}
			rec.splits[j].amount = canonical
		}

		row := qifRecordToStagedRow(rec, i, parsed.recordType, filename)
		result.Rows = append(result.Rows, row)

		// Date range metadata: ISO dates compare correctly as strings.
		if row.Date != "" {
			if result.Meta.DateFrom == "" || row.Date < result.Meta.DateFrom {
				result.Meta.DateFrom = row.Date
			}
			if result.Meta.DateTo == "" || row.Date > result.Meta.DateTo {
				result.Meta.DateTo = row.Date
			}
		}
	}
}

func newQIFRecord() *qifRecord {
	return &qifRecord{rawFields: make(map[string][]string)}
}

func qifRecordToStagedRow(rec *qifRecord, occurrenceIndex int, recordType, filename string) StagedRow {
	// Build raw map (flatten multi-value fields to first value for display).
	rawCap := 2
	if len(rec.rawFields) <= math.MaxInt-2 {
		rawCap += len(rec.rawFields)
	}
	raw := make(map[string]string, rawCap)
	for k, vs := range rec.rawFields {
		raw[k] = strings.Join(vs, "|")
	}
	raw["_record_type"] = recordType
	raw["_source_file"] = filename

	// Detect transfer: category field like [Account Name].
	transferHint := ""
	categoryHint := rec.category
	if strings.HasPrefix(categoryHint, "[") && strings.HasSuffix(categoryHint, "]") {
		transferHint = categoryHint[1 : len(categoryHint)-1]
		categoryHint = ""
	}

	var splits []StagedSplit
	for _, s := range rec.splits {
		cat := s.category
		xfer := ""
		if strings.HasPrefix(cat, "[") && strings.HasSuffix(cat, "]") {
			xfer = cat[1 : len(cat)-1]
			cat = ""
		}
		memo := s.memo
		if xfer != "" && cat == "" {
			memo = "[transfer:" + xfer + "]"
		}
		splits = append(splits, StagedSplit{
			CategoryHint: cat,
			Amount:       s.amount,
			Memo:         memo,
		})
	}

	// Build the dedupe fingerprint.
	// QIF has no bank-provided transaction ID, so we use a scoped content hash.
	fp := buildQIFFingerprint(filename, rec, occurrenceIndex)

	return StagedRow{
		DedupeFingerprint: fp,
		Date:              rec.date,
		Amount:            rec.amount,
		PayeeHint:         rec.payee,
		CategoryHint:      categoryHint,
		TransferHint:      transferHint,
		Memo:              rec.memo,
		ExternalRef:       rec.externalRef,
		Splits:            splits,
		Raw:               raw,
	}
}

func buildQIFFingerprint(filename string, rec *qifRecord, occurrence int) string {
	// Scoped content hash: source_kind|filename|date|amount|payee|memo|occurrence
	// We use a simple deterministic string rather than crypto hash for readability/debuggability.
	// The service layer will hash this to a fixed-length fingerprint.
	return fmt.Sprintf("qif|%s|%s|%s|%s|%s|%d",
		filename,
		rec.date,
		rec.amount,
		rec.payee,
		rec.memo,
		occurrence,
	)
}
