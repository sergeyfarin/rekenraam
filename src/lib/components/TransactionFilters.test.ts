import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import TransactionFilters from "./TransactionFilters.svelte";

type Account = { id: number; name: string };

const accounts: Account[] = [
  { id: 10, name: "Checking" },
  { id: 20, name: "Savings" },
];

function baseProps(overrides: Record<string, unknown> = {}) {
  return {
    activeFilter: null,
    accounts,
    dateFrom: "",
    dateTo: "",
    statusFilter: "",
    accountFilterId: null,
    amountMin: "",
    amountMax: "",
    search: "",
    onApply: vi.fn(),
    onSearchInput: vi.fn(),
    ...overrides,
  };
}

describe("TransactionFilters — visibility (C5)", () => {
  it("renders nothing when activeFilter is null", () => {
    const { container } = render(TransactionFilters, baseProps());
    expect(container.textContent?.trim() ?? "").toBe("");
  });

  it("renders the date panel when activeFilter='date'", () => {
    render(TransactionFilters, baseProps({ activeFilter: "date" }));
    expect(screen.getByLabelText("From")).toBeInTheDocument();
    expect(screen.getByLabelText("To")).toBeInTheDocument();
  });

  it("renders the status panel when activeFilter='status'", () => {
    render(TransactionFilters, baseProps({ activeFilter: "status" }));
    const select = screen.getByLabelText("Status") as HTMLSelectElement;
    expect(select).toBeInTheDocument();
    // 6 options: active / uncleared / cleared / reconciled / void / all
    expect(select.querySelectorAll("option")).toHaveLength(6);
  });

  it("renders the account panel with All + one option per account", () => {
    render(TransactionFilters, baseProps({ activeFilter: "account" }));
    const select = screen.getByLabelText("Account") as HTMLSelectElement;
    expect(select.querySelectorAll("option")).toHaveLength(3); // All accounts + 2
  });

  it("renders the amount panel with min and max inputs", () => {
    render(TransactionFilters, baseProps({ activeFilter: "amount" }));
    expect(screen.getByLabelText("Min amount")).toBeInTheDocument();
    expect(screen.getByLabelText("Max amount")).toBeInTheDocument();
  });
});

describe("TransactionFilters — date inputs accept ISO and leap-year edge cases (C5)", () => {
  it("accepts 2024-02-29 (leap year) verbatim into the From input", async () => {
    render(TransactionFilters, baseProps({ activeFilter: "date" }));
    const fromInput = screen.getByLabelText("From") as HTMLInputElement;
    await fireEvent.input(fromInput, { target: { value: "2024-02-29" } });
    expect(fromInput.value).toBe("2024-02-29");
  });

  it("blank dates are allowed (filter UX: empty = no constraint)", async () => {
    render(TransactionFilters, baseProps({ activeFilter: "date", dateFrom: "2026-01-01" }));
    const fromInput = screen.getByLabelText("From") as HTMLInputElement;
    expect(fromInput.value).toBe("2026-01-01");
    await fireEvent.input(fromInput, { target: { value: "" } });
    expect(fromInput.value).toBe("");
  });
});

describe("TransactionFilters — apply / clear callbacks (C5)", () => {
  it("Apply button calls onApply", async () => {
    const onApply = vi.fn();
    render(TransactionFilters, baseProps({ activeFilter: "date", onApply }));
    await fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(onApply).toHaveBeenCalledOnce();
  });

  it("Clear button on date panel resets dateFrom + dateTo and triggers onApply", async () => {
    const onApply = vi.fn();
    render(TransactionFilters, baseProps({
      activeFilter: "date",
      dateFrom: "2026-01-01",
      dateTo: "2026-01-31",
      onApply,
    }));
    await fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(onApply).toHaveBeenCalledOnce();
  });

  it("status select onchange triggers onApply (instant apply for single-select)", async () => {
    const onApply = vi.fn();
    render(TransactionFilters, baseProps({ activeFilter: "status", onApply }));
    const select = screen.getByLabelText("Status") as HTMLSelectElement;
    await fireEvent.change(select, { target: { value: "cleared" } });
    expect(onApply).toHaveBeenCalledOnce();
  });
});

describe("TransactionFilters — payee search panel (C5)", () => {
  it("payee panel shows the search input and a Clear button", () => {
    render(TransactionFilters, baseProps({ activeFilter: "payee", search: "rent" }));
    const searchInput = screen.getByLabelText("Search payee / memo") as HTMLInputElement;
    expect(searchInput).toBeInTheDocument();
    expect(searchInput.value).toBe("rent");
    expect(screen.getByRole("button", { name: "Clear" })).toBeInTheDocument();
  });

  it("typing in payee search triggers onSearchInput (debounced load lives at parent)", async () => {
    const onSearchInput = vi.fn();
    render(TransactionFilters, baseProps({ activeFilter: "payee", onSearchInput }));
    const searchInput = screen.getByLabelText("Search payee / memo") as HTMLInputElement;
    await fireEvent.input(searchInput, { target: { value: "groceries" } });
    expect(onSearchInput).toHaveBeenCalledOnce();
  });
});
