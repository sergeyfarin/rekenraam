import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiGet, apiPost, apiPut } from "$lib/api/client";
import {
  convertPositions,
  createDividendTransaction,
  getPositions,
  listInvestmentInstruments,
  listLotsWithHoldingPeriod,
  saveInvestmentInstrument,
} from "./investments";

vi.mock("$lib/api/client", () => ({
  apiGet: vi.fn(),
  apiPost: vi.fn(),
  apiPut: vi.fn(),
}));

describe("investments API client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiGet).mockResolvedValue([]);
    vi.mocked(apiPost).mockResolvedValue({});
    vi.mocked(apiPut).mockResolvedValue({});
  });

  it("includes book and as-of date in positions queries", async () => {
    await getPositions(42, "2026-05-18");
    await convertPositions(42, 7, "2026-05-18");
    await listLotsWithHoldingPeriod(42, null, 9, "2026-05-18");

    expect(apiGet).toHaveBeenNthCalledWith(1, "/investments/positions?book_id=42&as_of_date=2026-05-18");
    expect(apiGet).toHaveBeenNthCalledWith(2, "/investments/positions/converted?book_id=42&base_commodity_id=7&as_of_date=2026-05-18");
    expect(apiGet).toHaveBeenNthCalledWith(3, "/investments/lots?book_id=42&commodity_id=9&as_of_date=2026-05-18");
  });

  it("uses typed instrument endpoints for list and update", async () => {
    await listInvestmentInstruments(42);
    await saveInvestmentInstrument({
      id: 5,
      book_id: 42,
      commodity_id: 10,
      instrument_type: "stock",
      display_name: "ACME Corp",
      symbol: "ACME",
      isin: null,
      cusip: null,
      figi: null,
      exchange: null,
      venue: null,
      issuer: null,
      country_code: null,
      quote_commodity_id: 1,
      trading_commodity_id: null,
      quantity_scale: 4,
      price_scale: 4,
      is_active: true,
      metadata_json: null,
    });

    expect(apiGet).toHaveBeenCalledWith("/investments/instruments?book_id=42");
    expect(apiPut).toHaveBeenCalledWith("/investments/instruments/5", expect.objectContaining({ id: 5, book_id: 42 }));
  });

  it("posts dividend payloads without local reshaping", async () => {
    await createDividendTransaction({
      book_id: 42,
      txn_date: "2026-05-18",
      cash_account_id: 11,
      income_account_id: 12,
      amount_minor: 2550,
      memo: null,
      payee_id: null,
      status: null,
    });

    expect(apiPost).toHaveBeenCalledWith("/investments/dividend", {
      book_id: 42,
      txn_date: "2026-05-18",
      cash_account_id: 11,
      income_account_id: 12,
      amount_minor: 2550,
      memo: null,
      payee_id: null,
      status: null,
    });
  });
});