package app

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// ImportService orchestrates the import pipeline:
// parse → normalize → dedupe → stage (preview) → commit
type ImportService struct {
	settings           *SettingsService
	repository         *db.ImportRepository
	transactionService *TransactionService
	accountRepository  *db.AccountRepository
	registry           *adapterRegistry
	now                func() time.Time

	// Online-import dependencies (Slice 3). Both are nil-safe: callers that
	// only exercise the file-upload pipeline (e.g. unit tests) may omit them,
	// in which case StartOnlineImport/RefreshImportConnection/the fetch
	// worker simply report online import as unconfigured.
	connectionService *ImportConnectionService
	backgroundWork    *db.BackgroundWorkRepository
	httpClient        *http.Client
	trading212BaseURL string
	// fetchWake lets a newly enqueued fetch run now instead of on the
	// worker's next one-minute tick. Buffered 1: a pending wake already
	// covers every fetch enqueued before the worker reads it.
	fetchWake chan struct{}

	// investmentService routes resolved Trading 212 order-fill/dividend rows
	// to real Buy/Sell/Dividend calls at commit time (B-T212-INVST, Slice
	// 4b). Nil-safe: without it, these rows just commit as plain cash
	// transactions via the generic path, same as before Slice 4b.
	investmentService *InvestmentService

	// beforeImportRowCommitForTest creates a deterministic race window after
	// an idempotency read and before the row's write transaction. Production
	// callers leave it nil.
	beforeImportRowCommitForTest func()

	// beforeTrading212HoldingCreateForTest creates a deterministic race window
	// after a missing holding-map read and before account creation. Production
	// callers leave it nil.
	beforeTrading212HoldingCreateForTest func()

	// beforeImportCommitIdentityCheckForTest creates a deterministic stale-row
	// window after staged rows are listed and before their identity lookup.
	// Production callers leave it nil.
	beforeImportCommitIdentityCheckForTest func()
}

func NewImportService(
	repository *db.ImportRepository,
	transactionService *TransactionService,
	accountRepository *db.AccountRepository,
	connectionService *ImportConnectionService,
	backgroundWork *db.BackgroundWorkRepository,
	investmentService *InvestmentService,
) *ImportService {
	return &ImportService{
		repository:         repository,
		transactionService: transactionService,
		accountRepository:  accountRepository,
		registry:           newAdapterRegistry(&QIFAdapter{}, &CSVAdapter{}, &Trading212Adapter{}, &CryptoCSVAdapter{}),
		now:                time.Now,
		connectionService:  connectionService,
		backgroundWork:     backgroundWork,
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		fetchWake:          make(chan struct{}, 1),
		investmentService:  investmentService,
	}
}

// SetTrading212BaseURL points online fetches at a provider stub. The runtime
// calls it only with the development-only TRADING212_BASE_URL; empty keeps
// the live API.
func (s *ImportService) SetTrading212BaseURL(baseURL string) {
	s.trading212BaseURL = baseURL
}

// SetNowForTest overrides the clock used for batch/work timestamps and lease
// expiry, so tests can deterministically simulate lease expiry/restart.
func (s *ImportService) SetNowForTest(now func() time.Time) {
	s.now = now
}

// StartImport runs detect → parse → stage and returns the preview.
func (s *ImportService) StartImport(ctx context.Context, input StartImportInput) (StartImportResult, error) {
	if input.OwnerUserID <= 0 {
		return StartImportResult{}, ValidationError{Message: "owner user is required"}
	}

	adapter := s.registry.detect(input.Input)
	if input.Format == "crypto_csv" {
		adapter = &CryptoCSVAdapter{}
	} else if input.Format != "" {
		return StartImportResult{}, ValidationError{Message: "unsupported import format"}
	}
	if adapter == nil {
		return StartImportResult{}, ValidationError{Message: "no adapter can handle this file format"}
	}

	var profile *ImportProfile
	if input.ProfileID != nil {
		record, err := s.repository.ImportProfileByID(ctx, BookID, *input.ProfileID)
		if err != nil {
			if errors.Is(err, db.ErrImportProfileNotFound) {
				return StartImportResult{}, ErrImportProfileNotFound
			}
			return StartImportResult{}, fmt.Errorf("load import profile: %w", err)
		}
		profileValue := toImportProfile(record)
		profile = &profileValue
		if profile.AdapterKind != adapter.Kind() {
			return StartImportResult{}, ValidationError{Message: "import profile does not match the uploaded file format"}
		}
	}

	parseResult, err := adapter.Parse(ctx, input.Input, profile)
	if err != nil {
		return StartImportResult{}, fmt.Errorf("parse import: %w", err)
	}

	if err := s.prepareCSVInvestmentRows(ctx, &parseResult); err != nil {
		return StartImportResult{}, err
	}
	if adapter.Kind() == "crypto_csv" {
		if err := s.prepareCryptoCSV(ctx, input, &parseResult); err != nil {
			return StartImportResult{}, err
		}
	}
	nowStr := s.now().UTC().Format(time.RFC3339)

	var profileID sql.NullInt64
	if input.ProfileID != nil {
		profileID = sql.NullInt64{Int64: *input.ProfileID, Valid: true}
	}

	// The batch keeps its parse warnings, so a reloaded review screen shows
	// them as the upload response did (#300).
	meta := parseResult.Meta
	meta.Warnings = parseResult.Warnings
	metaJSON, _ := json.Marshal(meta)

	batch, err := s.repository.CreateImportBatch(ctx, db.CreateImportBatchParams{
		BookID:           BookID,
		SourceKind:       adapter.Kind(),
		ProfileID:        profileID,
		OriginalFilename: input.Input.Filename,
		SourceMetaJSON:   string(metaJSON),
		CreatedAt:        nowStr,
		ActorUserID:      input.OwnerUserID,
		AuthSessionID:    input.AuthSessionID,
		RequestID:        input.RequestID,
	})
	if err != nil {
		return StartImportResult{}, fmt.Errorf("create import batch: %w", err)
	}

	stagedRows, _, err := s.stageParseResult(ctx, batch.ID, parseResult)
	if err != nil {
		return StartImportResult{}, err
	}

	return StartImportResult{
		Batch:    toImportBatch(batch),
		Rows:     toImportStagedRows(stagedRows),
		Warnings: parseResult.Warnings,
		Meta:     parseResult.Meta,
	}, nil
}

// AnalyzeCSV decodes and inspects a CSV before a mapping profile exists.
// It does not persist a batch or any financial data.
func (s *ImportService) AnalyzeCSV(_ context.Context, input RawInput, delimiter string) (CSVAnalysis, error) {
	if (&CSVAdapter{}).Detect(input) == ConfidenceNone {
		return CSVAnalysis{}, ValidationError{Message: "csv analysis requires a CSV file"}
	}
	return AnalyzeCSVInput(input, delimiter)
}

func (s *ImportService) ListImportProfiles(ctx context.Context) ([]ImportProfile, error) {
	records, err := s.repository.ListImportProfiles(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list import profiles: %w", err)
	}
	profiles := make([]ImportProfile, 0, len(records))
	for _, record := range records {
		profiles = append(profiles, toImportProfile(record))
	}
	return profiles, nil
}

