package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rekenraam/backend/internal/app"
)

type exportAttachmentsResponse struct {
	Included  bool    `json:"included"`
	Directory *string `json:"directory"`
	Reason    string  `json:"reason"`
}

type exportLedgerTotalsResponse struct {
	PostingCount      int64  `json:"posting_count"`
	JournalEntryCount int64  `json:"journal_entry_count"`
	TransactionCount  int64  `json:"transaction_count"`
	AccountCount      int64  `json:"account_count"`
	CommodityCount    int64  `json:"commodity_count"`
	EarliestEntryDate string `json:"earliest_entry_date,omitempty"`
	LatestEntryDate   string `json:"latest_entry_date,omitempty"`
}

type exportFilterResponse struct {
	From               string   `json:"from,omitempty"`
	To                 string   `json:"to,omitempty"`
	DateBasis          string   `json:"date_basis"`
	AccountIDs         []int64  `json:"account_ids"`
	IncludeDescendants bool     `json:"include_descendants"`
	ResolvedAccountIDs []int64  `json:"resolved_account_ids"`
	CommodityIDs       []int64  `json:"commodity_ids"`
	SelectionUnit      string   `json:"selection_unit"`
	Notes              []string `json:"notes,omitempty"`
}

type exportQIFAccountResponse struct {
	AccountID   int64  `json:"account_id"`
	AccountPath string `json:"account_path"`
	QIFType     string `json:"qif_type,omitempty"`
	Supported   bool   `json:"supported"`
	Reason      string `json:"reason,omitempty"`
}

type exportQIFResponse struct {
	DateLayout  string                     `json:"date_layout"`
	Supported   []exportQIFAccountResponse `json:"supported_accounts"`
	Unsupported []exportQIFAccountResponse `json:"unsupported_accounts"`
}

type exportPreviewResponse struct {
	GeneratedAt                string                     `json:"generated_at"`
	SelectionUnit              string                     `json:"selection_unit"`
	RecordPolicy               string                     `json:"record_policy"`
	Query                      exportFilterResponse       `json:"query"`
	AllTransactionsComplete    bool                       `json:"all_transactions_complete"`
	IncompleteTransactionCount int64                      `json:"incomplete_transaction_count"`
	IncludesSystemAccounts     bool                       `json:"includes_system_accounts"`
	Columns                    []string                   `json:"columns"`
	Excluded                   []string                   `json:"excluded"`
	Ledger                     exportLedgerTotalsResponse `json:"ledger"`
	QIF                        exportQIFResponse          `json:"qif"`
	Attachments                exportAttachmentsResponse  `json:"attachments"`
}

// parseExportFilter reads the shared scope parameters. Repeated-ID filters
// follow R2's contract, so "these accounts" means the same thing in a report
// and in an export.
func parseExportFilter(query url.Values) (app.ExportFilter, error) {
	accountIDs, err := parseRepeatedIDs(query, "account_id")
	if err != nil {
		return app.ExportFilter{}, err
	}
	commodityIDs, err := parseRepeatedIDs(query, "commodity_id")
	if err != nil {
		return app.ExportFilter{}, err
	}

	return app.ExportFilter{
		From:               strings.TrimSpace(query.Get("from")),
		To:                 strings.TrimSpace(query.Get("to")),
		DateBasis:          strings.TrimSpace(query.Get("date_basis")),
		AccountIDs:         accountIDs,
		IncludeDescendants: query.Get("include_descendants") == "true",
		CommodityIDs:       commodityIDs,
	}, nil
}

func toExportFilterResponse(filter app.ResolvedExportFilter) exportFilterResponse {
	return exportFilterResponse{
		From:               filter.From,
		To:                 filter.To,
		DateBasis:          filter.DateBasis,
		AccountIDs:         emptyIfNil(filter.AccountIDs),
		IncludeDescendants: filter.IncludeDescendants,
		ResolvedAccountIDs: emptyIfNil(filter.ResolvedAccountIDs),
		CommodityIDs:       emptyIfNil(filter.CommodityIDs),
		SelectionUnit:      filter.SelectionUnit,
		Notes:              filter.Notes,
	}
}

// exportScopeParameters are the filters bundle.zip will accept and the flat CSV
// never will. Ignoring them silently would hand back a whole-ledger file to a
// caller who believes it is scoped, so they are rejected by name.
var exportScopeParameters = []string{
	"from",
	"to",
	"date_basis",
	"account_id",
	"include_descendants",
	"commodity_id",
}

