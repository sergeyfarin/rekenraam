import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { buildCsv, buildReportCsv, hasExportableReportData, saveCsv } from "./export";
import type { ReportsExportState } from "./export";

function baseState(): ReportsExportState {
  return {
    activeTab: "cashflow",
    dateFrom: "2026-01-01",
    dateTo: "2026-05-18",
    cashflowData: [{ period_start: "2026-05-01", inflow_minor: 100, outflow_minor: 25, net_minor: 75 }],
    netWorthData: [],
    accountTrendData: [],
    categorySpendData: [],
    payeeTotalsData: [],
    realizedGains: [],
    unrealizedGains: [],
    performanceSummary: null,
    accountValuationRows: [],
    currencyExposureRows: [],
    reportDefinitions: [],
    reportRuns: [],
  };
}

describe("reports export helpers", () => {
  const originalCreateObjectUrl = URL.createObjectURL;
  const originalRevokeObjectUrl = URL.revokeObjectURL;

  beforeEach(() => {
    URL.createObjectURL = vi.fn(() => "blob:test");
    URL.revokeObjectURL = vi.fn();
  });

  afterEach(() => {
    URL.createObjectURL = originalCreateObjectUrl;
    URL.revokeObjectURL = originalRevokeObjectUrl;
    vi.restoreAllMocks();
  });

  it("escapes csv cells that contain commas or quotes", () => {
    expect(buildCsv(["name"], [["A, \"quoted\" value"]])).toBe('name\n"A, ""quoted"" value"');
  });

  it("builds a cashflow csv for the active report tab", () => {
    expect(buildReportCsv(baseState())).toEqual({
      filename: "cashflow-2026-05-18.csv",
      content: "period_start,inflow_minor,outflow_minor,net_minor\n2026-05-01,100,25,75",
    });
    expect(hasExportableReportData(baseState())).toBe(true);
  });

  it("downloads csv content through the browser url api", () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    saveCsv("cashflow.csv", "a,b\n1,2");

    expect(URL.createObjectURL).toHaveBeenCalledOnce();
    expect(click).toHaveBeenCalledOnce();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:test");
  });
});