func (s *ImportService) CreateImportProfile(ctx context.Context, input CreateImportProfileInput) (ImportProfile, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.AdapterKind = strings.TrimSpace(input.AdapterKind)
	if input.OwnerUserID <= 0 || input.Name == "" {
		return ImportProfile{}, ValidationError{Message: "profile name is required"}
	}
	adapter, ok := s.registry.byKindName(input.AdapterKind)
	if !ok || adapter.Kind() != "csv" {
		return ImportProfile{}, ValidationError{Message: "only csv import profiles can be created"}
	}
	if err := validateCSVProfileConfig(input.ConfigJSON); err != nil {
		return ImportProfile{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.CreateImportProfile(ctx, db.CreateImportProfileParams{
		BookID: BookID, Name: input.Name, AdapterKind: input.AdapterKind,
		ConfigJSON: input.ConfigJSON, CreatedAt: now,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
	})
	if err != nil {
		return ImportProfile{}, fmt.Errorf("create import profile: %w", err)
	}
	return toImportProfile(record), nil
}

func (s *ImportService) UpdateImportProfile(ctx context.Context, input UpdateImportProfileInput) (ImportProfile, error) {
	if input.OwnerUserID <= 0 || input.ProfileID <= 0 || (input.Name == nil && input.ConfigJSON == nil) {
		return ImportProfile{}, ValidationError{Message: "profile update must include a name or config"}
	}
	existing, err := s.repository.ImportProfileByID(ctx, BookID, input.ProfileID)
	if errors.Is(err, db.ErrImportProfileNotFound) {
		return ImportProfile{}, ErrImportProfileNotFound
	}
	if err != nil {
		return ImportProfile{}, fmt.Errorf("load import profile for update: %w", err)
	}
	var name *string
	if input.Name != nil {
		trimmedName := strings.TrimSpace(*input.Name)
		if trimmedName == "" {
			return ImportProfile{}, ValidationError{Message: "profile name is required"}
		}
		name = &trimmedName
	}
	var configJSON *string
	if input.ConfigJSON != nil {
		if existing.AdapterKind != "csv" {
			return ImportProfile{}, ValidationError{Message: "only csv import profiles can be updated"}
		}
		if err := validateCSVProfileConfig(*input.ConfigJSON); err != nil {
			return ImportProfile{}, err
		}
		configJSON = input.ConfigJSON
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.UpdateImportProfile(ctx, db.UpdateImportProfileParams{
		BookID: BookID, ProfileID: input.ProfileID, Name: name, ConfigJSON: configJSON, UpdatedAt: now,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
	})
	if errors.Is(err, db.ErrImportProfileNotFound) {
		return ImportProfile{}, ErrImportProfileNotFound
	}
	if err != nil {
		return ImportProfile{}, fmt.Errorf("update import profile: %w", err)
	}
	return toImportProfile(record), nil
}

func (s *ImportService) DeleteImportProfile(ctx context.Context, input DeleteImportProfileInput) error {
	if input.OwnerUserID <= 0 || input.ProfileID <= 0 {
		return ValidationError{Message: "profile_id is invalid"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	err := s.repository.DeleteImportProfile(ctx, db.DeleteImportProfileParams{
		BookID: BookID, ProfileID: input.ProfileID, DeletedAt: now,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
	})
	if errors.Is(err, db.ErrImportProfileNotFound) {
		return ErrImportProfileNotFound
	}
	if err != nil {
		return fmt.Errorf("delete import profile: %w", err)
	}
	return nil
}

func toImportProfile(record db.ImportProfileRecord) ImportProfile {
	return ImportProfile{ID: record.ID, BookID: record.BookID, Name: record.Name, AdapterKind: record.AdapterKind, ConfigJSON: record.ConfigJSON, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

const (
	importRuleNameMaxBytes     = 200
	importRuleContainsMaxBytes = 500
	importRulePriorityMax      = 1000000
)

func (s *ImportService) ListImportRules(ctx context.Context) ([]ImportRule, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return nil, referenceErr
	}

	records, err := s.repository.ListImportRules(ctx, BookID)
	if err != nil {
		return nil, fmt.Errorf("list import rules: %w", err)
	}
	return toImportRules(records), nil
}

func (s *ImportService) CreateImportRule(ctx context.Context, input CreateImportRuleInput) (ImportRule, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return ImportRule{}, referenceErr
	}

	if input.OwnerUserID <= 0 {
		return ImportRule{}, ValidationError{Message: "owner user is required"}
	}
	spec, err := s.cleanImportRuleSpec(ctx, db.ImportRuleSpec{
		Name: input.Name, Priority: input.Priority, Enabled: input.Enabled,
		MatchField: input.MatchField, ContainsText: input.ContainsText,
		CategoryID: nullableInt64(input.CategoryID), PayeeID: nullableInt64(input.PayeeID), TagIDs: input.TagIDs,
	})
	if err != nil {
		return ImportRule{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.CreateImportRule(ctx, db.CreateImportRuleParams{
		BookID: BookID, Spec: spec, CreatedAt: now,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
	})
	if err != nil {
		return ImportRule{}, fmt.Errorf("create import rule: %w", err)
	}
	return toImportRule(record), nil
}

func (s *ImportService) UpdateImportRule(ctx context.Context, input UpdateImportRuleInput) (ImportRule, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return ImportRule{}, referenceErr
	}

	if input.OwnerUserID <= 0 || input.RuleID <= 0 {
		return ImportRule{}, ValidationError{Message: "import rule id is invalid"}
	}
	existing, err := s.repository.ImportRuleByID(ctx, BookID, input.RuleID)
	if errors.Is(err, db.ErrImportRuleNotFound) {
		return ImportRule{}, ErrImportRuleNotFound
	}
	if err != nil {
		return ImportRule{}, fmt.Errorf("read import rule for update: %w", err)
	}
	spec := db.ImportRuleSpec{
		Name: existing.Name, Priority: existing.Priority, Enabled: existing.Enabled,
		MatchField: existing.MatchField, ContainsText: existing.ContainsText,
		CategoryID: existing.CategoryID, PayeeID: existing.PayeeID, TagIDs: existing.TagIDs,
	}
	if input.Name != nil {
		spec.Name = *input.Name
	}
	if input.Priority != nil {
		spec.Priority = *input.Priority
	}
	if input.Enabled != nil {
		spec.Enabled = *input.Enabled
	}
	if input.MatchField != nil {
		spec.MatchField = *input.MatchField
	}
	if input.ContainsText != nil {
		spec.ContainsText = *input.ContainsText
	}
	if input.ClearCategory {
		spec.CategoryID = sql.NullInt64{}
	} else if input.CategoryID != nil {
		spec.CategoryID = nullableInt64(input.CategoryID)
	}
	if input.ClearPayee {
		spec.PayeeID = sql.NullInt64{}
	} else if input.PayeeID != nil {
		spec.PayeeID = nullableInt64(input.PayeeID)
	}
	if input.TagIDs != nil {
		spec.TagIDs = *input.TagIDs
	}
	spec, err = s.cleanImportRuleSpec(ctx, spec)
	if err != nil {
		return ImportRule{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	record, err := s.repository.UpdateImportRule(ctx, db.UpdateImportRuleParams{
		BookID: BookID, RuleID: input.RuleID, Spec: spec, UpdatedAt: now,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
	})
	if errors.Is(err, db.ErrImportRuleNotFound) {
		return ImportRule{}, ErrImportRuleNotFound
	}
	if err != nil {
		return ImportRule{}, fmt.Errorf("update import rule: %w", err)
	}
	return toImportRule(record), nil
}

func (s *ImportService) DeleteImportRule(ctx context.Context, input DeleteImportRuleInput) error {
	if input.OwnerUserID <= 0 || input.RuleID <= 0 {
		return ValidationError{Message: "import rule id is invalid"}
	}
	now := s.now().UTC().Format(time.RFC3339)
	err := s.repository.DeleteImportRule(ctx, db.DeleteImportRuleParams{
		BookID: BookID, RuleID: input.RuleID, DeletedAt: now,
		ActorUserID: input.OwnerUserID, AuthSessionID: input.AuthSessionID, RequestID: input.RequestID,
	})
	if errors.Is(err, db.ErrImportRuleNotFound) {
		return ErrImportRuleNotFound
	}
	if err != nil {
		return fmt.Errorf("delete import rule: %w", err)
	}
	return nil
}

func (s *ImportService) cleanImportRuleSpec(ctx context.Context, spec db.ImportRuleSpec) (db.ImportRuleSpec, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return db.ImportRuleSpec{}, referenceErr
	}

	spec.Name = strings.TrimSpace(spec.Name)
	spec.MatchField = strings.TrimSpace(spec.MatchField)
	spec.ContainsText = strings.TrimSpace(spec.ContainsText)
	if spec.Name == "" || len(spec.Name) > importRuleNameMaxBytes {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule name is invalid"}
	}
	if spec.Priority < 0 || spec.Priority > importRulePriorityMax {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule priority is invalid"}
	}
	if spec.MatchField != "payee" && spec.MatchField != "description" {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule match field is invalid"}
	}
	if spec.ContainsText == "" || len(spec.ContainsText) > importRuleContainsMaxBytes {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule contains text is invalid"}
	}
	slices.Sort(spec.TagIDs)
	spec.TagIDs = slices.Compact(spec.TagIDs)
	for _, tagID := range spec.TagIDs {
		if tagID <= 0 {
			return db.ImportRuleSpec{}, ValidationError{Message: "import rule tag is invalid"}
		}
	}
	if !spec.CategoryID.Valid && !spec.PayeeID.Valid && len(spec.TagIDs) == 0 {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule must set a category, payee, or tag"}
	}
	state, err := s.repository.ImportRuleTargetState(ctx, BookID, spec.CategoryID, spec.PayeeID, spec.TagIDs)
	if err != nil {
		return db.ImportRuleSpec{}, fmt.Errorf("validate import rule targets: %w", err)
	}
	if !state.CategoryActive {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule category is invalid"}
	}
	if !state.PayeeActive {
		return db.ImportRuleSpec{}, ValidationError{Message: "import rule payee is invalid"}
	}
	for _, tagID := range spec.TagIDs {
		if !state.ActiveTagIDs[tagID] {
			return db.ImportRuleSpec{}, ValidationError{Message: "import rule tag is invalid"}
		}
	}
	return spec, nil
}

func toImportRule(record db.ImportRuleRecord) ImportRule {
	return ImportRule{
		ID: record.ID, BookID: record.BookID, Name: record.Name, Priority: record.Priority,
		Enabled: record.Enabled, MatchField: record.MatchField, ContainsText: record.ContainsText,
		CategoryID: int64Ptr(record.CategoryID), PayeeID: int64Ptr(record.PayeeID), TagIDs: record.TagIDs,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func toImportRules(records []db.ImportRuleRecord) []ImportRule {
	rules := make([]ImportRule, 0, len(records))
	for _, record := range records {
		rules = append(rules, toImportRule(record))
	}
	return rules
}

// stageParseResult takes a SourceAdapter's ParseResult and runs the
// fingerprint-hash → dedupe → insert pipeline against an existing batch.
// Shared by StartImport (file path, always exactly one call per batch) and
// the Trading 212 fetch worker (Slice 3, online path — which, since adding
// pagination continuation, can call this more than once for the same batch
// as each chunk lands). The second return value is the row_index base this
// call assigned its rows at (== the count of rows already staged before this
// call), so a caller juggling multiple calls on one batch (continuation
// warnings) can translate a ParseResult-local RowIndex into a batch-global
// one.
func (s *ImportService) stageParseResult(ctx context.Context, batchID int64, parseResult ParseResult) ([]db.ImportStagedRowRecord, int, error) {
	ctx, referenceErr := s.settings.referenceContext(ctx, s.now())
	if referenceErr != nil {
		return nil, 0, referenceErr
	}

	existingRows, err := s.repository.ListAllImportStagedRows(ctx, batchID)
	if err != nil {
		return nil, 0, fmt.Errorf("list existing staged rows: %w", err)
	}
	baseIndex := len(existingRows)

	// seenInBatch starts seeded with fingerprints already staged from an
	// earlier call on this same batch. Without this, a continuation chunk
	// re-scanning the incremental cursor's overlap boundary (see the
	// Fetcher's same-timestamp handling) would insert that movement a
	// second time as a fresh "new" row instead of flagging it
	// "needs_attention" — this call has no other way to know about rows a
	// prior call already inserted.
	seenInBatch := make(map[string]bool, len(existingRows))
	for _, r := range existingRows {
		seenInBatch[r.DedupeFingerprint] = true
	}

	// Compute dedupe fingerprints and check against existing commit identities.
	for i := range parseResult.Rows {
		fp := hashFingerprint(parseResult.Rows[i].DedupeFingerprint)
		parseResult.Rows[i].DedupeFingerprint = fp
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, batchID)
	if err != nil {
		return nil, 0, fmt.Errorf("get import batch for staging: %w", err)
	}
	rules, err := s.repository.ListApplicableImportRules(ctx, BookID)
	if err != nil {
		return nil, 0, fmt.Errorf("list applicable import rules: %w", err)
	}

	// Check each row against existing commit identities (ledger-committed
	// duplicates) — one DB lookup per unique key in this call. An
	// account-scoped row whose account a rule already resolved checks that
	// account; otherwise it is flagged if it was committed to any account,
	// and review re-decides once the owner picks one (#302).
	committedKeys := make(map[string]bool)
	committedBefore := func(row StagedRow, accountID int64) (bool, error) {
		scoped := accountScopedImportIdentity(batch.SourceKind, row.Investment != nil)
		key := importCommitKey(row.DedupeFingerprint, scoped, accountID)
		if scoped && accountID == 0 {
			key = "source:" + row.DedupeFingerprint
		}
		if found, checked := committedKeys[key]; checked {
			return found, nil
		}
		var found bool
		var err error
		if scoped && accountID == 0 {
			found, err = s.repository.CommitIdentityExistsForSource(ctx, BookID, row.DedupeFingerprint)
		} else {
			_, found, err = s.repository.FindCommitIdentity(ctx, BookID, key)
		}
		if err != nil {
			return false, fmt.Errorf("check commit identity: %w", err)
		}
		committedKeys[key] = found
		return found, nil
	}

	// Insert staged rows.
	var dbRows []db.CreateImportStagedRowParams
	type committedFillSnapshot struct {
		rawJSON        string
		normalizedJSON string
		found          bool
	}
	fillSnapshots := make(map[string]committedFillSnapshot)
	for i, row := range parseResult.Rows {
		rawJSON, _ := json.Marshal(row.Raw)
		normalized := map[string]any{
			"date":           row.Date,
			"amount":         row.Amount,
			"commodity_hint": row.CommodityHint,
			"payee_hint":     row.PayeeHint,
			"category_hint":  row.CategoryHint,
			"account_hint":   row.AccountHint,
			"transfer_hint":  row.TransferHint,
			"memo":           row.Memo,
			"external_ref":   row.ExternalRef,
			"splits":         row.Splits,
		}
		if row.Investment != nil {
			normalized["investment"] = row.Investment
		}
		if len(row.ParseErrors) > 0 {
			normalized["parse_errors"] = row.ParseErrors
		}
		normalizedJSON, _ := json.Marshal(normalized)

		resolution := applyImportRules(row, rules)
		committed, err := committedBefore(row, resolution.AccountID)
		if err != nil {
			return nil, 0, err
		}
		dedupeStatus := "new"
		if committed {
			// Exists in commit_identities → duplicate of a previously committed row.
			dedupeStatus = "duplicate"
			if row.Investment == nil && row.Raw[rawKeyKind] == trading212RawKindOrderFill {
				snapshot, checked := fillSnapshots[row.DedupeFingerprint]
				if !checked {
					originalRaw, originalNormalized, found, err := s.repository.FindCommittedTrading212FillSnapshot(ctx, BookID, row.DedupeFingerprint)
					if err != nil {
						return nil, 0, fmt.Errorf("compare committed fill source: %w", err)
					}
					snapshot = committedFillSnapshot{originalRaw, originalNormalized, found}
					fillSnapshots[row.DedupeFingerprint] = snapshot
				}
				if snapshot.found {
					changed, err := trading212FillSourceChanged(snapshot.rawJSON, string(rawJSON), snapshot.normalizedJSON, string(normalizedJSON))
					if err != nil {
						return nil, 0, fmt.Errorf("compare Trading 212 fill payload: %w", err)
					}
					if changed {
						dedupeStatus = "needs_attention"
					}
				}
			}
		} else if seenInBatch[row.DedupeFingerprint] {
			// Duplicate within this batch (this call or an earlier one).
			dedupeStatus = "needs_attention"
		}
		seenInBatch[row.DedupeFingerprint] = true
		// An unsupported fill never posts, so it needs review — unless its
		// unchanged identity was already linked to a recorded action (T-122).
		if row.Investment == nil && unsupportedTrading212Fill(row.Raw) && dedupeStatus != "duplicate" {
			dedupeStatus = "needs_attention"
		}
		// A value rejected under the file's convention never commits (#300).
		if len(row.ParseErrors) > 0 && dedupeStatus != "duplicate" {
			dedupeStatus = "needs_attention"
		}

		resolutionJSON := "{}"
		if resolution.AppliedRuleID != nil {
			encoded, err := json.Marshal(resolution)
			if err != nil {
				return nil, 0, fmt.Errorf("encode import rule resolution: %w", err)
			}
			resolutionJSON = string(encoded)
		}

		if row.Raw["kind"] == "crypto_csv" {
			cashID, _ := strconv.ParseInt(row.Raw["cash_account_id"], 10, 64)
			commodityID, _ := strconv.ParseInt(row.Raw["cash_commodity_id"], 10, 64)
			encoded, _ := json.Marshal(ImportRowResolution{AccountID: cashID, CommodityID: commodityID})
			resolutionJSON = string(encoded)
		}
		if row.Investment != nil {
			encoded, _ := json.Marshal(row.Investment.Resolution)
			resolutionJSON = string(encoded)
			if row.Investment.Kind == "" || row.Investment.Resolution.InstrumentCommodityID == 0 {
				if dedupeStatus != "duplicate" {
					dedupeStatus = "needs_attention"
				}
			}
		}

		dbRows = append(dbRows, db.CreateImportStagedRowParams{
			BatchID:           batchID,
			BookID:            BookID,
			RowIndex:          baseIndex + i,
			DedupeFingerprint: row.DedupeFingerprint,
			RawJSON:           string(rawJSON),
			NormalizedJSON:    string(normalizedJSON),
			DedupeStatus:      dedupeStatus,
			ResolutionJSON:    resolutionJSON,
		})
	}

	if err := s.repository.InsertImportStagedRows(ctx, dbRows); err != nil {
		return nil, 0, fmt.Errorf("insert staged rows: %w", err)
	}

	stagedRows, err := s.repository.ListAllImportStagedRows(ctx, batchID)
	if err != nil {
		return nil, 0, fmt.Errorf("list staged rows: %w", err)
	}
	return stagedRows, baseIndex, nil
}

func trading212FillSourceChanged(originalRaw, currentRaw, originalNormalized, currentNormalized string) (bool, error) {
	if originalNormalized != currentNormalized {
		return true, nil
	}
	var original, current map[string]string
	if err := json.Unmarshal([]byte(originalRaw), &original); err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(currentRaw), &current); err != nil {
		return false, err
	}
	// These keys come from local resolution, not the provider payload. A
	// changed local instrument mapping must not invent a source revision.
	for _, raw := range []map[string]string{original, current} {
		delete(raw, rawKeyResolvedCommodityID)
		delete(raw, rawKeyResolvedHoldingID)
		// Order lifecycle is advisory evidence, not execution economics.
		delete(raw, "order_status")
	}
	originalSource, err := json.Marshal(original)
	if err != nil {
		return false, err
	}
	currentSource, err := json.Marshal(current)
	if err != nil {
		return false, err
	}
	return string(originalSource) != string(currentSource), nil
}

// applyImportRules snapshots the first matching enabled rule into a staged
// row's resolution. Rules never touch committed transactions and are never
// re-run over an existing batch. A category action is deliberately ignored for
// a transfer hint so a broad text match cannot turn a transfer into spending.
func applyImportRules(row StagedRow, rules []db.ImportRuleRecord) ImportRowResolution {
	for _, rule := range rules {
		candidate := row.PayeeHint
		if rule.MatchField == "description" {
			candidate = row.Memo
		}
		if !strings.Contains(strings.ToLower(candidate), strings.ToLower(rule.ContainsText)) {
			continue
		}

		resolution := ImportRowResolution{}
		if rule.CategoryID.Valid && strings.TrimSpace(row.TransferHint) == "" {
			categoryID := rule.CategoryID.Int64
			resolution.CategoryID = &categoryID
		}
		if rule.PayeeID.Valid {
			payeeID := rule.PayeeID.Int64
			resolution.PayeeID = &payeeID
			resolution.PayeeName = rule.PayeeName.String
		}
		resolution.TagIDs = append([]int64(nil), rule.TagIDs...)
		if resolution.CategoryID == nil && resolution.PayeeID == nil && len(resolution.TagIDs) == 0 {
			continue
		}
		ruleID := rule.ID
		resolution.AppliedRuleID = &ruleID
		resolution.AppliedRuleName = rule.Name
		return resolution
	}
	return ImportRowResolution{}
}

// GetImportBatch returns a batch with its staged rows (paginated).
func (s *ImportService) GetImportBatch(ctx context.Context, input GetImportBatchInput) (GetImportBatchResult, error) {
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return GetImportBatchResult{}, ErrImportBatchNotFound
		}
		return GetImportBatchResult{}, fmt.Errorf("get import batch: %w", err)
	}

	limit := input.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	var cursorRowIndex int
	var cursorID int64
	if input.Cursor != "" {
		_, _ = fmt.Sscanf(input.Cursor, "%d:%d", &cursorRowIndex, &cursorID)
	}

	stagedRows, err := s.repository.ListImportStagedRows(ctx, db.ListImportStagedRowsParams{
		BatchID:        input.BatchID,
		Limit:          limit + 1,
		CursorRowIndex: cursorRowIndex,
		CursorID:       cursorID,
	})
	if err != nil {
		return GetImportBatchResult{}, fmt.Errorf("list staged rows: %w", err)
	}

	nextCursor := ""
	if len(stagedRows) > limit {
		stagedRows = stagedRows[:limit]
		last := stagedRows[len(stagedRows)-1]
		nextCursor = fmt.Sprintf("%d:%d", last.RowIndex, last.ID)
	}

	return GetImportBatchResult{
		Batch:      toImportBatch(batch),
		Rows:       toImportStagedRows(stagedRows),
		Warnings:   storedParseWarnings(batch.SourceMetaJSON),
		NextCursor: nextCursor,
	}, nil
}

// PatchImportBatch applies row-level resolution updates (account mapping, exclude/include).
func (s *ImportService) PatchImportBatch(ctx context.Context, input PatchImportBatchInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return ErrImportBatchNotFound
		}
		return fmt.Errorf("get import batch for patch: %w", err)
	}
	// Uncommitted rows of a partially committed or failed batch stay
	// reviewable so the owner can fix them and commit again (#301).
	if !importBatchReviewable(batch.Status) {
		return ErrImportBatchNotOpen
	}

	// Normalize and validate every patch before writing any of them:
	// import_staged_rows enforces dedupe_status and resolution_json shape with
	// DB CHECK constraints, and a violation from client input should surface
	// as a clean 400 VALIDATION_FAILED rather than a 500 from a raw SQL error.
	// The repository then applies the whole patch in one transaction, so a
	// row outside the batch cannot leave earlier choices half-applied.
	rows, err := s.repository.ListAllImportStagedRows(ctx, input.BatchID)
	if err != nil {
		return fmt.Errorf("list staged rows for patch: %w", err)
	}
	rowsByID := make(map[int64]db.ImportStagedRowRecord, len(rows))
	for _, row := range rows {
		rowsByID[row.ID] = row
	}
	updates := make([]db.UpdateImportStagedRowResolutionParams, 0, len(input.RowResolutions))
	for _, patch := range input.RowResolutions {
		patch = normalizeRowResolutionPatch(patch)
		if err := validateRowResolutionPatch(patch); err != nil {
			return err
		}
		if row, ok := rowsByID[patch.RowID]; ok && patch.DedupeStatus == "duplicate" {
			status, err := s.rederiveScopedDuplicate(ctx, batch, row, patch.ResolutionJSON)
			if err != nil {
				return err
			}
			patch.DedupeStatus = status
		}
		updates = append(updates, db.UpdateImportStagedRowResolutionParams{
			RowID:          patch.RowID,
			BatchID:        input.BatchID,
			DedupeStatus:   patch.DedupeStatus,
			ResolutionJSON: patch.ResolutionJSON,
		})
	}

	if err := s.repository.PatchImportStagedRowResolutions(ctx, input.BatchID, updates); err != nil {
		switch {
		case errors.Is(err, db.ErrImportBatchNotFound):
			return ErrImportBatchNotFound
		case errors.Is(err, db.ErrImportBatchNotPreviewing):
			return ErrImportBatchNotOpen
		case errors.Is(err, db.ErrImportStagedRowNotReviewable):
			return ErrImportStagedRowNotReviewable
		case errors.Is(err, db.ErrNotFound):
			return ValidationError{Message: "row_id is not a row of this import batch"}
		}
		return fmt.Errorf("patch import batch rows: %w", err)
	}
	return nil
}

// rederiveScopedDuplicate re-decides a "duplicate" flag kept through review
// for an account-scoped row once its account is known. Staging flags such a
// row if it was committed to any account; it is a duplicate only of a commit
// to the account now chosen (#302). Without an account, or for a book-global
// identity, the flag stands.
func (s *ImportService) rederiveScopedDuplicate(ctx context.Context, batch db.ImportBatchRecord, row db.ImportStagedRowRecord, resolutionJSON string) (string, error) {
	if !accountScopedImportIdentity(batch.SourceKind, isCSVInvestmentRow(batch, row)) {
		return "duplicate", nil
	}
	resolution, err := parseResolutionJSON(resolutionJSON)
	if err != nil || resolution.AccountID == 0 {
		return "duplicate", nil
	}
	_, found, err := s.repository.FindCommitIdentity(ctx, BookID, stagedRowCommitKey(batch, row, resolution.AccountID))
	if err != nil {
		return "", fmt.Errorf("check commit identity for patch: %w", err)
	}
	switch {
	case found:
		return "duplicate", nil
	case refuseStagedParseErrors(row.NormalizedJSON) != nil:
		// Staging would have flagged it for attention (#300).
		return "needs_attention", nil
	default:
		return "new", nil
	}
}

// normalizeRowResolutionPatch applies the documented defaults for omitted
// fields. A patch replaces the row's whole review state, so an omitted
// dedupe_status means "new" (include the row) and an omitted resolution
// means "{}" (no mapping), at the service boundary as well as over HTTP.
func normalizeRowResolutionPatch(patch RowResolutionPatch) RowResolutionPatch {
	if patch.DedupeStatus == "" {
		patch.DedupeStatus = "new"
	}
	if patch.ResolutionJSON == "" {
		patch.ResolutionJSON = "{}"
	}
	return patch
}

// validRowResolutionDedupeStatuses mirrors the import_staged_rows.dedupe_status
// CHECK constraint (migrations).
var validRowResolutionDedupeStatuses = map[string]bool{
	"new":             true,
	"duplicate":       true,
	"needs_attention": true,
	"excluded":        true,
}

func validateRowResolutionPatch(patch RowResolutionPatch) error {
	if patch.RowID <= 0 {
		return ValidationError{Message: "row_id is required"}
	}
	if !validRowResolutionDedupeStatuses[patch.DedupeStatus] {
		return ValidationError{Message: "dedupe_status is invalid"}
	}
	if !json.Valid([]byte(patch.ResolutionJSON)) {
		return ValidationError{Message: "resolution is not valid JSON"}
	}
	return validateImportMatchChoice(patch.ResolutionJSON)
}

// PreviewCommit performs a dry-run of commit to report reconciliation issues and counts.
func (s *ImportService) PreviewCommit(ctx context.Context, input PreviewCommitInput) (PreviewCommitResult, error) {
	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return PreviewCommitResult{}, ErrImportBatchNotFound
		}
		return PreviewCommitResult{}, fmt.Errorf("get import batch for preview: %w", err)
	}
	// A partially committed or failed batch can hold rows awaiting review,
	// an override or a repair, so it previews the same way it commits (T-126,
	// #301).
	if !importBatchReviewable(batch.Status) {
		return PreviewCommitResult{}, ErrImportBatchNotOpen
	}

	rows, err := s.repository.ListAllImportStagedRows(ctx, input.BatchID)
	if err != nil {
		return PreviewCommitResult{}, fmt.Errorf("list rows for preview: %w", err)
	}

	var result PreviewCommitResult
	sortRowsForInvestmentCommitOrder(rows)
	for _, row := range rows {
		if row.CommitStatus == "committed" || row.CommitStatus == "skipped" {
			continue
		}
		if row.DedupeStatus == "duplicate" || row.DedupeStatus == "excluded" || row.SourceChanged || (!isCSVInvestmentRow(batch, row) && unsupportedTrading212FillJSON(row.RawJSON)) {
			result.DuplicateCount++
			continue
		}
		if isCSVInvestmentRow(batch, row) {
			dividend, err := s.csvInvestmentDividend(ctx, row, input.OwnerUserID, 0, "")
			if err != nil {
				return PreviewCommitResult{}, err
			}
			impact, err := s.investmentService.DividendReconciliationImpact(ctx, dividend)
			if err != nil {
				return PreviewCommitResult{}, err
			}
			result.addReconciliationIssue(row.RowIndex, impact.AffectedCheckpoints)
			result.IncludableCount++
			continue
		}

		if isTrading212DividendRow(batch, row) {
			c, err := s.importDividendContext(ctx, input.OwnerUserID, batch.ID, row.ID)
			if err != nil {
				return PreviewCommitResult{}, err
			}
			dividend, _, err := s.reviewedDividendInput(ctx, c, c.resolution, input.OwnerUserID, 0, "")
			if errors.Is(err, ErrImportDividendReviewRequired) {
				continue
			}
			if err != nil {
				return PreviewCommitResult{}, err
			}
			impact, err := s.investmentService.DividendReconciliationImpact(ctx, dividend)
			var validation ValidationError
			if errors.As(err, &validation) {
				continue
			}
			if err != nil {
				return PreviewCommitResult{}, err
			}
			result.addReconciliationIssue(row.RowIndex, impact.AffectedCheckpoints)
			result.IncludableCount++
			continue
		}
		if batch.SourceKind == "crypto_csv" {
			trade, side, err := s.cryptoCSVTrade(row, input.OwnerUserID, 0, "")
			if err != nil {
				return PreviewCommitResult{}, err
			}
			kind := InvestmentImpactBuy
			if side == "SELL" {
				kind = InvestmentImpactSell
			}
			impact, err := s.investmentService.TradeReconciliationImpact(ctx, kind, trade)
			if err == nil {
				if impact.GainImpact != nil && len(impact.GainImpact.Changes) > 0 {
					result.GainImpacts = append(result.GainImpacts, RowGainImpactPreview{RowID: row.ID, RowIndex: row.RowIndex, GainImpact: *impact.GainImpact})
				}
				result.addReconciliationIssue(row.RowIndex, impact.AffectedCheckpoints)
			}
			// Earlier rows in the same batch may supply the lots. Commit uses
			// the real guarded writer and holds newly changed gains for review.
			result.IncludableCount++
			continue
		}

		gainImpact, err := s.previewTrading212BuyGainImpact(ctx, row, batch, input.OwnerUserID)
		if err != nil {
			return PreviewCommitResult{}, fmt.Errorf("preview imported acquisition gains row %d: %w", row.ID, err)
		}
		if gainImpact != nil && len(gainImpact.Changes) > 0 {
			result.GainImpacts = append(result.GainImpacts, RowGainImpactPreview{
				RowID: row.ID, RowIndex: row.RowIndex, GainImpact: *gainImpact})
		}
		// An order fill commits through the investment writer, not the
		// resolution below, so its checkpoints come from the fill (#305).
		fillCheckpoints, isFill, err := s.previewTrading212OrderFillCheckpoints(ctx, row, batch, input.OwnerUserID)
		if err != nil {
			return PreviewCommitResult{}, fmt.Errorf("preview imported order fill checkpoints row %d: %w", row.ID, err)
		}
		if isFill {
			result.addReconciliationIssue(row.RowIndex, fillCheckpoints)
			result.IncludableCount++
			continue
		}

		resolution, err := parseResolutionJSON(row.ResolutionJSON)
		if err != nil || resolution.AccountID == 0 {
			// Not yet resolved — count as includable but cannot check reconciliation.
			result.IncludableCount++
			continue
		}

		// A row matching a posting already in the ledger is reviewed; a
		// standing link writes no journal and so reaches no checkpoint (#485).
		matchReview, err := s.importMatchReview(ctx, batch, row, resolution)
		if err != nil {
			return PreviewCommitResult{}, err
		}
		if matchReview != nil {
			result.MatchReviews = append(result.MatchReviews, *matchReview)
			if matchReview.NeedsDecision || matchReview.Choice.Decision == ImportMatchDecisionLink {
				result.IncludableCount++
				if !matchReview.NeedsDecision {
					result.LinkCount++
				}
				continue
			}
		}

		spec, err := buildTransactionSpec(row, resolution)
		if err != nil {
			result.IncludableCount++
			continue
		}

		impact, err := s.transactionService.ReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
			OwnerUserID: input.OwnerUserID,
			Spec:        spec,
		})
		if err != nil || len(impact.AffectedCheckpoints) == 0 {
			result.IncludableCount++
			continue
		}

		result.addReconciliationIssue(row.RowIndex, impact.AffectedCheckpoints)
		result.IncludableCount++
	}

	// The union is what the owner reviews beside the override; label its
	// accounts the way the single-write previews do.
	if err := s.transactionService.enrichCheckpointRefs(ctx, result.ReconciliationCheckpoints); err != nil {
		return PreviewCommitResult{}, err
	}
	return result, nil
}

