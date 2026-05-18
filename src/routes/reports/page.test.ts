import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";

const createObjectURL = vi.fn(() => "blob:test");
const revokeObjectURL = vi.fn();

Object.defineProperty(URL, "createObjectURL", { value: createObjectURL, writable: true });
Object.defineProperty(URL, "revokeObjectURL", { value: revokeObjectURL, writable: true });

vi.mock("$lib/book-context", () => ({
  requireActiveBookId: vi.fn(() => 42),
}));

vi.mock("$lib/api/metadata", () => ({
  listCommodities: vi.fn(),
}));

vi.mock("$lib/api/reports", () => ({
  accountValuationReport: vi.fn(),
  createReportRun: vi.fn(),
  currencyExposureReport: vi.fn(),
  deleteReportDefinition: vi.fn(),
  investmentPerformanceReport: vi.fn(),
  listReportDefinitions: vi.fn(),
  listReportRuns: vi.fn(),
  realizedGainsReport: vi.fn(),
  reportAccountTrends: vi.fn(),
  reportCashflow: vi.fn(),
  reportCategorySpend: vi.fn(),
  reportNetWorth: vi.fn(),
  reportPayeeTotals: vi.fn(),
  saveReportDefinition: vi.fn(),
  unrealizedGainsReport: vi.fn(),
}));

import { listCommodities } from "$lib/api/metadata";
import {
  investmentPerformanceReport,
  listReportDefinitions,
  listReportRuns,
  reportCashflow,
  reportNetWorth,
} from "$lib/api/reports";
import ReportsPage from "./+page.svelte";

function mockCommodity(id: number, kind: string, symbol: string, name: string, scale: number) {
  return {
    id,
    book_id: 42,
    kind,
    symbol,
    name,
    scale,
    metadata: null,
    created_at: "2026-05-18T00:00:00Z",
    updated_at: "2026-05-18T00:00:00Z",
  };
}

beforeEach(() => {
  vi.mocked(listCommodities).mockResolvedValue([
    mockCommodity(1, "currency", "EUR", "Euro", 2),
    mockCommodity(2, "stock", "ACME", "ACME Corp", 4),
  ]);
  vi.mocked(reportCashflow).mockResolvedValue([
    {
      period_start: "2026-05-01",
      inflow_minor: 10000,
      outflow_minor: 2500,
      net_minor: 7500,
    },
  ]);
  vi.mocked(reportNetWorth).mockResolvedValue([
    {
      period_start: "2026-05-01",
      assets_minor: 200000,
      liabilities_minor: 50000,
      net_worth_minor: 150000,
    },
  ]);
  vi.mocked(investmentPerformanceReport).mockResolvedValue({
    book_id: 42,
    base_commodity_id: 1,
    as_of_date: "2026-05-18",
    market_value_minor: 120000,
    cost_basis_minor: 100000,
    unrealized_gain_minor: 20000,
    realized_gain_minor: 5000,
    income_minor: 1200,
    price_missing_count: 0,
  });
  vi.mocked(listReportDefinitions).mockResolvedValue([
    {
      id: 7,
      book_id: 42,
      name: "Saved Net Worth",
      kind: "custom",
      query_type: "sql",
      query_text: "select 1",
      params_schema: null,
      created_at: "2026-05-18T00:00:00Z",
    },
  ]);
  vi.mocked(listReportRuns).mockResolvedValue([
    {
      id: 3,
      book_id: 42,
      definition_id: 7,
      params_hash: "abc",
      as_of_seq: 9,
      pricing_mode: "latest_corrected",
      pricing_policy_id: null,
      pricing_policy_version: null,
      valuation_snapshot_id: null,
      pricing_resolved_at: null,
      result_json: "{}",
      created_at: "2026-05-18T12:00:00Z",
    },
  ]);
});

afterEach(() => {
  vi.resetAllMocks();
  createObjectURL.mockReset();
  revokeObjectURL.mockReset();
});

describe("Reports page", () => {
  it("loads cashflow for the active book on mount", async () => {
    const today = new Date().toISOString().split("T")[0];
    const yearStart = `${new Date().getFullYear()}-01-01`;
    render(ReportsPage);

    await waitFor(() => {
      expect(listCommodities).toHaveBeenCalledWith(42);
      expect(reportCashflow).toHaveBeenCalledWith({
        book_id: 42,
        date_from: yearStart,
        date_to: today,
        group_by: "month",
      });
    });

    expect(screen.getByText("Cash Flow Report")).toBeInTheDocument();
  });

  it("loads net worth with the typed report client when its tab opens", async () => {
    render(ReportsPage);

    await fireEvent.click(screen.getByRole("tab", { name: "Net Worth" }));

    await waitFor(() => {
      expect(reportNetWorth).toHaveBeenCalledWith(
        expect.objectContaining({ book_id: 42, group_by: "month" })
      );
    });

    expect(screen.getAllByText("Net Worth").length).toBeGreaterThan(0);
  });

  it("loads portfolio performance with the selected quote currency", async () => {
    const today = new Date().toISOString().split("T")[0];
    render(ReportsPage);

    await fireEvent.click(screen.getByRole("tab", { name: "Performance" }));

    await waitFor(() => {
      expect(investmentPerformanceReport).toHaveBeenCalledWith(42, 1, today);
    });

    expect(screen.getByText("Portfolio Performance")).toBeInTheDocument();
  });

  it("surfaces saved definitions and runs for the active book", async () => {
    render(ReportsPage);

    await fireEvent.click(screen.getByRole("tab", { name: "Saved Reports" }));

    await waitFor(() => {
      expect(listReportDefinitions).toHaveBeenCalledWith(42);
      expect(listReportRuns).toHaveBeenCalledWith(42);
    });

    expect(screen.getByText("Saved Net Worth")).toBeInTheDocument();
    expect(screen.getByText("Saved Runs")).toBeInTheDocument();
  });

  it("exports the active report as csv and opens print", async () => {
    const printSpy = vi.spyOn(window, "print").mockImplementation(() => {});
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    render(ReportsPage);

    await waitFor(() => expect(screen.getByRole("button", { name: "Export CSV" })).not.toBeDisabled());
    await fireEvent.click(screen.getByRole("button", { name: "Export CSV" }));
    await fireEvent.click(screen.getByRole("button", { name: "Print" }));

    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(clickSpy).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test");
    expect(printSpy).toHaveBeenCalledOnce();
    expect(screen.getAllByText(/cashflow-.*downloaded\./i).length).toBeGreaterThan(0);
  });
});