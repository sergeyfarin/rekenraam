package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

type ForecastEventsInput struct {
	ForecastInput
	Date              string
	BasisToken        string
	DetailAccountID   *int64
	DetailCommodityID *int64
	Limit             int
	Cursor            string
}

type ForecastEventsResult struct {
	BasisToken string
	Date       string
	Items      []ForecastEvent
	TotalCount int
	NextCursor string
}

type forecastEventCursor struct {
	Version           int    `json:"v"`
	BasisToken        string `json:"basis_token"`
	Date              string `json:"date"`
	DetailAccountID   *int64 `json:"detail_account_id"`
	DetailCommodityID *int64 `json:"detail_commodity_id"`
	Source            string `json:"source"`
	SourceID          int64  `json:"source_id"`
	EntryID           int64  `json:"entry_id"`
	OccurrenceDate    string `json:"occurrence_date"`
	Key               string `json:"key"`
}

func (s *ForecastService) Events(ctx context.Context, input ForecastEventsInput) (ForecastEventsResult, error) {
	if input.Limit == 0 {
		input.Limit = ForecastDefaultEventLimit
	}
	if input.Limit < 1 || input.Limit > ForecastMaxEventLimit {
		return ForecastEventsResult{}, ValidationError{Message: "forecast event limit must be between 1 and 200"}
	}
	if input.Date == "" {
		return ForecastEventsResult{}, ValidationError{Message: "forecast event date is required"}
	}
	if _, err := time.Parse(time.DateOnly, input.Date); err != nil {
		return ForecastEventsResult{}, ValidationError{Message: "forecast event date is invalid"}
	}
	if !validForecastBasisToken(input.BasisToken) {
		return ForecastEventsResult{}, ValidationError{Message: "forecast basis token is invalid"}
	}

	result, err := s.Balances(ctx, input.ForecastInput)
	if err != nil {
		return ForecastEventsResult{}, err
	}
	if result.BasisToken != input.BasisToken {
		return ForecastEventsResult{}, ErrForecastBasisChanged
	}
	if input.Date < result.FirstDate || input.Date > result.ThroughDate {
		return ForecastEventsResult{}, ValidationError{Message: "forecast event date is outside the requested horizon"}
	}
	if input.DetailAccountID != nil && !containsInt64(result.AccountIDs, *input.DetailAccountID) {
		return ForecastEventsResult{}, ValidationError{Message: "forecast detail account is outside the resolved scope"}
	}
	if input.DetailCommodityID != nil {
		found := false
		for _, series := range result.Aggregates {
			found = found || series.CommodityID == *input.DetailCommodityID
		}
		if !found {
			return ForecastEventsResult{}, ValidationError{Message: "forecast detail commodity is not present"}
		}
	}

	items := make([]ForecastEvent, 0)
	for _, event := range result.Events {
		if event.ProjectedDate != input.Date || !forecastEventMatches(event, input.DetailAccountID, input.DetailCommodityID) {
			continue
		}
		filtered := event
		filtered.Amounts = filterForecastEventAmounts(event.Amounts, input.DetailAccountID, input.DetailCommodityID)
		items = append(items, filtered)
	}
	total := len(items)
	start := 0
	if input.Cursor != "" {
		cursor, err := decodeForecastEventCursor(input.Cursor)
		if err != nil {
			return ForecastEventsResult{}, err
		}
		if cursor.BasisToken != result.BasisToken {
			return ForecastEventsResult{}, ErrForecastBasisChanged
		}
		if cursor.Date != input.Date || !sameOptionalInt64(cursor.DetailAccountID, input.DetailAccountID) || !sameOptionalInt64(cursor.DetailCommodityID, input.DetailCommodityID) {
			return ForecastEventsResult{}, ValidationError{Message: "forecast cursor does not match the requested details"}
		}
		found := false
		for index, event := range items {
			if forecastEventMatchesCursor(event, cursor) {
				start, found = index+1, true
				break
			}
		}
		if !found {
			return ForecastEventsResult{}, ValidationError{Message: "forecast cursor position is invalid"}
		}
	}
	end := min(start+input.Limit, len(items))
	page := append([]ForecastEvent{}, items[start:end]...)
	next := ""
	if end < len(items) {
		next, err = encodeForecastEventCursor(result.BasisToken, input.Date, input.DetailAccountID, input.DetailCommodityID, page[len(page)-1])
		if err != nil {
			return ForecastEventsResult{}, err
		}
	}
	return ForecastEventsResult{BasisToken: result.BasisToken, Date: input.Date, Items: page, TotalCount: total, NextCursor: next}, nil
}

func validForecastBasisToken(value string) bool {
	if len(value) != sha256HexLength {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

const sha256HexLength = 64

func forecastEventMatches(event ForecastEvent, accountID, commodityID *int64) bool {
	for _, amount := range event.Amounts {
		if (accountID == nil || amount.AccountID == *accountID) && (commodityID == nil || amount.CommodityID == *commodityID) {
			return true
		}
	}
	return false
}

func filterForecastEventAmounts(amounts []ForecastEventAmount, accountID, commodityID *int64) []ForecastEventAmount {
	result := make([]ForecastEventAmount, 0, len(amounts))
	for _, amount := range amounts {
		if (accountID == nil || amount.AccountID == *accountID) && (commodityID == nil || amount.CommodityID == *commodityID) {
			result = append(result, amount)
		}
	}
	return result
}

func encodeForecastEventCursor(basis, date string, accountID, commodityID *int64, event ForecastEvent) (string, error) {
	payload, err := json.Marshal(forecastEventCursor{Version: 1, BasisToken: basis, Date: date, DetailAccountID: accountID, DetailCommodityID: commodityID, Source: event.Source, SourceID: event.SourceID, EntryID: event.EntryID, OccurrenceDate: event.OccurrenceDate, Key: event.Key})
	if err != nil {
		return "", fmt.Errorf("encode forecast event cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeForecastEventCursor(value string) (forecastEventCursor, error) {
	if len(value) > ForecastMaxCursorBytes {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is too large"}
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor forecastEventCursor
	if err := decoder.Decode(&cursor); err != nil {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	estimated := cursor.Source == "estimated_spending"
	if cursor.Version != 1 || !validForecastBasisToken(cursor.BasisToken) || cursor.Date == "" || cursor.Key == "" || cursor.EntryID < 0 ||
		(cursor.Source != "posted" && cursor.Source != "draft" && cursor.Source != "template" && !estimated) {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	// Estimated events carry no saved record, so their source id is zero by
	// design; every other source must still reference a real row.
	if estimated != (cursor.SourceID == 0) {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	if !estimated && cursor.SourceID < 0 {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	if cursor.DetailAccountID != nil && *cursor.DetailAccountID <= 0 || cursor.DetailCommodityID != nil && *cursor.DetailCommodityID <= 0 {
		return forecastEventCursor{}, ValidationError{Message: "forecast cursor is invalid"}
	}
	return cursor, nil
}

func forecastEventMatchesCursor(event ForecastEvent, cursor forecastEventCursor) bool {
	return event.Source == cursor.Source && event.SourceID == cursor.SourceID && event.EntryID == cursor.EntryID && event.OccurrenceDate == cursor.OccurrenceDate && event.Key == cursor.Key
}

func containsInt64(values []int64, target int64) bool {
	index := sort.Search(len(values), func(index int) bool { return values[index] >= target })
	return index < len(values) && values[index] == target
}

func sameOptionalInt64(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
