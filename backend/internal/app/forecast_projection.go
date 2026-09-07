package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/recur"
)

type forecastPair struct {
	AccountID   int64
	CommodityID int64
}

type forecastMovement struct {
	Posted   *exact.ScaledInt
	Draft    *exact.ScaledInt
	Template *exact.ScaledInt
}

type forecastBuild struct {
	ctx         context.Context
	bounds      forecastBounds
	scope       forecastScope
	snapshot    db.ForecastSnapshot
	selected    map[int64]bool
	opening     map[forecastPair]*exact.ScaledInt
	movements   map[forecastPair]map[string]*forecastMovement
	scales      map[forecastPair]int
	events      []ForecastEvent
	diagnostics []ForecastDiagnostic
	incomplete  bool
	candidates  int
	rules       forecastRuleLookup
}

func buildForecast(ctx context.Context, input forecastNormalizedInput, bounds forecastBounds, scope forecastScope, snapshot db.ForecastSnapshot, computedAt string) (ForecastResult, error) {
	if err := ctx.Err(); err != nil {
		return ForecastResult{}, err
	}
	b := &forecastBuild{ctx: ctx, bounds: bounds, scope: scope, snapshot: snapshot,
		selected: map[int64]bool{}, opening: map[forecastPair]*exact.ScaledInt{}, movements: map[forecastPair]map[string]*forecastMovement{}, scales: map[forecastPair]int{}, rules: newForecastRuleLookup(snapshot)}
	for _, id := range scope.AccountIDs {
		b.selected[id] = true
	}
	if err := b.addPosted(); err != nil {
		return ForecastResult{}, err
	}
	b.addZeroDefaultSeries()
	acted := map[string]db.ForecastOccurrenceRecord{}
	for _, occurrence := range snapshot.Occurrences {
		acted[forecastOccurrenceKey(occurrence.TemplateID, occurrence.OccurrenceDate)] = occurrence
	}
	if err := b.addDrafts(); err != nil {
		return ForecastResult{}, err
	}
	b.addOccurrenceDiagnostics()
	if err := b.addTemplates(acted); err != nil {
		return ForecastResult{}, err
	}
	return b.finish(input, computedAt)
}

func (b *forecastBuild) addPosted() error {
	groups := map[int64][]db.ForecastPostingRecord{}
	entryOrder := []int64{}
	for _, posting := range b.snapshot.PostedPostings {
		if posting.EntryDate > b.bounds.Through {
			continue
		}
		commodity, ok := b.rules.commodityAt(posting.CommodityID, posting.EntryDate)
		if !ok || commodity.Kind != "currency" {
			continue
		}
		pair := forecastPair{posting.AccountID, posting.CommodityID}
		b.noteScale(pair, posting.QuantityScale)
		if posting.EntryDate <= b.bounds.AsOf {
			b.accumulator(b.opening, pair).AddCoefficient(posting.QuantityValue, posting.QuantityScale)
			continue
		}
		if _, ok := groups[posting.JournalEntryID]; !ok {
			entryOrder = append(entryOrder, posting.JournalEntryID)
		}
		groups[posting.JournalEntryID] = append(groups[posting.JournalEntryID], posting)
	}
	for _, entryID := range entryOrder {
		rows := groups[entryID]
		if len(rows) == 0 {
			continue
		}
		event := ForecastEvent{Key: fmt.Sprintf("posted:%d:%d:%d", rows[0].TransactionID, rows[0].TransactionVersionID, entryID), Source: "posted", SourceID: rows[0].TransactionID, EntryID: entryID, SourceDate: rows[0].EntryDate, ProjectedDate: rows[0].EntryDate, Amounts: []ForecastEventAmount{}}
		for _, row := range rows {
			pair := forecastPair{row.AccountID, row.CommodityID}
			b.movement(pair, row.EntryDate).Posted.AddCoefficient(row.QuantityValue, row.QuantityScale)
			event.Amounts = append(event.Amounts, ForecastEventAmount{AccountID: row.AccountID, CommodityID: row.CommodityID, Quantity: ForecastQuantity{Value: row.QuantityValue, Scale: row.QuantityScale}})
		}
		b.events = append(b.events, event)
	}
	return nil
}

