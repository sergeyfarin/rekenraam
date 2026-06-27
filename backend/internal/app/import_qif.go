package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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
	dateLayout := "01/02/06" // US MM/DD/YY default; overridable via profile
	if profile != nil {
		// Future: parse profile.ConfigJSON for date_layout
		_ = profile
	}
	_ = dateLayout // layout stored in raw; normalization step applies it

	scanner := bufio.NewScanner(bytes.NewReader(input.Bytes))
	return parseQIF(scanner, input.Filename)
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

func parseQIF(scanner *bufio.Scanner, filename string) (ParseResult, error) {
	var result ParseResult
	var current *qifRecord
	var inSplit bool
	var currentSplit qifSplit
	var recordType string   // "Bank", "Cash", "CCard", "Invst", etc.
	var occurrenceIndex int // within-file occurrence counter for fingerprinting
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

		row := qifRecordToStagedRow(current, occurrenceIndex, recordType, filename)
		result.Rows = append(result.Rows, row)
		occurrenceIndex++
		rowIndex++

		// Update date range metadata.
		if row.Date != "" {
			if result.Meta.DateFrom == "" || row.Date < result.Meta.DateFrom {
				result.Meta.DateFrom = row.Date
			}
			if result.Meta.DateTo == "" || row.Date > result.Meta.DateTo {
				result.Meta.DateTo = row.Date
			}
		}

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
			amt := strings.TrimSpace(value)
			// Remove thousands separators (commas in US QIF).
			amt = strings.ReplaceAll(amt, ",", "")
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
				amt = strings.ReplaceAll(amt, ",", "")
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

	return result, nil
}

func newQIFRecord() *qifRecord {
	return &qifRecord{rawFields: make(map[string][]string)}
}

func qifRecordToStagedRow(rec *qifRecord, occurrenceIndex int, recordType, filename string) StagedRow {
	// Build raw map (flatten multi-value fields to first value for display).
	raw := make(map[string]string, len(rec.rawFields)+2)
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
