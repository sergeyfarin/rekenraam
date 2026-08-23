package api

import (
	"fmt"
	"log/slog"
	"net/http"
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

type exportPreviewResponse struct {
	GeneratedAt             string                     `json:"generated_at"`
	SelectionUnit           string                     `json:"selection_unit"`
	RecordPolicy            string                     `json:"record_policy"`
	AllTransactionsComplete bool                       `json:"all_transactions_complete"`
	IncludesSystemAccounts  bool                       `json:"includes_system_accounts"`
	Columns                 []string                   `json:"columns"`
	Excluded                []string                   `json:"excluded"`
	Ledger                  exportLedgerTotalsResponse `json:"ledger"`
	Attachments             exportAttachmentsResponse  `json:"attachments"`
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

		preview, err := exportService.Preview(r.Context())
		if err != nil {
			writeServiceInternalError(w, r, logger, "read export preview", err)
			return
		}

		writeJSON(w, http.StatusOK, exportPreviewResponse{
			GeneratedAt:             preview.GeneratedAt,
			SelectionUnit:           preview.SelectionUnit,
			RecordPolicy:            preview.RecordPolicy,
			AllTransactionsComplete: preview.AllTransactionsComplete,
			IncludesSystemAccounts:  preview.IncludesSystemAccounts,
			Columns:                 preview.Columns,
			Excluded:                preview.Excluded,
			Ledger: exportLedgerTotalsResponse{
				PostingCount:      preview.PostingCount,
				JournalEntryCount: preview.JournalEntryCount,
				TransactionCount:  preview.TransactionCount,
				AccountCount:      preview.AccountCount,
				CommodityCount:    preview.CommodityCount,
				EarliestEntryDate: preview.EarliestEntryDate,
				LatestEntryDate:   preview.LatestEntryDate,
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
