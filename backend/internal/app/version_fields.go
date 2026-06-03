package app

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const changeReasonMaxBytes = 500

func cleanEffectiveFrom(value string, now time.Time) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return now.UTC().Format(time.DateOnly), nil
	}

	parsed, err := time.Parse(time.DateOnly, cleaned)
	if err != nil {
		return "", ValidationError{Message: "effective date must use YYYY-MM-DD"}
	}

	today, _ := time.Parse(time.DateOnly, now.UTC().Format(time.DateOnly))
	if parsed.After(today) {
		return "", ValidationError{Message: "effective date must not be in the future"}
	}

	return cleaned, nil
}

func cleanChangeReason(value string, fallback string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		cleaned = fallback
	}
	if len(cleaned) > changeReasonMaxBytes {
		return "", ValidationError{Message: fmt.Sprintf("change reason must be at most %d bytes", changeReasonMaxBytes)}
	}
	for _, r := range cleaned {
		if unicode.IsControl(r) {
			return "", ValidationError{Message: "change reason must not contain control characters"}
		}
	}

	return cleaned, nil
}