func (b *forecastBuild) addZeroDefaultSeries() {
	commodities := commodityRulesAt(b.snapshot.CommodityVersions, b.bounds.AsOf)
	for id, account := range b.scope.Accounts {
		if !account.DefaultCommodityID.Valid {
			continue
		}
		commodity, ok := commodities[account.DefaultCommodityID.Int64]
		if !ok || commodity.Kind != "currency" {
			continue
		}
		pair := forecastPair{id, commodity.CommodityID}
		scale := commodity.StandardScale
		if account.QuantityScaleOverride.Valid {
			scale = int(account.QuantityScaleOverride.Int64)
		}
		b.noteScale(pair, scale)
		b.accumulator(b.opening, pair)
	}
}

func (b *forecastBuild) addDrafts() error {
	type draftEntry struct {
		ID       int64
		Date     string
		Postings []forecastPosting
		Rows     []db.ForecastDraftPostingRecord
	}
	type draftTransaction struct {
		OccurrenceID   int64
		TemplateID     int64
		OccurrenceDate string
		TransactionID  int64
		VersionID      int64
		Kind           string
		PayeeName      string
		Entries        map[int64]*draftEntry
	}
	transactions := map[int64]*draftTransaction{}
	for _, row := range b.snapshot.DraftPostings {
		transaction := transactions[row.TransactionID]
		if transaction == nil {
			payee := row.PayeeName.String
			if row.PayeeID.Valid && b.snapshot.PayeeNames[row.PayeeID.Int64] != "" {
				payee = b.snapshot.PayeeNames[row.PayeeID.Int64]
			}
			transaction = &draftTransaction{OccurrenceID: row.OccurrenceID, TemplateID: row.TemplateID, OccurrenceDate: row.OccurrenceDate, TransactionID: row.TransactionID, VersionID: row.TransactionVersionID, Kind: row.TransactionKind, PayeeName: payee, Entries: map[int64]*draftEntry{}}
			transactions[row.TransactionID] = transaction
		}
		entry := transaction.Entries[row.JournalEntryID]
		if entry == nil {
			entry = &draftEntry{ID: row.JournalEntryID, Date: row.EntryDate}
			transaction.Entries[row.JournalEntryID] = entry
		}
		entry.Postings = append(entry.Postings, forecastPosting{row.AccountID, row.CommodityID, row.QuantityValue, row.QuantityScale})
		entry.Rows = append(entry.Rows, row)
	}
	ids := make([]int64, 0, len(transactions))
	for id := range transactions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		transaction := transactions[id]
		entryIDs := make([]int64, 0, len(transaction.Entries))
		valid := transaction.Kind == "ordinary" || transaction.Kind == "transfer"
		relevant := false
		for entryID := range transaction.Entries {
			entryIDs = append(entryIDs, entryID)
		}
		sort.Slice(entryIDs, func(i, j int) bool { return entryIDs[i] < entryIDs[j] })
		for _, entryID := range entryIDs {
			entry := transaction.Entries[entryID]
			projectionDate := max(entry.Date, b.bounds.First)
			if projectionDate <= b.bounds.Through {
				relevant = true
				if err := b.countCandidate(); err != nil {
					return err
				}
			}
			if !validateForecastPostings(entry.Postings, entry.Date, projectionDate, b.rules) {
				valid = false
			}
		}
		if !relevant {
			continue
		}
		if !valid || len(entryIDs) == 0 {
			b.diagnose(ForecastDiagnostic{Code: "invalid_draft", TemplateID: transaction.TemplateID, OccurrenceID: transaction.OccurrenceID, OccurrenceDate: transaction.OccurrenceDate}, true)
			continue
		}
		for _, entryID := range entryIDs {
			entry := transaction.Entries[entryID]
			projectionDate := max(entry.Date, b.bounds.First)
			if projectionDate > b.bounds.Through {
				continue
			}
			event := ForecastEvent{Key: fmt.Sprintf("draft:%d:%d:%d", transaction.OccurrenceID, transaction.VersionID, entry.ID), Source: "draft", SourceID: transaction.TransactionID, EntryID: entry.ID, TemplateID: transaction.TemplateID, OccurrenceID: transaction.OccurrenceID, OccurrenceDate: transaction.OccurrenceDate, SourceDate: entry.Date, ProjectedDate: projectionDate, CarriedForward: entry.Date < b.bounds.First, PayeeName: transaction.PayeeName, Amounts: []ForecastEventAmount{}}
			for _, row := range entry.Rows {
				if !b.selected[row.AccountID] {
					continue
				}
				commodity, ok := b.rules.commodityAt(row.CommodityID, entry.Date)
				if !ok || commodity.Kind != "currency" {
					continue
				}
				pair := forecastPair{row.AccountID, row.CommodityID}
				b.noteScale(pair, row.QuantityScale)
				b.movement(pair, projectionDate).Draft.AddCoefficient(row.QuantityValue, row.QuantityScale)
				event.Amounts = append(event.Amounts, ForecastEventAmount{AccountID: row.AccountID, CommodityID: row.CommodityID, Quantity: ForecastQuantity{Value: row.QuantityValue, Scale: row.QuantityScale}})
			}
			if len(event.Amounts) > 0 {
				b.events = append(b.events, event)
				if event.CarriedForward {
					b.diagnose(ForecastDiagnostic{Code: "carried_forward", TemplateID: event.TemplateID, OccurrenceID: event.OccurrenceID, OccurrenceDate: event.OccurrenceDate, SourceDate: event.SourceDate}, false)
				}
			}
		}
	}
	return nil
}