// addReconciliationIssue records one row's checkpoints and merges them into
// the batch's reviewed union (#305).
func (r *PreviewCommitResult) addReconciliationIssue(rowIndex int, refs []db.CheckpointInvalidationRef) {
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		if ref.CheckpointID <= 0 || slices.Contains(ids, ref.CheckpointID) {
			continue
		}
		ids = append(ids, ref.CheckpointID)
		at, found := slices.BinarySearchFunc(r.ReconciliationCheckpoints, ref.CheckpointID, func(existing db.CheckpointInvalidationRef, id int64) int {
			return cmp.Compare(existing.CheckpointID, id)
		})
		if !found {
			r.ReconciliationCheckpoints = slices.Insert(r.ReconciliationCheckpoints, at, ref)
		}
	}
	if len(ids) == 0 {
		return
	}
	slices.Sort(ids)
	r.ReconciliationIssues = append(r.ReconciliationIssues, ReconciliationIssuePreview{RowIndex: rowIndex, CheckpointIDs: ids})
}

// CommitImportBatch commits all pending staged rows via the transaction service.
// Each row is its own DB transaction (partial-commit semantics).
func (s *ImportService) CommitImportBatch(ctx context.Context, input CommitImportBatchInput) (CommitImportBatchResult, error) {
	result, err := s.commitImportBatch(ctx, input)
	// A discard that lands after the open-batch check refuses the remaining
	// row writes and the final transition (T-139).
	if errors.Is(err, db.ErrImportBatchDiscarded) || errors.Is(err, db.ErrImportBatchStatusChanged) {
		return result, ErrImportBatchNotOpen
	}
	return result, err
}

