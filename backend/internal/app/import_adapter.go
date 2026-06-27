package app

import "context"

// SourceAdapter is the interface every format adapter implements.
// The only format-specific code is in the adapter; everything downstream
// of StagedRow is shared pipeline.
type SourceAdapter interface {
	// Kind returns the format/source identifier (e.g. "qif", "csv", "ofx").
	Kind() string
	// Detect reports the adapter's confidence that it can handle this input
	// (sniff extension / magic bytes / headers) so the UI can auto-suggest.
	Detect(input RawInput) Confidence
	// Parse turns raw input + an optional provider profile into staged rows.
	// It MUST NOT touch the ledger; it only emits canonical rows + warnings.
	Parse(ctx context.Context, input RawInput, profile *ImportProfile) (ParseResult, error)
}

// adapterRegistry holds all registered SourceAdapters by kind.
type adapterRegistry struct {
	adapters []SourceAdapter
	byKind   map[string]SourceAdapter
}

func newAdapterRegistry(adapters ...SourceAdapter) *adapterRegistry {
	r := &adapterRegistry{
		adapters: adapters,
		byKind:   make(map[string]SourceAdapter, len(adapters)),
	}
	for _, a := range adapters {
		r.byKind[a.Kind()] = a
	}
	return r
}

func (r *adapterRegistry) detect(input RawInput) SourceAdapter {
	best := SourceAdapter(nil)
	bestConf := ConfidenceNone
	for _, a := range r.adapters {
		c := a.Detect(input)
		if c > bestConf {
			bestConf = c
			best = a
		}
	}
	return best
}

func (r *adapterRegistry) byKindName(kind string) (SourceAdapter, bool) {
	a, ok := r.byKind[kind]
	return a, ok
}