func (b *forecastBuild) addOccurrenceDiagnostics() {
	draftOccurrence := map[int64]bool{}
	for _, row := range b.snapshot.DraftPostings {
		draftOccurrence[row.OccurrenceID] = true
	}
	for _, occurrence := range b.snapshot.Occurrences {
		if occurrence.OccurrenceDate > b.bounds.Through {
			continue
		}
		switch {
		case occurrence.Status == "blocked":
			b.diagnose(ForecastDiagnostic{Code: "blocked_occurrence", TemplateID: occurrence.TemplateID, OccurrenceID: occurrence.ID, OccurrenceDate: occurrence.OccurrenceDate}, true)
		case occurrence.Status == "generated" && !draftOccurrence[occurrence.ID] && (!occurrence.TransactionStatus.Valid || occurrence.TransactionStatus.String == "draft") && !occurrence.TransactionDeleted:
			b.diagnose(ForecastDiagnostic{Code: "broken_occurrence_link", TemplateID: occurrence.TemplateID, OccurrenceID: occurrence.ID, OccurrenceDate: occurrence.OccurrenceDate}, true)
		}
	}
}

func (b *forecastBuild) addTemplates(acted map[string]db.ForecastOccurrenceRecord) error {
	postings := map[int64][]forecastPosting{}
	for _, row := range b.snapshot.TemplatePostings {
		postings[row.TemplateID] = append(postings[row.TemplateID], forecastPosting{row.AccountID, row.CommodityID, row.QuantityValue, row.QuantityScale})
	}
	for _, template := range b.snapshot.Templates {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if !template.Enabled || template.ArchivedAt.Valid {
			continue
		}
		spec := forecastSchedule(template)
		from := max(template.StartsOn, template.GenerateFrom)
		if from > b.bounds.Through {
			continue
		}
		cursor, err := time.Parse(time.DateOnly, from)
		if err != nil {
			return fmt.Errorf("parse forecast schedule start: %w", err)
		}
		through, _ := time.Parse(time.DateOnly, b.bounds.Through)
		for !cursor.After(through) {
			windowEnd := cursor.AddDate(0, 0, recur.MaxOccurrencesPerWindow-1)
			if windowEnd.After(through) {
				windowEnd = through
			}
			dates, err := recur.Occurrences(spec, cursor.Format(time.DateOnly), windowEnd.Format(time.DateOnly))
			if err != nil {
				return fmt.Errorf("enumerate forecast schedule: %w", err)
			}
			for _, date := range dates {
				if err := b.countCandidate(); err != nil {
					return err
				}
				if _, exists := acted[forecastOccurrenceKey(template.ID, date)]; exists {
					continue
				}
				projectionDate := max(date, b.bounds.First)
				rows := postings[template.ID]
				if (template.TransactionKind != "ordinary" && template.TransactionKind != "transfer") || !validateForecastPostings(rows, date, projectionDate, b.rules) {
					b.diagnose(ForecastDiagnostic{Code: "invalid_template", TemplateID: template.ID, OccurrenceDate: date, SourceDate: date}, true)
					continue
				}
				event := ForecastEvent{Key: fmt.Sprintf("template:%d:%s", template.ID, date), Source: "template", SourceID: template.ID, TemplateID: template.ID, OccurrenceDate: date, SourceDate: date, ProjectedDate: projectionDate, CarriedForward: date < b.bounds.First, PayeeName: template.PayeeName.String, Amounts: []ForecastEventAmount{}}
				if template.PayeeID.Valid && b.snapshot.PayeeNames[template.PayeeID.Int64] != "" {
					event.PayeeName = b.snapshot.PayeeNames[template.PayeeID.Int64]
				}
				for _, row := range rows {
					if !b.selected[row.AccountID] {
						continue
					}
					pair := forecastPair{row.AccountID, row.CommodityID}
					b.noteScale(pair, row.Scale)
					b.movement(pair, projectionDate).Template.AddCoefficient(row.Value, row.Scale)
					event.Amounts = append(event.Amounts, ForecastEventAmount{AccountID: row.AccountID, CommodityID: row.CommodityID, Quantity: ForecastQuantity{Value: row.Value, Scale: row.Scale}})
				}
				if len(event.Amounts) > 0 {
					b.events = append(b.events, event)
					if event.CarriedForward {
						b.diagnose(ForecastDiagnostic{Code: "carried_forward", TemplateID: event.TemplateID, OccurrenceDate: date, SourceDate: date}, false)
					}
				}
			}
			cursor = windowEnd.AddDate(0, 0, 1)
		}
	}
	return nil
}

