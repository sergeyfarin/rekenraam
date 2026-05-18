import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";

vi.mock("$lib/book-context", () => ({
  requireActiveBookId: vi.fn(() => 42),
}));

vi.mock("$lib/api/accounts", () => ({
  listAccounts: vi.fn(),
}));

vi.mock("$lib/api/metadata", () => ({
  listCommodities: vi.fn(),
}));

vi.mock("$lib/api/commodities", () => ({
  autocompleteCommodities: vi.fn(),
}));

vi.mock("$lib/api/investments", () => ({
  buyCommodity: vi.fn(),
  convertPositions: vi.fn(),
  createDividendTransaction: vi.fn(),
  getPositions: vi.fn(),
  listLotsWithHoldingPeriod: vi.fn(),
  sellCommodity: vi.fn(),
}));

import { listAccounts } from "$lib/api/accounts";
import { autocompleteCommodities } from "$lib/api/commodities";
import { convertPositions, getPositions, listLotsWithHoldingPeriod } from "$lib/api/investments";
import { listCommodities } from "$lib/api/metadata";
import InvestmentsPage from "./+page.svelte";

function mockAccount(id: number, name: string, accountType: string, commodityId = 1) {
  return {
    id,
    book_id: 42,
    parent_id: null,
    account_type: accountType,
    name,
    commodity_id: commodityId,
    institution_id: null,
    institution_name: null,
    country_id: null,
    country_name: null,
    number_last4: null,
    is_closed: false,
    is_hidden: false,
    is_system: false,
    system_role: null,
    created_at: "2026-05-18T00:00:00Z",
    updated_at: "2026-05-18T00:00:00Z",
  };
}

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
  vi.mocked(listAccounts).mockResolvedValue([
    mockAccount(10, "Brokerage", "investment"),
    mockAccount(11, "Checking", "checking"),
    mockAccount(12, "Dividend Income", "income"),
  ]);
  vi.mocked(listCommodities).mockResolvedValue([
    mockCommodity(1, "currency", "EUR", "Euro", 2),
    mockCommodity(2, "stock", "ACME", "ACME Corp", 4),
  ]);
  vi.mocked(getPositions).mockResolvedValue([
    {
      account_id: 10,
      account_name: "Brokerage",
      account_type: "investment",
      commodity_id: 2,
      commodity_name: "ACME Corp",
      commodity_scale: 4,
      balance_minor: 123456,
      lots: [
        {
          lot_id: 1,
          opened_date: "2026-01-02",
          quantity_minor: 123456,
          cost_basis_minor: 10000,
          remaining_cost_basis_minor: 10000,
          converted_value_minor: 12000,
          converted_cost_basis_minor: 10000,
          price_missing: false,
        },
      ],
    },
  ]);
  vi.mocked(convertPositions).mockResolvedValue([
    {
      account_id: 10,
      account_name: "Brokerage",
      account_type: "investment",
      commodity_id: 2,
      commodity_name: "ACME Corp",
      commodity_scale: 4,
      balance_minor: 123456,
      value_minor: 12000,
      price_missing: false,
      lots: [
        {
          lot_id: 1,
          opened_date: "2026-01-02",
          quantity_minor: 123456,
          cost_basis_minor: 10000,
          remaining_cost_basis_minor: 10000,
          converted_value_minor: 12000,
          converted_cost_basis_minor: 10000,
          price_missing: false,
        },
      ],
    },
  ]);
  vi.mocked(listLotsWithHoldingPeriod).mockResolvedValue([]);
  vi.mocked(autocompleteCommodities).mockResolvedValue([]);
});

afterEach(() => {
  vi.resetAllMocks();
});

describe("Investments page", () => {
  it("loads positions for the active book and current as-of date", async () => {
    const today = new Date().toISOString().split("T")[0];
    render(InvestmentsPage);

    await waitFor(() => {
      expect(getPositions).toHaveBeenCalledWith(42, today);
      expect(convertPositions).toHaveBeenCalledWith(42, 1, today);
    });

    expect(screen.getByText("Brokerage")).toBeInTheDocument();
    expect(screen.getByText("12.3456")).toBeInTheDocument();
  });

  it("opens the buy dialog with the focused trade fields", async () => {
    render(InvestmentsPage);
    await waitFor(() => expect(screen.getByRole("button", { name: "Buy" })).not.toBeDisabled());

    await fireEvent.click(screen.getByRole("button", { name: "Buy" }));
    expect(screen.getByLabelText("Investment Account")).toBeInTheDocument();
    expect(screen.getByLabelText("Cash Account")).toBeInTheDocument();
    expect(screen.getByLabelText("Quantity")).toBeInTheDocument();
    expect(screen.getByLabelText("Total Cost")).toBeInTheDocument();
  });

  it("shows the contract-aligned dividend fields only", async () => {
    render(InvestmentsPage);
    await waitFor(() => expect(screen.getByRole("button", { name: "Dividend" })).not.toBeDisabled());

    await fireEvent.click(screen.getByRole("button", { name: "Dividend" }));
    expect(screen.getByLabelText("Cash Account")).toBeInTheDocument();
    expect(screen.getByLabelText("Income Account")).toBeInTheDocument();
    expect(screen.getByLabelText("Amount")).toBeInTheDocument();
    expect(screen.queryByText(/Investment Account \(source\)/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Security \(optional\)/)).not.toBeInTheDocument();
  });

  it("formats network errors for user-facing failures", async () => {
    vi.mocked(listAccounts).mockRejectedValueOnce(new TypeError("Failed to fetch"));
    render(InvestmentsPage);

    await waitFor(() => {
      expect(screen.getByText("Failed to load accounts: Backend unavailable. Check the API service and try again.")).toBeInTheDocument();
    });
  });
});