package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"rekenraam/backend/internal/db"
)

// The basis token is one hash over named parts, each hashed on its own, so a
// rejected detail request can say which part moved. A forecast basis changed
// once in CI with no request writing anything (#515), and one opaque hash
// could not tell an account version from a recurring occurrence.

// forecastBasisPart is one hashed input of a forecast: its digest and, for a
// list, how many rows it held.
type forecastBasisPart struct {
	Name   string
	Digest [sha256.Size]byte
	Rows   int
}

// ForecastBasisChange names a part whose digest differs between the basis a
// client was given and the one computed now.
type ForecastBasisChange struct {
	Part       string
	RowsBefore int
	RowsAfter  int
}

// ForecastBasisChangedError is ErrForecastBasisChanged with the evidence:
// Known is false when this process no longer remembers the requested token
// (another process issued it, or it aged out), and Changed is then empty.
type ForecastBasisChangedError struct {
	Requested string
	Current   string
	Known     bool
	Changed   []ForecastBasisChange
}

func (e *ForecastBasisChangedError) Error() string { return ErrForecastBasisChanged.Error() }

func (e *ForecastBasisChangedError) Is(target error) bool { return target == ErrForecastBasisChanged }

func forecastBasisParts(input forecastNormalizedInput, bounds forecastBounds, scope []int64, snapshot db.ForecastSnapshot, learned *ForecastLearnedSpending) ([]forecastBasisPart, error) {
	learnedRows := 0
	if learned != nil {
		learnedRows = len(learned.Events)
	}
	values := []struct {
		name  string
		value any
		rows  int
	}{
		{"input", input, 0},
		{"bounds", bounds, 0},
		{"scope", scope, len(scope)},
		{"time_zone", snapshot.TimeZone, 0},
		{"account_versions", snapshot.AccountVersions, len(snapshot.AccountVersions)},
		{"commodity_versions", snapshot.CommodityVersions, len(snapshot.CommodityVersions)},
		{"posted_postings", snapshot.PostedPostings, len(snapshot.PostedPostings)},
		{"templates", snapshot.Templates, len(snapshot.Templates)},
		{"template_postings", snapshot.TemplatePostings, len(snapshot.TemplatePostings)},
		{"occurrences", snapshot.Occurrences, len(snapshot.Occurrences)},
		{"draft_postings", snapshot.DraftPostings, len(snapshot.DraftPostings)},
		{"payee_names", snapshot.PayeeNames, len(snapshot.PayeeNames)},
		{"rates", snapshot.Rates, len(snapshot.Rates)},
		{"learning_postings", snapshot.LearningPostings, len(snapshot.LearningPostings)},
		{"learned_spending", learned, learnedRows},
	}
	parts := make([]forecastBasisPart, 0, len(values))
	for _, item := range values {
		encoded, err := json.Marshal(item.value)
		if err != nil {
			return nil, fmt.Errorf("encode forecast basis %s: %w", item.name, err)
		}
		parts = append(parts, forecastBasisPart{Name: item.name, Digest: sha256.Sum256(encoded), Rows: item.rows})
	}
	return parts, nil
}

// forecastBasisToken hashes the parts in order, names included, so moving a
// value from one part to another still changes the token.
func forecastBasisToken(parts []forecastBasisPart) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(part.Name))
		hash.Write([]byte{0})
		hash.Write(part.Digest[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// forecastBasisMemory keeps the parts of the most recent tokens this process
// issued, only to explain a later mismatch. It never decides one.
type forecastBasisMemory struct {
	mu     sync.Mutex
	order  []string
	tokens map[string][]forecastBasisPart
}

const forecastBasisMemorySize = 256

func newForecastBasisMemory() *forecastBasisMemory {
	return &forecastBasisMemory{tokens: map[string][]forecastBasisPart{}}
}

func (m *forecastBasisMemory) remember(token string, parts []forecastBasisPart) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tokens[token]; ok {
		return
	}
	if len(m.order) == forecastBasisMemorySize {
		delete(m.tokens, m.order[0])
		m.order = m.order[1:]
	}
	m.order = append(m.order, token)
	m.tokens[token] = parts
}

func (m *forecastBasisMemory) explain(requested, current string, now []forecastBasisPart) *ForecastBasisChangedError {
	result := &ForecastBasisChangedError{Requested: requested, Current: current}
	m.mu.Lock()
	before, ok := m.tokens[requested]
	m.mu.Unlock()
	if !ok {
		return result
	}
	result.Known = true
	previous := make(map[string]forecastBasisPart, len(before))
	for _, part := range before {
		previous[part.Name] = part
	}
	for _, part := range now {
		old := previous[part.Name]
		if old.Digest != part.Digest {
			result.Changed = append(result.Changed, ForecastBasisChange{Part: part.Name, RowsBefore: old.Rows, RowsAfter: part.Rows})
		}
	}
	return result
}