func forecastSchedule(template db.ForecastTemplateRecord) recur.ScheduleSpec {
	result := recur.ScheduleSpec{Frequency: recur.Frequency(template.Frequency), IntervalCount: template.IntervalCount, LastDayOfMonth: template.LastDayOfMonth, StartsOn: template.StartsOn, EndsOn: template.EndsOn.String}
	if template.ByWeekday.Valid {
		value := time.Weekday(template.ByWeekday.Int64)
		result.ByWeekday = &value
	}
	if template.DayOfMonth.Valid {
		result.DayOfMonth = int(template.DayOfMonth.Int64)
	}
	if template.MonthOfYear.Valid {
		result.MonthOfYear = int(template.MonthOfYear.Int64)
	}
	if template.MaxOccurrences.Valid {
		result.MaxOccurrences = int(template.MaxOccurrences.Int64)
	}
	return result
}

func (b *forecastBuild) finish(input forecastNormalizedInput, computedAt string) (ForecastResult, error) {
	pairs := make([]forecastPair, 0, len(b.scales))
	for pair := range b.scales {
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].AccountID != pairs[j].AccountID {
			return pairs[i].AccountID < pairs[j].AccountID
		}
		return pairs[i].CommodityID < pairs[j].CommodityID
	})
	commoditySet := map[int64]bool{}
	for _, pair := range pairs {
		commoditySet[pair.CommodityID] = true
	}
	if (len(pairs)+len(commoditySet))*input.HorizonDays > forecastMaxOutputPoints {
		return ForecastResult{}, ErrForecastTooLarge
	}
	result := ForecastResult{AsOfDate: b.bounds.AsOf, FirstDate: b.bounds.First, ThroughDate: b.bounds.Through, ComputedAt: computedAt, AccountIDs: append([]int64(nil), b.scope.AccountIDs...), Accounts: []ForecastAccount{}, Commodities: []ForecastCommodity{}, Series: []ForecastSeries{}, Aggregates: []ForecastSeries{}, Events: b.events}
	for _, id := range b.scope.AccountIDs {
		a := b.scope.Accounts[id]
		result.Accounts = append(result.Accounts, ForecastAccount{ID: id, Name: a.Name.String, AccountClass: a.AccountClass, AccountKind: a.AccountKind, Status: a.Status, ParentAccountID: pointerFromNull(a.ParentAccountID), DefaultCommodityID: pointerFromNull(a.DefaultCommodityID), AllowsPostings: a.AllowsPostings})
	}
	commodityRules := commodityRulesAt(b.snapshot.CommodityVersions, b.bounds.AsOf)
	commodityIDs := make([]int64, 0, len(commoditySet))
	for id := range commoditySet {
		commodityIDs = append(commodityIDs, id)
	}
	sort.Slice(commodityIDs, func(i, j int) bool { return commodityIDs[i] < commodityIDs[j] })
	for _, id := range commodityIDs {
		c := commodityRules[id]
		result.Commodities = append(result.Commodities, ForecastCommodity{ID: id, Code: c.Code, StandardScale: c.StandardScale})
	}
	for _, pair := range pairs {
		series, err := b.makeSeries(pair, b.scales[pair])
		if err != nil {
			return ForecastResult{}, err
		}
		result.Series = append(result.Series, series)
	}
	for _, commodityID := range commodityIDs {
		aggregate, err := b.makeAggregate(commodityID, pairs)
		if err != nil {
			return ForecastResult{}, err
		}
		result.Aggregates = append(result.Aggregates, aggregate)
	}
	sortForecastEvents(result.Events)
	for _, event := range result.Events {
		switch event.Source {
		case "posted":
			result.SourceCounts.Posted++
		case "draft":
			result.SourceCounts.Draft++
		case "template":
			result.SourceCounts.Template++
		}
	}
	sort.Slice(b.diagnostics, func(i, j int) bool {
		a, c := b.diagnostics[i], b.diagnostics[j]
		if a.OccurrenceDate != c.OccurrenceDate {
			return a.OccurrenceDate < c.OccurrenceDate
		}
		if a.Code != c.Code {
			return a.Code < c.Code
		}
		if a.TemplateID != c.TemplateID {
			return a.TemplateID < c.TemplateID
		}
		return a.OccurrenceID < c.OccurrenceID
	})
	total := len(b.diagnostics)
	carried, excluded := 0, 0
	for _, diagnostic := range b.diagnostics {
		if diagnostic.Code == "carried_forward" {
			carried++
		} else {
			excluded++
		}
	}
	shown := b.diagnostics
	if len(shown) > forecastMaxDiagnostics {
		shown = shown[:forecastMaxDiagnostics]
	}
	result.Assumptions = ForecastAssumptions{Complete: !b.incomplete, CarriedForward: carried, Excluded: excluded, Total: total, Hidden: total - len(shown), Diagnostics: append([]ForecastDiagnostic(nil), shown...)}
	return result, nil
}