func exportPreview(logger *slog.Logger, authService *app.AuthService, exportService *app.ExportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		query := r.URL.Query()
		filter, err := parseExportFilter(query)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		preview, err := exportService.Preview(r.Context(), filter)
		if err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			writeServiceInternalError(w, r, logger, "read export preview", err)
			return
		}

		qifSelection, err := exportService.ResolveQIFSelection(r.Context(), filter, query.Get("qif_date_layout"))
		if err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			writeServiceInternalError(w, r, logger, "read qif export selection", err)
			return
		}

		writeJSON(w, http.StatusOK, exportPreviewResponse{
			GeneratedAt:                preview.GeneratedAt,
			SelectionUnit:              preview.SelectionUnit,
			RecordPolicy:               preview.RecordPolicy,
			Query:                      toExportFilterResponse(preview.Filter),
			AllTransactionsComplete:    preview.AllTransactionsComplete,
			IncompleteTransactionCount: preview.IncompleteTransactionCount,
			IncludesSystemAccounts:     preview.IncludesSystemAccounts,
			Columns:                    preview.Columns,
			Excluded:                   preview.Excluded,
			Ledger: exportLedgerTotalsResponse{
				PostingCount:      preview.PostingCount,
				JournalEntryCount: preview.JournalEntryCount,
				TransactionCount:  preview.TransactionCount,
				AccountCount:      preview.AccountCount,
				CommodityCount:    preview.CommodityCount,
				EarliestEntryDate: preview.EarliestEntryDate,
				LatestEntryDate:   preview.LatestEntryDate,
			},
			QIF: exportQIFResponse{
				DateLayout:  string(qifSelection.Layout),
				Supported:   toQIFAccountResponses(qifSelection.Supported),
				Unsupported: toQIFAccountResponses(qifSelection.Unsupported),
			},
			Attachments: exportAttachmentsResponse{
				Included:  preview.Attachments.Included,
				Directory: preview.Attachments.Directory,
				Reason:    preview.Attachments.Reason,
			},
		})
	}
}

func exportLedgerCSV(logger *slog.Logger, authService *app.AuthService, exportService *app.ExportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		query := r.URL.Query()
		for _, parameter := range exportScopeParameters {
			if query.Has(parameter) {
				writeAPIError(w, http.StatusBadRequest, "EXPORT_SCOPE_UNSUPPORTED",
					fmt.Sprintf("%s is not accepted here: the flat ledger CSV is always the whole ledger, and a scoped export is produced as an archive that carries its own manifest", parameter))
				return
			}
		}

		filename := "rekenraam-ledger-" + time.Now().UTC().Format("20060102") + ".csv"
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		// Streamed and snapshot-bound: a cached copy would be a stale ledger.
		w.Header().Set("Cache-Control", "no-store")

		// The status code is committed the moment the first byte lands, so a
		// failure past that point can only be logged and the response
		// truncated. That is why /exports/preview exists: it is where a caller
		// finds out whether the export is sound while an error envelope is
		// still possible.
		if _, err := exportService.WriteLedgerCSV(r.Context(), w); err != nil {
			if logger == nil {
				logger = slog.Default()
			}
			logger.ErrorContext(r.Context(), "write ledger export", slog.Any("err", err))
			return
		}
	}
}

// writeExportValidationError maps a rejected filter to the envelope. Reported
// here rather than as a 500, because "account filter is invalid" is something
// the caller can fix.
func writeExportValidationError(w http.ResponseWriter, err error) bool {
	var validationErr app.ValidationError
	if errors.As(err, &validationErr) {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", validationErr.Message)
		return true
	}
	return false
}

func exportBundle(logger *slog.Logger, authService *app.AuthService, exportService *app.ExportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		filter, err := parseExportFilter(r.URL.Query())
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}

		// The filter is validated before the first byte, because a streamed
		// archive cannot change its status code afterwards. /exports/preview
		// answers the same question in full without producing the file.
		if err := exportService.ValidateFilter(r.Context(), filter); err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			writeServiceInternalError(w, r, logger, "validate export bundle request", err)
			return
		}

		// Headers are attached on the first byte, not before it. A failure
		// between here and there — the trial-balance fold reads the whole book —
		// would otherwise leave the caller holding a 200 with an empty body that
		// claims to be a zip. This way such a failure is still an error
		// envelope, and only a failure *during* the archive truncates it, which
		// a zip reader rejects rather than accepting as a short export.
		filename := "rekenraam-export-" + time.Now().UTC().Format("20060102") + ".zip"
		body := &deferredHeaderWriter{response: w, onFirstWrite: func() {
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			w.Header().Set("Cache-Control", "no-store")
		}}

		if err := exportService.WriteBundle(r.Context(), body, filter); err != nil {
			if logger == nil {
				logger = slog.Default()
			}
			logger.ErrorContext(r.Context(), "write export bundle", slog.Any("err", err))
			if !body.started {
				writeServiceInternalError(w, r, logger, "write export bundle", err)
			}
			return
		}
	}
}

