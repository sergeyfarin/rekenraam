import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";
import {
  createReportRun,
  currencyExposureReport,
  listReportDefinitions,
  listReportRuns,
  realizedGainsReport,
  reportCashflow,
  saveReportDefinition,
  unrealizedGainsReport,
} from "./reports";

vi.mock("$lib/api/client", () => ({
  apiDelete: vi.fn(),
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
}));

describe("reports API client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiGet).mockResolvedValue([]);
    vi.mocked(apiPost).mockResolvedValue({});
    vi.mocked(apiPut).mockResolvedValue({});
    vi.mocked(apiDelete).mockResolvedValue(undefined);
  });

  it("posts typed report payloads without generic reshaping", async () => {
    await reportCashflow({
      book_id: 42,
      date_from: "2026-01-01",
      date_to: "2026-05-18",
      group_by: "month",
    });

    expect(apiPost).toHaveBeenCalledWith("/reports/cashflow", {
      book_id: 42,
      date_from: "2026-01-01",
      date_to: "2026-05-18",
      group_by: "month",
    });
  });

  it("includes explicit book ids in report query endpoints", async () => {
    await realizedGainsReport(42, "2026-01-01", "2026-05-18", 7);
    await unrealizedGainsReport(42, 3, "2026-05-18", 7);
    await currencyExposureReport(42, "2026-05-18");
    await listReportRuns(42, 9);

    expect(apiGet).toHaveBeenNthCalledWith(
      1,
      "/reports/realized-gains?book_id=42&date_from=2026-01-01&date_to=2026-05-18&cost_basis_profile_id=7"
    );
    expect(apiGet).toHaveBeenNthCalledWith(
      2,
      "/reports/unrealized-gains?book_id=42&base_commodity_id=3&as_of_date=2026-05-18&cost_basis_profile_id=7"
    );
    expect(apiGet).toHaveBeenNthCalledWith(3, "/reports/currency-exposure?book_id=42&as_of_date=2026-05-18");
    expect(apiGet).toHaveBeenNthCalledWith(4, "/reports/runs?book_id=42&definition_id=9");
  });

  it("uses typed saved-report endpoints for list, create, update, run, and delete", async () => {
    await listReportDefinitions(42);
    await saveReportDefinition({
      book_id: 42,
      name: "Net worth snapshot",
      kind: "custom",
      query_type: "sql",
      query_text: "select 1",
      params_schema: null,
    });
    await saveReportDefinition({
      id: 5,
      book_id: 42,
      name: "Net worth snapshot",
      kind: "custom",
      query_type: "sql",
      query_text: "select 2",
      params_schema: null,
    });
    await createReportRun({
      book_id: 42,
      definition_id: 5,
      params_hash: "abc123",
      as_of_seq: 9,
      pricing_mode: "latest_corrected",
      pricing_policy_id: null,
      pricing_policy_version: null,
      valuation_snapshot_id: null,
      pricing_resolved_at: null,
      result_json: "{}",
    });

    expect(apiGet).toHaveBeenCalledWith("/reports/definitions?book_id=42");
    expect(apiPost).toHaveBeenNthCalledWith(1, "/reports/definitions", expect.objectContaining({ book_id: 42 }));
    expect(apiPut).toHaveBeenCalledWith("/reports/definitions/5", expect.objectContaining({ id: 5, book_id: 42 }));
    expect(apiPost).toHaveBeenNthCalledWith(2, "/reports/runs", expect.objectContaining({ definition_id: 5, book_id: 42 }));

    await import("./reports").then(({ deleteReportDefinition }) => deleteReportDefinition(5));
    expect(apiDelete).toHaveBeenCalledWith("/reports/definitions/5");
  });
});