func (b *forecastBuild) makeSeries(pair forecastPair, scale int) (ForecastSeries, error) {
	if err := checkForecastValue(b.opening[pair], pair.CommodityID); err != nil {
		return ForecastSeries{}, err
	}
	opening := b.valueAtScale(b.opening[pair], scale)
	series := ForecastSeries{AccountID: pair.AccountID, CommodityID: pair.CommodityID, Opening: opening, Points: []ForecastPoint{}, Minimum: opening, MinimumDate: b.bounds.AsOf}
	recorded := exact.ScaledIntFromCoefficient(opening.Value, scale)
	projected := exact.ScaledIntFromCoefficient(opening.Value, scale)
	if b.scope.Accounts[pair.AccountID].AccountClass == "asset" && isCashKind(b.scope.Accounts[pair.AccountID].AccountKind) && projected.Sign() < 0 {
		series.FirstNegativeDate = b.bounds.AsOf
	}
	for date := b.bounds.First; date <= b.bounds.Through; date = forecastNextDate(date) {
		movement := b.movement(pair, date)
		recorded.AddScaled(movement.Posted)
		projected.AddScaled(movement.Posted)
		projected.AddScaled(movement.Draft)
		projected.AddScaled(movement.Template)
		for _, value := range []*exact.ScaledInt{movement.Posted, movement.Draft, movement.Template, recorded, projected} {
			if err := checkForecastValue(value, pair.CommodityID); err != nil {
				return ForecastSeries{}, err
			}
		}
		point := ForecastPoint{Date: date, Components: ForecastComponents{Posted: b.valueAtScale(movement.Posted, scale), Draft: b.valueAtScale(movement.Draft, scale), Template: b.valueAtScale(movement.Template, scale)}, RecordedBalance: b.valueAtScale(recorded, scale), ProjectedBalance: b.valueAtScale(projected, scale)}
		series.Points = append(series.Points, point)
		if projected.Cmp(exact.ScaledIntFromCoefficient(series.Minimum.Value, series.Minimum.Scale)) < 0 {
			series.Minimum = point.ProjectedBalance
			series.MinimumDate = date
		}
		if series.FirstNegativeDate == "" && b.scope.Accounts[pair.AccountID].AccountClass == "asset" && isCashKind(b.scope.Accounts[pair.AccountID].AccountKind) && projected.Sign() < 0 {
			series.FirstNegativeDate = date
		}
	}
	return series, nil
}

