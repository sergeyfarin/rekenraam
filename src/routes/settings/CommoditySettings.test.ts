import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import CommoditySettings from "./CommoditySettings.svelte";
import { requireActiveBookId } from "$lib/book-context";
import { hasApiBaseUrl } from "$lib/api/client";
import {
  getFxRefreshExecutionStatus,
  getFxRateSettings,
  listCurrencies,
  listFxRefreshRunHistory,
  listFxRateRefreshState,
  listFxRateSourceAssignments,
  listFxRateSources,
  refreshFxRatesNow,
} from "$lib/api/commodities";
import { listCommodities } from "$lib/api/metadata";

vi.mock("$lib/book-context", () => ({
  requireActiveBookId: vi.fn(() => 42),
}));

vi.mock("$lib/api/client", () => ({
  ApiError: class ApiError extends Error {},
  hasApiBaseUrl: vi.fn(() => true),
}));

vi.mock("$lib/api/metadata", () => ({
  listCommodities: vi.fn(),
}));

vi.mock("$lib/api/commodities", () => ({
  createFxRateDaily: vi.fn(),
  createFxRateOfficial: vi.fn(),
  deleteFxRateDaily: vi.fn(),
  deleteFxRateOfficial: vi.fn(),
  deleteFxRateSourceAssignment: vi.fn(),
  getFxRefreshExecutionStatus: vi.fn(),
  getFxRateSettings: vi.fn(),
  listCurrencies: vi.fn(),
  listFxRefreshRunHistory: vi.fn(),
  listFxRateRefreshState: vi.fn(),
  listFxRateSourceAssignments: vi.fn(),
  listFxRateSources: vi.fn(),
  listFxRatesDaily: vi.fn(),
  listFxRatesOfficial: vi.fn(),
  refreshFxRatesNow: vi.fn(),
  renameCommoditySymbol: vi.fn(),
  restartFxRateScheduler: vi.fn(),
  saveCurrency: vi.fn(),
  saveFxRateSourceAssignment: vi.fn(),
  setDefaultCurrency: vi.fn(),
  setFxRateSettings: vi.fn(),
  toggleCurrencyActive: vi.fn(),
}));

const currenciesFixture = [
  {
    id: 1,
    book_id: 42,
    symbol: "EUR",
    display_symbol: "€",
    name: "Euro",
    scale: 2,
    is_active: true,
    is_default: true,
    created_at: "2026-01-01",
    updated_at: "2026-01-01",
  },
  {
    id: 2,
    book_id: 42,
    symbol: "USD",
    display_symbol: "$",
    name: "US Dollar",
    scale: 2,
    is_active: true,
    is_default: false,
    created_at: "2026-01-01",
    updated_at: "2026-01-01",
  },
];

const commoditiesFixture = [
  {
    id: 10,
    book_id: 42,
    kind: "currency",
    symbol: "EUR",
    name: "Euro",
    scale: 2,
    metadata: null,
    created_at: "2026-01-01",
    updated_at: "2026-01-01",
  },
  {
    id: 11,
    book_id: 42,
    kind: "stock",
    symbol: "ACME",
    name: "ACME Corp",
    scale: 4,
    metadata: null,
    created_at: "2026-01-01",
    updated_at: "2026-01-01",
  },
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(requireActiveBookId).mockReturnValue(42);
  vi.mocked(hasApiBaseUrl).mockReturnValue(true);
  vi.mocked(listCommodities).mockResolvedValue(commoditiesFixture);
  vi.mocked(listCurrencies).mockResolvedValue(currenciesFixture);
  vi.mocked(listFxRateSources).mockResolvedValue([
    {
      id: 9,
      book_id: 42,
      name: "ECB",
      country_code: "EU",
      website_url: null,
      notes: null,
      created_at: "2026-01-01",
      updated_at: "2026-01-01",
    },
  ]);
  vi.mocked(getFxRateSettings).mockResolvedValue({
    book_id: 42,
    base_currency_id: 1,
    base_currency_symbol: "EUR",
    default_source_id: 9,
    default_source_name: "ECB",
    refresh_enabled: true,
    refresh_hour_utc: 2,
    refresh_minute_utc: 0,
    max_backfill_days: 7,
    weekend_policy: "skip",
    created_at: "2026-01-01",
    updated_at: "2026-01-01",
  });
  vi.mocked(listFxRateSourceAssignments).mockResolvedValue([]);
  vi.mocked(listFxRateRefreshState).mockResolvedValue([]);
  vi.mocked(getFxRefreshExecutionStatus).mockResolvedValue({
    book_id: 42,
    scheduler_enabled: true,
    scheduler_poll_seconds: 60,
    worker_started_at: "2026-01-01T00:00:00Z",
    is_running: false,
    active_book_ids: [42],
    next_scheduled_at: "2026-01-02T00:00:00Z",
    last_run: null,
  });
  vi.mocked(listFxRefreshRunHistory).mockResolvedValue([]);
  vi.mocked(refreshFxRatesNow).mockResolvedValue({
    book_id: 42,
    trigger: "manual",
    started_at: "2026-01-01T00:00:00Z",
    finished_at: "2026-01-01T00:01:00Z",
    pairs_total: 2,
    pairs_success: 2,
    pairs_failed: 0,
    rates_inserted: 6,
    derived_inserted: 0,
    last_error: null,
  });
  vi.stubGlobal("confirm", vi.fn(() => true));
});

describe("CommoditySettings", () => {
  it("loads commodities and currencies for the active book", async () => {
    render(CommoditySettings, { commodities: [], busy: false });

    await waitFor(() => {
      expect(listCommodities).toHaveBeenCalledWith(42);
      expect(listCurrencies).toHaveBeenCalledWith(42);
    });

    expect(screen.getByText("EUR")).toBeInTheDocument();
  });

  it("loads FX settings data when the FX settings tab is opened", async () => {
    render(CommoditySettings, { commodities: [], busy: false });

    await fireEvent.click(screen.getByRole("button", { name: "FX Settings" }));

    await waitFor(() => {
      expect(listFxRateSources).toHaveBeenCalledWith(42);
      expect(getFxRateSettings).toHaveBeenCalledWith(42);
      expect(listFxRateSourceAssignments).toHaveBeenCalledWith(42);
      expect(listFxRateRefreshState).toHaveBeenCalledWith(42);
      expect(getFxRefreshExecutionStatus).toHaveBeenCalledWith(42);
      expect(listFxRefreshRunHistory).toHaveBeenCalledWith(42, 10);
    });

    expect(screen.getByText("Worker Status")).toBeInTheDocument();
  });

  it("passes the active book id to manual FX refresh", async () => {
    render(CommoditySettings, { commodities: [], busy: false });

    await fireEvent.click(screen.getByRole("button", { name: "FX Settings" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Run Refresh Now" })).toBeInTheDocument());
    await fireEvent.click(screen.getByRole("button", { name: "Run Refresh Now" }));

    await waitFor(() => {
      expect(refreshFxRatesNow).toHaveBeenCalledWith(42);
      expect(screen.getByText("FX refresh complete. 2/2 pairs updated, 6 rates.")).toBeInTheDocument();
    });
  });

  it("formats network errors for user-facing load failures", async () => {
    vi.mocked(listCurrencies).mockRejectedValueOnce(new TypeError("Failed to fetch"));

    render(CommoditySettings, { commodities: [], busy: false });

    await waitFor(() => {
      expect(screen.getByText("Failed to load currencies: Backend unavailable. Check the API service and try again.")).toBeInTheDocument();
    });
  });
});