func (s *ImportService) commitImportBatch(ctx context.Context, input CommitImportBatchInput) (CommitImportBatchResult, error) {
	if input.OwnerUserID <= 0 {
		return CommitImportBatchResult{}, ValidationError{Message: "owner user is required"}
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return CommitImportBatchResult{}, ErrImportBatchNotFound
		}
		return CommitImportBatchResult{}, fmt.Errorf("get import batch for commit: %w", err)
	}
	if !importBatchReviewable(batch.Status) {
		return CommitImportBatchResult{}, ErrImportBatchNotOpen
	}

	rows, err := s.repository.ListAllImportStagedRows(ctx, input.BatchID)
	if err != nil {
		return CommitImportBatchResult{}, fmt.Errorf("list rows for commit: %w", err)
	}
	sortRowsForInvestmentCommitOrder(rows)

	now := s.now().UTC()
	nowStr := now.Format(time.RFC3339)

	// A bound override names the preview's union of checkpoints. Each row
	// writes in its own transaction, so each row's set must be contained in
	// it, not equal to it (#305).
	if input.ReconciliationOverride.Allowed && input.ReconciliationOverride.Reviewed != nil {
		ctx = db.WithBatchReviewedCheckpoints(ctx)
	}

	result := CommitImportBatchResult{BatchID: input.BatchID}
	result.TotalRows = len(rows)

	for _, row := range rows {
		if !isCSVInvestmentRow(batch, row) || row.CommitStatus == "committed" || row.CommitStatus == "skipped" || row.DedupeStatus == "duplicate" || row.DedupeStatus == "excluded" {
			continue
		}
		dividend, err := s.csvInvestmentDividend(ctx, row, input.OwnerUserID, input.AuthSessionID, input.RequestID)
		if err != nil {
			return result, err
		}
		impact, err := s.investmentService.DividendReconciliationImpact(ctx, dividend)
		if err != nil {
			return result, err
		}
		if len(impact.AffectedCheckpoints) > 0 && !input.ReconciliationOverride.Allowed {
			return result, ErrReconciliationOverrideRequired
		}
	}

	for _, row := range rows {
		if row.CommitStatus == "committed" || row.CommitStatus == "skipped" {
			// Already committed or skipped in a prior partial-commit run — count
			// and continue. A failed row is retried: the owner may have fixed
			// its resolution or what made it fail (#301).
			if row.CommitStatus == "committed" {
				result.CommittedCount++
			} else {
				result.SkippedCount++
			}
			continue
		}

		if !isCSVInvestmentRow(batch, row) && unsupportedTrading212FillJSON(row.RawJSON) {
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID: row.ID, CommitStatus: "skipped",
				CommitError: sql.NullString{String: "unsupported fill type; review required", Valid: true},
			}, nil); err != nil {
				return result, fmt.Errorf("hold unsupported fill %d: %w", row.ID, err)
			}
			continue
		}

		if row.SourceChanged {
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID: row.ID, CommitStatus: "skipped",
				CommitError: sql.NullString{String: "source fill changed; correction review required", Valid: true},
			}, nil); err != nil {
				return result, fmt.Errorf("hold changed source row %d: %w", row.ID, err)
			}
			continue
		}
		if row.DedupeStatus == "duplicate" || row.DedupeStatus == "excluded" {
			// Skip rows excluded by the user or already in the ledger.
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "skipped",
			}, nil); err != nil {
				return result, fmt.Errorf("skip row %d: %w", row.ID, err)
			}
			continue
		}
		// A value the adapter rejected under the file's convention is never
		// re-read under another one, whatever the review set (#300).
		if err := refuseStagedParseErrors(row.NormalizedJSON); err != nil {
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail rejected-convention row %d: %w", row.ID, err2)
			}
			continue
		}

		// Check idempotency: if this fingerprint is already committed, skip.
		if s.beforeImportCommitIdentityCheckForTest != nil {
			s.beforeImportCommitIdentityCheckForTest()
		}
		// A CSV or QIF bank row is a duplicate only of a commit to the
		// account it resolves to now (#302).
		stagedResolution, _ := parseResolutionJSON(row.ResolutionJSON)
		commitKey := stagedRowCommitKey(batch, row, stagedResolution.AccountID)
		identity, found, err := s.repository.FindCommitIdentity(ctx, BookID, commitKey)
		if err != nil {
			return result, fmt.Errorf("check idempotency row %d: %w", row.ID, err)
		}
		if found {
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "skipped",
			}, &identity.CommittedTransactionID); err != nil {
				return result, fmt.Errorf("skip duplicate row %d: %w", row.ID, err)
			}
			continue
		}
		if s.beforeImportRowCommitForTest != nil {
			s.beforeImportRowCommitForTest()
		}

		// Named investment writers keep financial facts and source identity
		// atomic. Trading 212 dividends with missing declarations/accounts
		// remain pending; only order fills retain expected-gap cash fallback.
		// Unexpected errors fail the affected row, preserving per-row commits.
		if handled, _, _, err := s.commitInvestmentRow(ctx, row, batch, input, nowStr); err != nil {
			if isReconciliationImpactStale(err) {
				if err2 := s.holdImportStagedRowForStaleReview(ctx, &result, row.ID); err2 != nil {
					return result, fmt.Errorf("hold stale investment reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}
			var dividendValidation ValidationError
			if isTrading212DividendRow(batch, row) && (errors.Is(err, ErrImportDividendReviewRequired) || errors.As(err, &dividendValidation) || errors.Is(err, ErrReconciliationOverrideRequired)) {
				result.DividendReviewRowIDs = append(result.DividendReviewRowIDs, row.ID)
				continue
			}
			if errors.Is(err, db.ErrCommitIdentityConflict) {
				if err := s.resolveConcurrentImportCommit(ctx, row, row.DedupeFingerprint); err != nil {
					return result, fmt.Errorf("resolve concurrent investment commit row %d: %w", row.ID, err)
				}
				result.CommittedCount++
				continue
			}
			if errors.Is(err, ErrReconciliationOverrideRequired) {
				if err2 := s.holdImportStagedRowForReconciliation(ctx, &result, row.ID); err2 != nil {
					return result, fmt.Errorf("hold investment reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}
			if errors.Is(err, ErrGainImpactAcknowledgementRequired) || errors.Is(err, ErrGainImpactAcknowledgementStale) {
				// The acquisition would change committed gains that were not
				// acknowledged as they stand now. Its write rolled back; keep it
				// pending so a fresh preview can disclose the current set (T-126).
				result.GainReviewRowIDs = append(result.GainReviewRowIDs, row.ID)
				continue
			}
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail investment row %d after commit error: %w", row.ID, err2)
			}
			continue
		} else if handled {
			// The investment path recorded the import identity and marked this
			// staged row in the same transaction as its ledger/lot writes.
			// Keeping that work inside the investment transaction closes T-26:
			// a crash can no longer post an investment transaction without its
			// idempotency identity.
			result.CommittedCount++
			continue
		}

		resolution, err := parseResolutionJSON(row.ResolutionJSON)
		if err != nil || resolution.AccountID == 0 {
			if err := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: "missing account resolution", Valid: true},
			}, nil); err != nil {
				return result, fmt.Errorf("fail unresolved row %d: %w", row.ID, err)
			}
			continue
		}

		// A row matching a posting already in the ledger links to the one the
		// owner chose, or commits as new on their word; with no standing
		// decision it waits for review (#485).
		if handled, err := s.commitMatchedImportRow(ctx, &result, batch, row, resolution, commitKey, input, nowStr); err != nil {
			return result, err
		} else if handled {
			continue
		}

		spec, err := buildTransactionSpec(row, resolution)
		if err != nil {
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: err.Error(), Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail build spec row %d: %w", row.ID, err2)
			}
			continue
		}

		createParams, err := s.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
			OwnerUserID:            input.OwnerUserID,
			AuthSessionID:          input.AuthSessionID,
			RequestID:              input.RequestID,
			OriginType:             "import",
			Operation:              "transaction.create",
			Spec:                   spec,
			ChangeReason:           "imported from " + batch.SourceKind,
			ReconciliationOverride: input.ReconciliationOverride.Allowed, ReconciliationOverrideCheckpointIDs: input.ReconciliationOverride.Reviewed,
		})
		if err != nil {
			if isReconciliationImpactStale(err) {
				if err2 := s.holdImportStagedRowForStaleReview(ctx, &result, row.ID); err2 != nil {
					return result, fmt.Errorf("hold stale reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}
			if errors.Is(err, ErrReconciliationOverrideRequired) {
				// Row crosses a reconciled period and override not set → hold it
				// pending for a commit with the override (#301).
				if err2 := s.holdImportStagedRowForReconciliation(ctx, &result, row.ID); err2 != nil {
					return result, fmt.Errorf("hold reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}

			errMsg := err.Error()
			if err2 := s.recordImportStagedRowTerminal(ctx, &result, db.CommitImportStagedRowParams{
				RowID:        row.ID,
				CommitStatus: "failed",
				CommitError:  sql.NullString{String: errMsg, Valid: true},
			}, nil); err2 != nil {
				return result, fmt.Errorf("fail row %d after create error: %w", row.ID, err2)
			}
			continue
		}

		if _, err := s.repository.CommitImportedTransaction(ctx, db.CommitImportedTransactionParams{
			Identity: db.CreateImportCommitIdentityParams{
				BookID:            BookID,
				DedupeFingerprint: commitKey,
				SourceFingerprint: row.DedupeFingerprint,
				SourceKind:        batch.SourceKind,
				AccountID:         resolution.AccountID,
				CreatedAt:         nowStr,
			},
			Row: db.CommitImportStagedRowParams{
				RowID: row.ID,
			},
		}, func(tx *sql.Tx) (int64, error) {
			record, err := s.transactionService.createTransactionRecordInTx(ctx, tx, createParams)
			if err != nil {
				return 0, err
			}
			return record.ID, nil
		}); err != nil {
			// A concurrent committer already won the race for this exact
			// fingerprint (e.g. two requests committing the same batch at
			// once): the loser's own transaction insert was rolled back
			// (CommitImportedTransaction's tx never committed), so there is
			// no partial write to clean up — just adopt the winner's
			// outcome, same as the Trading 212 investment path already does
			// via resolveConcurrentImportCommit.
			if errors.Is(err, db.ErrCommitIdentityConflict) {
				if err2 := s.resolveConcurrentImportCommit(ctx, row, commitKey); err2 != nil {
					return result, fmt.Errorf("resolve concurrent commit row %d: %w", row.ID, err2)
				}
				result.CommittedCount++
				continue
			}
			// The write transaction's own guard decides against the
			// checkpoints that exist at commit (T-94): a statement finished
			// since the early check, or since the owner's review, holds this
			// row with nothing written rather than failing the batch (#305).
			if isReconciliationImpactStale(err) {
				if err2 := s.holdImportStagedRowForStaleReview(ctx, &result, row.ID); err2 != nil {
					return result, fmt.Errorf("hold stale reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}
			if errors.Is(err, ErrReconciliationOverrideRequired) || errors.Is(err, db.ErrReconciliationOverrideRequired) {
				if err2 := s.holdImportStagedRowForReconciliation(ctx, &result, row.ID); err2 != nil {
					return result, fmt.Errorf("hold reconciliation row %d: %w", row.ID, err2)
				}
				continue
			}
			return result, fmt.Errorf("record commit row %d: %w", row.ID, err)
		}
		result.CommittedCount++
	}

	// The batch is committed only when no row is left pending or failed
	// (skipped duplicates and exclusions are settled). Rows held for gain,
	// dividend or reconciliation review are still pending, so the batch stays
	// open even if nothing committed; a batch whose remaining rows all failed
	// is failed. Both stay open for repair and another commit (#301).
	held := len(result.GainReviewRowIDs) + len(result.DividendReviewRowIDs) + len(result.ReconciliationHoldRowIDs) + len(result.ReconciliationStaleRowIDs) + len(result.MatchReviewRowIDs)
	finalStatus := "committed"
	switch {
	case held > 0:
		finalStatus = "partially_committed"
	case result.FailedCount > 0 && result.CommittedCount == 0:
		finalStatus = "failed"
	case result.FailedCount > 0:
		finalStatus = "partially_committed"
	}

	detailJSON, _ := json.Marshal(map[string]any{
		"committed":       result.CommittedCount,
		"skipped":         result.SkippedCount,
		"failed":          result.FailedCount,
		"gain_review":     len(result.GainReviewRowIDs),
		"dividend_review": len(result.DividendReviewRowIDs),
		"reconciliation":  len(result.ReconciliationHoldRowIDs),
		"stale_review":    len(result.ReconciliationStaleRowIDs),
		"linked":          result.LinkedCount,
		"match_review":    len(result.MatchReviewRowIDs),
	})

	eventKind := finalStatus
	if finalStatus == "committed" {
		eventKind = "committed"
	}

	if err := s.repository.UpdateImportBatchStatus(ctx, db.UpdateImportBatchStatusParams{
		BatchID:    input.BatchID,
		Status:     finalStatus,
		EventKind:  eventKind,
		DetailJSON: string(detailJSON),
		// A concurrent commit of the same batch may already have finished;
		// only a discard (or failure) since the open check refuses.
		FromStatuses:  []string{"previewing", "partially_committed", "failed", "committed"},
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OccurredAt:    nowStr,
	}); err != nil {
		return result, fmt.Errorf("update batch status: %w", err)
	}

	result.Status = finalStatus
	return result, nil
}

// importBatchReviewable reports whether a batch's uncommitted rows can still
// be re-resolved, previewed and committed: a partially committed or failed
// batch resumes where it stopped (#301).
func importBatchReviewable(status string) bool {
	return status == "previewing" || status == "partially_committed" || status == "failed"
}

// holdImportStagedRowForReconciliation keeps a row that would cross a
// reconciliation checkpoint pending, with the reason, so a commit with
// reconciliation_override can post it (#301).
func (s *ImportService) holdImportStagedRowForReconciliation(ctx context.Context, result *CommitImportBatchResult, rowID int64) error {
	return s.recordImportStagedRowTerminal(ctx, result, db.CommitImportStagedRowParams{
		RowID:        rowID,
		CommitStatus: "pending",
		CommitError:  sql.NullString{String: "reconciliation override required", Valid: true},
	}, nil)
}

// holdImportStagedRowForStaleReview keeps a row whose write would invalidate
// a checkpoint outside the set the owner reviewed pending, with nothing
// invalidated, so another preview can show the current set (#305).
func (s *ImportService) holdImportStagedRowForStaleReview(ctx context.Context, result *CommitImportBatchResult, rowID int64) error {
	if err := s.repository.CommitImportStagedRow(ctx, db.CommitImportStagedRowParams{
		RowID:        rowID,
		CommitStatus: "pending",
		CommitError:  sql.NullString{String: importReconciliationStaleReason, Valid: true},
	}); err != nil {
		if errors.Is(err, db.ErrImportStagedRowAlreadyCommitted) {
			// A concurrent commit posted the row; adopt it as the other holds do.
			return s.recordImportStagedRowTerminal(ctx, result, db.CommitImportStagedRowParams{RowID: rowID, CommitStatus: "pending"}, nil)
		}
		return err
	}
	result.ReconciliationStaleRowIDs = append(result.ReconciliationStaleRowIDs, rowID)
	return nil
}

const importReconciliationStaleReason = "reconciliation impact changed after review"

func isReconciliationImpactStale(err error) bool {
	return errors.Is(err, ErrReconciliationImpactStale) || errors.Is(err, db.ErrReconciliationImpactStale)
}

// recordImportStagedRowTerminal applies a failed or skipped terminal update,
// or a pending reconciliation hold.
// A caller may hold a stale pending snapshot while another caller atomically
// commits the transaction, identity, and row marker. In that case the guarded
// repository update reports ErrImportStagedRowAlreadyCommitted; reload and
// verify the winner before reporting the row as a successful commit.
func (s *ImportService) recordImportStagedRowTerminal(ctx context.Context, result *CommitImportBatchResult, params db.CommitImportStagedRowParams, expectedTransactionID *int64) error {
	err := s.repository.CommitImportStagedRow(ctx, params)
	if err == nil {
		switch params.CommitStatus {
		case "skipped":
			result.SkippedCount++
		case "failed":
			result.FailedCount++
		case "pending":
			result.ReconciliationHoldRowIDs = append(result.ReconciliationHoldRowIDs, params.RowID)
		default:
			return fmt.Errorf("unsupported import staged-row terminal status %q", params.CommitStatus)
		}
		return nil
	}
	if !errors.Is(err, db.ErrImportStagedRowAlreadyCommitted) {
		return err
	}

	row, err := s.repository.ImportStagedRowByID(ctx, params.RowID)
	if err != nil {
		return fmt.Errorf("reload committed staged row: %w", err)
	}
	// A row linked to an existing posting has no effects; its transaction
	// summary names the linked transaction (#485).
	if row.CommitStatus != "committed" || !row.CommittedIdentityID.Valid || (len(row.CommitEffects) == 0 && !row.CommittedTransactionID.Valid) {
		return fmt.Errorf("staged row %d reported committed transition conflict but is not committed", params.RowID)
	}
	if expectedTransactionID != nil && *expectedTransactionID > 0 && (!row.CommittedTransactionID.Valid || row.CommittedTransactionID.Int64 != *expectedTransactionID) {
		return fmt.Errorf("staged row %d committed transaction does not match import identity", params.RowID)
	}
	result.CommittedCount++
	return nil
}

// resolveConcurrentImportCommit turns a duplicate transaction attempt into
// the successful outcome it represents. Commit identities and the winning
// row marker share one transaction, so once an identity conflict occurs its
// referenced transaction is durable and the row is either already committed
// or still pending for this conditional repair.
func (s *ImportService) resolveConcurrentImportCommit(ctx context.Context, row db.ImportStagedRowRecord, commitKey string) error {
	identity, found, err := s.repository.FindCommitIdentity(ctx, BookID, commitKey)
	if err != nil {
		return fmt.Errorf("find existing commit identity: %w", err)
	}
	if !found {
		return errors.New("commit identity conflict without existing identity")
	}
	if _, err := s.repository.MarkImportStagedRowCommittedIfPending(ctx, row.ID, identity.ID, identity.CommittedTransactionID); err != nil {
		return fmt.Errorf("mark existing commit identity as committed: %w", err)
	}
	return nil
}

// recordCommitIdentityAndMarkRowInTx is the investment-import variant used by
// Buy/Sell/Dividend's post-write callback. The caller owns tx, which already
// contains the transaction and, for trades, lot/acquisition or disposal
// writes. Keeping the identity and staged-row marker in that tx makes the
// at-least-once import retry boundary genuinely idempotent.
func (s *ImportService) recordCommitIdentityAndMarkRowInTx(ctx context.Context, tx *sql.Tx, rowID int64, dedupeFingerprint string, sourceKind string, accountID int64, transactionID int64, nowStr string) error {
	identityID, err := s.repository.CreateCommitIdentityWithEffects(ctx, tx, db.CreateImportCommitIdentityParams{
		BookID:                 BookID,
		DedupeFingerprint:      dedupeFingerprint,
		CommittedTransactionID: transactionID,
		SourceKind:             sourceKind,
		AccountID:              accountID,
		CreatedAt:              nowStr,
	}, []db.CreateImportCommitEffectParams{{TransactionID: sql.NullInt64{Int64: transactionID, Valid: true}}})
	if err != nil {
		return fmt.Errorf("create investment commit identity: %w", err)
	}
	if err := s.repository.CommitImportStagedRowInTx(ctx, tx, db.CommitImportStagedRowParams{
		RowID:                  rowID,
		CommitStatus:           "committed",
		CommittedIdentityID:    sql.NullInt64{Int64: identityID, Valid: true},
		CommittedTransactionID: sql.NullInt64{Int64: transactionID, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark investment row committed: %w", err)
	}
	return nil
}

// DiscardImportBatch marks a previewing batch as discarded.
func (s *ImportService) DiscardImportBatch(ctx context.Context, input DiscardImportBatchInput) error {
	if input.OwnerUserID <= 0 {
		return ValidationError{Message: "owner user is required"}
	}

	batch, err := s.repository.ImportBatchByID(ctx, BookID, input.BatchID)
	if err != nil {
		if errors.Is(err, db.ErrImportBatchNotFound) {
			return ErrImportBatchNotFound
		}
		return fmt.Errorf("get import batch for discard: %w", err)
	}
	if batch.Status != "previewing" {
		return ErrImportBatchNotOpen
	}

	now := s.now().UTC().Format(time.RFC3339)
	err = s.repository.UpdateImportBatchStatus(ctx, db.UpdateImportBatchStatusParams{
		BatchID:       input.BatchID,
		Status:        "discarded",
		EventKind:     "discarded",
		DetailJSON:    "{}",
		FromStatuses:  []string{"previewing"},
		ActorUserID:   input.OwnerUserID,
		AuthSessionID: input.AuthSessionID,
		RequestID:     input.RequestID,
		OccurredAt:    now,
	})
	if errors.Is(err, db.ErrImportBatchStatusChanged) {
		return ErrImportBatchNotOpen
	}
	return err
}

// ListImportBatches returns paginated batch history.
func (s *ImportService) ListImportBatches(ctx context.Context, input ListImportBatchesInput) (ListImportBatchesResult, error) {
	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var cursorCreatedAt string
	var cursorID int64
	if input.Cursor != "" {
		_, _ = fmt.Sscanf(input.Cursor, "%s:%d", &cursorCreatedAt, &cursorID)
	}

	batches, err := s.repository.ListImportBatches(ctx, db.ListImportBatchesParams{
		BookID:          BookID,
		Limit:           limit + 1,
		CursorCreatedAt: cursorCreatedAt,
		CursorID:        cursorID,
	})
	if err != nil {
		return ListImportBatchesResult{}, fmt.Errorf("list import batches: %w", err)
	}

	nextCursor := ""
	if len(batches) > limit {
		batches = batches[:limit]
		last := batches[len(batches)-1]
		nextCursor = fmt.Sprintf("%s:%d", last.CreatedAt, last.ID)
	}

	return ListImportBatchesResult{
		Batches:    toImportBatches(batches),
		NextCursor: nextCursor,
	}, nil
}

// --- Helpers ---

// sortRowsForInvestmentCommitOrder stable-sorts staged rows so Trading 212
// order fills commit oldest-first by their full provider fill timestamp,
// instead of the provider's newest-first paging order (row_index ASC).
// Ordering by only the YYYY-MM-DD transaction date is insufficient: an
// intraday SELL can otherwise remain before the BUY that opened its lot,
// fail "insufficient lots", and fall back to a plain cash transaction.
// Non-investment rows have no normalized date key here and sort by their
// empty key, which — being a stable sort — preserves their original
// row_index-relative order; only investment rows are reordered relative to
// each other and to non-investment rows.
func sortRowsForInvestmentCommitOrder(rows []db.ImportStagedRowRecord) {
	// Decorate-sort-undecorate: investmentRowSortKey unmarshals JSON, so
	// computing it once per row rather than on every comparison keeps a large
	// brokerage import from re-parsing the same rows O(n log n) times. The key
	// travels with its row rather than sitting in a map, so rows that share an
	// ID (or carry none) still sort by their own key.
	type keyedRow struct {
		key string
		row db.ImportStagedRowRecord
	}
	decorated := make([]keyedRow, len(rows))
	for i, row := range rows {
		decorated[i] = keyedRow{key: investmentRowSortKey(row), row: row}
	}
	slices.SortStableFunc(decorated, func(a, b keyedRow) int {
		return cmp.Compare(a.key, b.key)
	})
	for i, d := range decorated {
		rows[i] = d.row
	}
}

// investmentRowSortKey returns a full order-fill timestamp where available,
// falling back to the normalized date for dividends and older staged data.
// Empty keys preserve original row order via the stable sort.
func investmentRowSortKey(row db.ImportStagedRowRecord) string {
	var raw map[string]string
	if err := json.Unmarshal([]byte(row.RawJSON), &raw); err != nil {
		return ""
	}
	kind := raw[rawKeyKind]
	if kind != trading212RawKindOrderFill && kind != trading212RawKindDividend && kind != "crypto_csv" {
		return ""
	}
	if kind == trading212RawKindOrderFill && raw[rawKeyFilledAt] != "" {
		return raw[rawKeyFilledAt]
	}
	var normalized struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal([]byte(row.NormalizedJSON), &normalized); err != nil {
		return ""
	}
	return normalized.Date
}

func hashFingerprint(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex, sufficient uniqueness
}

// accountScopedImportIdentity reports whether a row's commit identity is
// scoped to the account it posts to (#302). CSV and QIF bank rows are: the
// same row in two accounts' statements is two transactions, and a statement
// names no account until review. A CSV bank reference is scoped too, since a
// bank may number each account's statement on its own. Provider rows
// (Trading 212, crypto CSV) and CSV investment rows already carry their
// connection or accounts in the fingerprint and stay book-global.
func accountScopedImportIdentity(sourceKind string, investment bool) bool {
	return (sourceKind == "csv" || sourceKind == "qif") && !investment
}

// importCommitKey is the unique import_commit_identities key for a staged
// row's fingerprint committed to accountID.
func importCommitKey(fingerprint string, scoped bool, accountID int64) string {
	if !scoped {
		return fingerprint
	}
	return hashFingerprint(fmt.Sprintf("%s|account:%d", fingerprint, accountID))
}

// stagedRowCommitKey is importCommitKey for a staged row resolved to accountID.
func stagedRowCommitKey(batch db.ImportBatchRecord, row db.ImportStagedRowRecord, accountID int64) string {
	return importCommitKey(row.DedupeFingerprint, accountScopedImportIdentity(batch.SourceKind, isCSVInvestmentRow(batch, row)), accountID)
}

func parseResolutionJSON(resolutionJSON string) (ImportRowResolution, error) {
	if resolutionJSON == "" || resolutionJSON == "{}" {
		return ImportRowResolution{}, nil
	}
	var r ImportRowResolution
	if err := json.Unmarshal([]byte(resolutionJSON), &r); err != nil {
		return ImportRowResolution{}, fmt.Errorf("parse resolution json: %w", err)
	}
	return r, nil
}

// resolveOffsetAccount returns the account ID for the second posting leg.
// Transfer rows use TransferAccountID; ordinary rows use CategoryID.
func resolveOffsetAccount(resolution ImportRowResolution) (int64, error) {
	if resolution.TransferAccountID != nil {
		return *resolution.TransferAccountID, nil
	}
	if resolution.CategoryID != nil {
		return *resolution.CategoryID, nil
	}
	return 0, fmt.Errorf("missing category or transfer account resolution")
}

// buildTransactionSpec constructs a balanced TransactionInput from a staged row + resolution.
// For a simple bank account debit/credit, the two legs are:
//   - account posting: the signed amount
//   - category/offset posting: the negated amount (expense/income/transfer)
//
// When the row has QIF splits, each split becomes its own offset posting.
func buildTransactionSpec(row db.ImportStagedRowRecord, resolution ImportRowResolution) (TransactionInput, error) {
	// Parse normalized JSON for date, amount, splits, etc.
	if err := refuseStagedParseErrors(row.NormalizedJSON); err != nil {
		return TransactionInput{}, err
	}
	var normalized struct {
		Date         string `json:"date"`
		Amount       string `json:"amount"`
		PayeeHint    string `json:"payee_hint"`
		Memo         string `json:"memo"`
		ExternalRef  string `json:"external_ref"`
		TransferHint string `json:"transfer_hint"`
		Splits       []struct {
			CategoryHint string `json:"category_hint"`
			Amount       string `json:"amount"`
			Memo         string `json:"memo"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(row.NormalizedJSON), &normalized); err != nil {
		return TransactionInput{}, fmt.Errorf("parse normalized json: %w", err)
	}

	// Adapters stage ISO dates and canonical amounts; the file's convention
	// was applied there, and anything else is refused, not guessed (#300).
	date, err := parseCanonicalDate(normalized.Date)
	if err != nil {
		return TransactionInput{}, fmt.Errorf("parse date %q: %w", normalized.Date, err)
	}

	coeff, scale, err := parseCanonicalAmount(normalized.Amount)
	if err != nil {
		return TransactionInput{}, fmt.Errorf("parse amount %q: %w", normalized.Amount, err)
	}

	if resolution.CommodityID == 0 {
		return TransactionInput{}, fmt.Errorf("missing commodity resolution")
	}

	// Posting 1: the account (the raw amount, signed as-is from the file).
	posting1 := PostingInput{
		AccountID:     resolution.AccountID,
		QuantityValue: coeff,
		QuantityScale: scale,
		CommodityID:   resolution.CommodityID,
		Memo:          normalized.Memo,
	}

	var offsetPostings []PostingInput

	// Resolve the offset account: transfer takes priority over category.
	offsetAccountID, err := resolveOffsetAccount(resolution)
	if err != nil {
		return TransactionInput{}, err
	}

	if len(normalized.Splits) > 0 {
		// Splits: one offset posting per split line, each with its own amount.
		// All splits share the same resolved offset account (category or transfer).
		for _, split := range normalized.Splits {
			splitCoeff, splitScale, err := parseCanonicalAmount(split.Amount)
			if err != nil {
				return TransactionInput{}, fmt.Errorf("parse split amount %q: %w", split.Amount, err)
			}
			negSplitCoeff := exact.Coefficient(new(big.Int).Neg(splitCoeff.BigInt()).String())
			offsetPostings = append(offsetPostings, PostingInput{
				AccountID:     offsetAccountID,
				QuantityValue: negSplitCoeff,
				QuantityScale: splitScale,
				CommodityID:   resolution.CommodityID,
				Memo:          split.Memo,
			})
		}
	} else {
		// No splits: single offset posting (category or transfer account), negated.
		negCoeff := exact.Coefficient(new(big.Int).Neg(coeff.BigInt()).String())
		offsetPostings = append(offsetPostings, PostingInput{
			AccountID:     offsetAccountID,
			QuantityValue: negCoeff,
			QuantityScale: scale,
			CommodityID:   resolution.CommodityID,
		})
	}

	postings := append([]PostingInput{posting1}, offsetPostings...)

	payeeName := resolution.PayeeName
	if payeeName == "" {
		payeeName = normalized.PayeeHint
	}

	spec := TransactionInput{
		Status:          "posted",
		TransactionKind: "ordinary",
		TransactionDate: date,
		PayeeID:         resolution.PayeeID,
		PayeeName:       payeeName,
		Description:     normalized.Memo,
		ExternalRefHint: normalized.ExternalRef,
		NeedsReview:     true,
		TagIDs:          append([]int64(nil), resolution.TagIDs...),
		JournalEntries: []JournalEntryInput{
			{
				EntryDate: date,
				EntryKind: "ordinary",
				Postings:  postings,
			},
		},
	}

	return spec, nil
}

// storedParseWarnings reads the parse warnings a batch keeps in its
// source_meta "warnings" (file batches and online batches use the same key).
func storedParseWarnings(sourceMetaJSON string) []ParseWarning {
	var meta struct {
		Warnings []ParseWarning `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(sourceMetaJSON), &meta); err != nil {
		return nil
	}
	return meta.Warnings
}

// refuseStagedParseErrors returns the commit error for a staged row whose
// date, amount or split amount failed under the file's convention (#300).
func refuseStagedParseErrors(normalizedJSON string) error {
	var normalized struct {
		ParseErrors []StagedParseError `json:"parse_errors"`
	}
	if err := json.Unmarshal([]byte(normalizedJSON), &normalized); err != nil || len(normalized.ParseErrors) == 0 {
		return nil
	}
	first := normalized.ParseErrors[0]
	return fmt.Errorf("%s %q does not match the file's date or decimal convention; correct the import profile or the file and upload it again, or exclude the row", strings.ReplaceAll(first.Field, "_", " "), first.Value)
}

// parseCanonicalDate accepts only the ISO date an adapter stages.
func parseCanonicalDate(raw string) (string, error) {
	if _, err := time.Parse(time.DateOnly, raw); err != nil {
		return "", fmt.Errorf("staged date is not YYYY-MM-DD")
	}
	return raw, nil
}

// parseCanonicalAmount parses an amount already in canonical form ("-1234.56"),
// as adapters stage it after applying the file's decimal convention. It never
// detects a separator: text in any other shape is refused (#300).
func parseCanonicalAmount(raw string) (exact.Coefficient, int, error) {
	if !isCanonicalDecimal(raw) {
		return "", 0, fmt.Errorf("staged amount is not a canonical decimal")
	}
	return coefficientFromCanonical(raw)
}

func isCanonicalDecimal(raw string) bool {
	intPart, fracPart, hasPoint := strings.Cut(strings.TrimPrefix(raw, "-"), ".")
	return isDigits(intPart) && intPart != "" && (!hasPoint || (fracPart != "" && isDigits(fracPart)))
}

// parseDecimalAmount parses a decimal string (e.g. "-123.45") into a Coefficient + scale.
// No floating-point is used; we parse digit strings directly. Decimal-comma
// input ("1.234,56") is normalized first, detecting the separator from this one
// value — so it is only for text that has no file-level convention (provider
// API values, crypto CSV's dot-only cells, owner-entered review amounts).
// Staged CSV/QIF text uses parseCanonicalAmount.
func parseDecimalAmount(raw string) (exact.Coefficient, int, error) {
	canonical, err := canonicalDecimal(raw, 0)
	if err != nil {
		return "", 0, err
	}
	return coefficientFromCanonical(canonical)
}

func coefficientFromCanonical(canonical string) (exact.Coefficient, int, error) {
	negative := false
	if canonical[0] == '-' {
		negative = true
		canonical = canonical[1:]
	}

	intPart, fracPart, _ := strings.Cut(canonical, ".")

	scale := len(fracPart)
	digits := intPart + fracPart

	// Strip leading zeros to get canonical integer.
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		digits = "0"
	}
	if negative {
		digits = "-" + digits
	}

	coeff, err := exact.Parse(digits)
	if err != nil {
		return "", 0, fmt.Errorf("parse coefficient: %w", err)
	}
	return coeff, scale, nil
}

// --- Mapping helpers ---

func toImportBatch(rec db.ImportBatchRecord) ImportBatch {
	var profileID *int64
	if rec.ProfileID.Valid {
		profileID = &rec.ProfileID.Int64
	}
	var connectionID *int64
	if rec.ConnectionID.Valid {
		connectionID = &rec.ConnectionID.Int64
	}
	return ImportBatch{
		ID:               rec.ID,
		BookID:           rec.BookID,
		SourceKind:       rec.SourceKind,
		ProfileID:        profileID,
		ConnectionID:     connectionID,
		Status:           rec.Status,
		OriginalFilename: rec.OriginalFilename,
		SourceMetaJSON:   rec.SourceMetaJSON,
		CreatedAt:        rec.CreatedAt,
	}
}

func toImportBatches(records []db.ImportBatchRecord) []ImportBatch {
	out := make([]ImportBatch, len(records))
	for i, r := range records {
		out[i] = toImportBatch(r)
	}
	return out
}

func toImportStagedRow(rec db.ImportStagedRowRecord) ImportStagedRow {
	var identityID *int64
	if rec.CommittedIdentityID.Valid {
		identityID = &rec.CommittedIdentityID.Int64
	}
	var txnID *int64
	if rec.CommittedTransactionID.Valid {
		txnID = &rec.CommittedTransactionID.Int64
	}
	commitError := ""
	if rec.CommitError.Valid {
		commitError = rec.CommitError.String
	}
	var sourceTransactionID *int64
	if rec.SourceTransactionID.Valid {
		sourceTransactionID = &rec.SourceTransactionID.Int64
	}
	effects := make([]ImportCommitEffect, 0, len(rec.CommitEffects))
	for _, effect := range rec.CommitEffects {
		converted := ImportCommitEffect{EffectSeq: effect.EffectSeq}
		if effect.OperationID.Valid {
			id := effect.OperationID.Int64
			converted.OperationID = &id
		}
		if effect.TransactionID.Valid {
			id := effect.TransactionID.Int64
			converted.TransactionID = &id
		}
		effects = append(effects, converted)
	}
	return ImportStagedRow{
		ID:                     rec.ID,
		BatchID:                rec.BatchID,
		BookID:                 rec.BookID,
		RowIndex:               rec.RowIndex,
		DedupeFingerprint:      rec.DedupeFingerprint,
		RawJSON:                rec.RawJSON,
		NormalizedJSON:         rec.NormalizedJSON,
		DedupeStatus:           rec.DedupeStatus,
		SourceChanged:          rec.SourceChanged,
		SourceBuyOperation:     rec.SourceBuyOperation,
		SourceSaleOperation:    rec.SourceSaleOperation,
		SourceTransactionID:    sourceTransactionID,
		ResolutionJSON:         rec.ResolutionJSON,
		CommitStatus:           rec.CommitStatus,
		CommittedIdentityID:    identityID,
		CommittedTransactionID: txnID,
		CommitEffects:          effects,
		CommitError:            commitError,
	}
}

func toImportStagedRows(records []db.ImportStagedRowRecord) []ImportStagedRow {
	out := make([]ImportStagedRow, len(records))
	for i, r := range records {
		out[i] = toImportStagedRow(r)
	}
	return out
}

func (s *ImportService) SetSettings(settings *SettingsService) { s.settings = settings }