func (b *forecastBuild) makeAggregate(commodityID int64, pairs []forecastPair) (ForecastSeries, error) {
	scale := 0
	for _, pair := range pairs {
		if pair.CommodityID == commodityID && b.scales[pair] > scale {
			scale = b.scales[pair]
		}
	}
	opening := exact.NewScaledInt()
	for _, pair := range pairs {
		if pair.CommodityID == commodityID {
			opening.AddScaled(b.opening[pair])
		}
	}
	if err := checkForecastValue(opening, commodityID); err != nil {
		return ForecastSeries{}, err
	}
	series := ForecastSeries{CommodityID: commodityID, Opening: b.valueAtScale(opening, scale), Points: []ForecastPoint{}, Minimum: b.valueAtScale(opening, scale), MinimumDate: b.bounds.AsOf}
	recorded := exact.ScaledIntFromCoefficient(series.Opening.Value, scale)
	projected := exact.ScaledIntFromCoefficient(series.Opening.Value, scale)
	for date := b.bounds.First; date <= b.bounds.Through; date = forecastNextDate(date) {
		movement := forecastMovement{Posted: exact.NewScaledInt(), Draft: exact.NewScaledInt(), Template: exact.NewScaledInt()}
		for _, pair := range pairs {
			if pair.CommodityID == commodityID {
				m := b.movement(pair, date)
				movement.Posted.AddScaled(m.Posted)
				movement.Draft.AddScaled(m.Draft)
				movement.Template.AddScaled(m.Template)
			}
		}
		recorded.AddScaled(movement.Posted)
		projected.AddScaled(movement.Posted)
		projected.AddScaled(movement.Draft)
		projected.AddScaled(movement.Template)
		for _, value := range []*exact.ScaledInt{movement.Posted, movement.Draft, movement.Template, recorded, projected} {
			if err := checkForecastValue(value, commodityID); err != nil {
				return ForecastSeries{}, err
			}
		}
		point := ForecastPoint{Date: date, Components: ForecastComponents{Posted: b.valueAtScale(movement.Posted, scale), Draft: b.valueAtScale(movement.Draft, scale), Template: b.valueAtScale(movement.Template, scale)}, RecordedBalance: b.valueAtScale(recorded, scale), ProjectedBalance: b.valueAtScale(projected, scale)}
		series.Points = append(series.Points, point)
		if projected.Cmp(exact.ScaledIntFromCoefficient(series.Minimum.Value, series.Minimum.Scale)) < 0 {
			series.Minimum = point.ProjectedBalance
			series.MinimumDate = date
		}
	}
	return series, nil
}

