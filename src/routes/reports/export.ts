import type {
  AccountTrendRow,
  AccountValuationRow,
  CashflowRow,
  CategorySpendRow,
  CurrencyExposureRow,
  NetWorthRow,
  PayeeTotalRow,
  PortfolioPerformanceSummary,
  RealizedGainEntry,
  ReportDefinitionSummary,
  ReportRunSummary,
  UnrealizedGainEntry,
} from "$lib/api/reports";
import type { ReportsTab } from "./types";

type CsvValue = string | number | boolean | null | undefined;

export type ReportsExportState = {
  activeTab: ReportsTab;
  dateFrom: string;
  dateTo: string;
  cashflowData: CashflowRow[];
  netWorthData: NetWorthRow[];
  accountTrendData: AccountTrendRow[];
  categorySpendData: CategorySpendRow[];
  payeeTotalsData: PayeeTotalRow[];
  realizedGains: RealizedGainEntry[];
  unrealizedGains: UnrealizedGainEntry[];
  performanceSummary: PortfolioPerformanceSummary | null;
  accountValuationRows: AccountValuationRow[];
  currencyExposureRows: CurrencyExposureRow[];
  reportDefinitions: ReportDefinitionSummary[];
  reportRuns: ReportRunSummary[];
};

export type CsvExport = {
  filename: string;
  content: string;
};

function escapeCsvValue(value: CsvValue): string {
  if (value === null || value === undefined) {
    return "";
  }
  const text = String(value);
  if (/[",\n]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`;
  }
  return text;
}

export function buildCsv(headers: string[], rows: CsvValue[][]): string {
  return [headers, ...rows].map((row) => row.map(escapeCsvValue).join(",")).join("\n");
}

function fileSuffix(dateTo: string): string {
  return dateTo || new Date().toISOString().split("T")[0];
}

export function buildReportCsv(state: ReportsExportState): CsvExport | null {
  const suffix = fileSuffix(state.dateTo);

  if (state.activeTab === "cashflow") {
    return {
      filename: `cashflow-${suffix}.csv`,
      content: buildCsv(
        ["period_start", "inflow_minor", "outflow_minor", "net_minor"],
        state.cashflowData.map((row) => [row.period_start, row.inflow_minor, row.outflow_minor, row.net_minor])
      ),
    };
  }

  if (state.activeTab === "net-worth") {
    return {
      filename: `net-worth-${suffix}.csv`,
      content: buildCsv(
        ["period_start", "assets_minor", "liabilities_minor", "net_worth_minor"],
        state.netWorthData.map((row) => [row.period_start, row.assets_minor, row.liabilities_minor, row.net_worth_minor])
      ),
    };
  }

  if (state.activeTab === "account-trends") {
    return {
      filename: `account-trends-${suffix}.csv`,
      content: buildCsv(
        ["period_start", "account_id", "account_name", "balance_minor"],
        state.accountTrendData.map((row) => [row.period_start, row.account_id, row.account_name, row.balance_minor])
      ),
    };
  }

  if (state.activeTab === "spending") {
    return {
      filename: `category-spend-${suffix}.csv`,
      content: buildCsv(
        ["category_id", "category_name", "total_minor"],
        state.categorySpendData.map((row) => [row.category_id, row.category_name, row.total_minor])
      ),
    };
  }

  if (state.activeTab === "payees") {
    return {
      filename: `payee-totals-${suffix}.csv`,
      content: buildCsv(
        ["payee_id", "payee_name", "total_minor"],
        state.payeeTotalsData.map((row) => [row.payee_id, row.payee_name, row.total_minor])
      ),
    };
  }

  if (state.activeTab === "gains") {
    return {
      filename: `investment-gains-${suffix}.csv`,
      content: buildCsv(
        [
          "section",
          "account_name",
          "txn_date",
          "commodity_name",
          "quantity_minor",
          "proceeds_minor",
          "value_minor",
          "cost_basis_minor",
          "gain_loss_minor",
          "missing",
        ],
        [
          ...state.realizedGains.map((row) => [
            "realized",
            "",
            row.txn_date,
            row.commodity_id,
            row.quantity_minor,
            row.proceeds_minor,
            "",
            row.cost_basis_minor,
            row.gain_loss_minor,
            row.proceeds_missing,
          ]),
          ...state.unrealizedGains.map((row) => [
            "unrealized",
            row.account_name,
            "",
            row.commodity_name,
            "",
            "",
            row.value_minor,
            row.cost_basis_minor,
            row.unrealized_gain_minor,
            row.price_missing,
          ]),
        ]
      ),
    };
  }

  if (state.activeTab === "performance") {
    if (!state.performanceSummary) {
      return null;
    }
    return {
      filename: `portfolio-performance-${suffix}.csv`,
      content: buildCsv(
        [
          "book_id",
          "base_commodity_id",
          "as_of_date",
          "market_value_minor",
          "cost_basis_minor",
          "unrealized_gain_minor",
          "realized_gain_minor",
          "income_minor",
          "price_missing_count",
        ],
        [[
          state.performanceSummary.book_id,
          state.performanceSummary.base_commodity_id,
          state.performanceSummary.as_of_date,
          state.performanceSummary.market_value_minor,
          state.performanceSummary.cost_basis_minor,
          state.performanceSummary.unrealized_gain_minor,
          state.performanceSummary.realized_gain_minor,
          state.performanceSummary.income_minor,
          state.performanceSummary.price_missing_count,
        ]]
      ),
    };
  }

  if (state.activeTab === "valuation") {
    return {
      filename: `account-valuation-${suffix}.csv`,
      content: buildCsv(
        [
          "account_id",
          "account_name",
          "market_value_minor",
          "cost_basis_minor",
          "unrealized_gain_minor",
          "price_missing",
          "fx_missing",
        ],
        state.accountValuationRows.map((row) => [
          row.account_id,
          row.account_name,
          row.market_value_minor,
          row.cost_basis_minor,
          row.unrealized_gain_minor,
          row.price_missing,
          row.fx_missing,
        ])
      ),
    };
  }

  if (state.activeTab === "exposure") {
    return {
      filename: `currency-exposure-${suffix}.csv`,
      content: buildCsv(
        ["commodity_id", "commodity_name", "quantity_minor", "native_value_minor", "price_missing"],
        state.currencyExposureRows.map((row) => [
          row.commodity_id,
          row.commodity_name,
          row.quantity_minor,
          row.native_value_minor,
          row.price_missing,
        ])
      ),
    };
  }

  if (state.activeTab === "saved") {
    return {
      filename: `saved-reports-${suffix}.csv`,
      content: buildCsv(
        ["section", "id", "name_or_definition_id", "kind_or_pricing_mode", "created_at"],
        [
          ...state.reportDefinitions.map((row) => ["definition", row.id, row.name, row.kind, row.created_at]),
          ...state.reportRuns.map((row) => ["run", row.id, row.definition_id, row.pricing_mode, row.created_at]),
        ]
      ),
    };
  }

  return null;
}

export function hasExportableReportData(state: ReportsExportState): boolean {
  const csv = buildReportCsv(state);
  if (!csv) {
    return false;
  }
  return csv.content.trim().includes("\n");
}

export function saveCsv(filename: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: "text/csv;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}