// deferredHeaderWriter delays a download's headers until something is actually
// being downloaded, so a failure before the first byte can still be reported as
// an error rather than as an empty file.
type deferredHeaderWriter struct {
	response     io.Writer
	onFirstWrite func()
	started      bool
}

func (w *deferredHeaderWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.started = true
		w.onFirstWrite()
	}
	return w.response.Write(p)
}

func toQIFAccountResponses(accounts []app.QIFAccountStatus) []exportQIFAccountResponse {
	responses := make([]exportQIFAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		responses = append(responses, exportQIFAccountResponse{
			AccountID:   account.AccountID,
			AccountPath: account.AccountPath,
			QIFType:     account.QIFType,
			Supported:   account.Supported,
			Reason:      account.Reason,
		})
	}
	return responses
}

func exportQIF(logger *slog.Logger, authService *app.AuthService, exportService *app.ExportService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticatedOwner(w, r, logger, authService); !ok {
			return
		}

		query := r.URL.Query()
		filter, err := parseExportFilter(query)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
			return
		}
		layout := query.Get("qif_date_layout")
		allowPartial := query.Get("allow_partial") == "true"

		// Which accounts can be written is decided before anything is, so a
		// selection carrying accounts QIF cannot express is refused by name
		// rather than downloaded silently short.
		selection, err := exportService.ResolveQIFSelection(r.Context(), filter, layout)
		if err != nil {
			if writeExportValidationError(w, err) {
				return
			}
			writeServiceInternalError(w, r, logger, "resolve qif export selection", err)
			return
		}
		if len(selection.Unsupported) > 0 && !allowPartial {
			writeAPIError(w, http.StatusUnprocessableEntity, "QIF_ACCOUNT_UNSUPPORTED",
				"QIF cannot represent "+qifUnsupportedSummary(selection.Unsupported)+
					"; export those through the CSV bundle, or repeat this request with allow_partial=true to accept the omission")
			return
		}
		if len(selection.Supported) == 0 {
			writeAPIError(w, http.StatusUnprocessableEntity, "QIF_ACCOUNT_UNSUPPORTED",
				"no account in this selection can be written as QIF; the CSV bundle carries them all")
			return
		}

		// WriteQIF decides between a bare file and an archive from what it
		// renders — an ambiguous-only file has nowhere to state its layout — so
		// it announces the shape through this callback before writing anything,
		// and the headers are attached then. A failure before that point is
		// still an error envelope, because nothing has been written yet.
		var started bool
		err = exportService.WriteQIF(r.Context(), w, filter, layout, allowPartial, func(result app.QIFExportResult) {
			started = true
			w.Header().Set("Content-Type", result.ContentType)
			w.Header().Set("Content-Disposition", `attachment; filename="`+result.Filename+`"`)
			w.Header().Set("Cache-Control", "no-store")
		})
		if err != nil {
			if logger == nil {
				logger = slog.Default()
			}
			logger.ErrorContext(r.Context(), "write qif export", slog.Any("err", err))
			if !started {
				if errors.Is(err, app.ErrQIFSelectionUnsupported) || errors.Is(err, app.ErrQIFNothingToExport) {
					writeAPIError(w, http.StatusUnprocessableEntity, "QIF_ACCOUNT_UNSUPPORTED", err.Error())
					return
				}
				if writeExportValidationError(w, err) {
					return
				}
				writeServiceInternalError(w, r, logger, "write qif export", err)
			}
			return
		}
	}
}

// qifUnsupportedSummary names the accounts rather than counting them: a reader
// deciding whether to accept an omission needs to know what is being omitted.
func qifUnsupportedSummary(accounts []app.QIFAccountStatus) string {
	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.AccountPath+" ("+account.Reason+")")
	}
	return strings.Join(names, ", ")
}