func (b *forecastBuild) accumulator(values map[forecastPair]*exact.ScaledInt, pair forecastPair) *exact.ScaledInt {
	if values[pair] == nil {
		values[pair] = exact.NewScaledInt()
	}
	return values[pair]
}
func (b *forecastBuild) movement(pair forecastPair, date string) *forecastMovement {
	if b.movements[pair] == nil {
		b.movements[pair] = map[string]*forecastMovement{}
	}
	if b.movements[pair][date] == nil {
		b.movements[pair][date] = &forecastMovement{exact.NewScaledInt(), exact.NewScaledInt(), exact.NewScaledInt()}
	}
	return b.movements[pair][date]
}
func (b *forecastBuild) noteScale(pair forecastPair, scale int) {
	if scale > b.scales[pair] {
		b.scales[pair] = scale
	} else if _, ok := b.scales[pair]; !ok {
		b.scales[pair] = scale
	}
}
func (b *forecastBuild) valueAtScale(value *exact.ScaledInt, scale int) ForecastQuantity {
	if value == nil {
		value = exact.NewScaledInt()
	}
	converted := value.TruncatedTo(scale)
	return ForecastQuantity{exact.Coefficient(converted.BigInt().String()), scale}
}
func checkForecastValue(value *exact.ScaledInt, commodityID int64) error {
	if value == nil {
		return nil
	}
	if _, err := value.Coefficient(); err != nil {
		return LedgerOverflowError{CommodityID: commodityID}
	}
	return nil
}

func (b *forecastBuild) countCandidate() error {
	b.candidates++
	if b.candidates > forecastMaxCandidates {
		return ErrForecastTooLarge
	}
	return b.ctx.Err()
}
func (b *forecastBuild) diagnose(d ForecastDiagnostic, incomplete bool) {
	b.diagnostics = append(b.diagnostics, d)
	if incomplete {
		b.incomplete = true
	}
}
func forecastOccurrenceKey(templateID int64, date string) string {
	return fmt.Sprintf("%d:%s", templateID, date)
}
func forecastNextDate(date string) string {
	parsed, _ := time.Parse(time.DateOnly, date)
	return parsed.AddDate(0, 0, 1).Format(time.DateOnly)
}
func isCashKind(kind string) bool {
	return kind == "cash" || kind == "checking" || kind == "savings" || kind == "brokerage_cash"
}
func sortForecastEvents(events []ForecastEvent) {
	order := map[string]int{"posted": 0, "draft": 1, "template": 2}
	for index := range events {
		sort.Slice(events[index].Amounts, func(i, j int) bool {
			if events[index].Amounts[i].AccountID != events[index].Amounts[j].AccountID {
				return events[index].Amounts[i].AccountID < events[index].Amounts[j].AccountID
			}
			return events[index].Amounts[i].CommodityID < events[index].Amounts[j].CommodityID
		})
	}
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.ProjectedDate != b.ProjectedDate {
			return a.ProjectedDate < b.ProjectedDate
		}
		if order[a.Source] != order[b.Source] {
			return order[a.Source] < order[b.Source]
		}
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.EntryID != b.EntryID {
			return a.EntryID < b.EntryID
		}
		return a.OccurrenceDate < b.OccurrenceDate
	})
}
