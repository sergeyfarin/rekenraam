import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";

// Mock all planning + lookup API modules before importing the page.
vi.mock("$lib/api/accounts", () => ({
  listAccounts: vi.fn(),
}));
vi.mock("$lib/api/metadata", () => ({
  listCategories: vi.fn(),
}));
vi.mock("$lib/api/planning", () => ({
  budgetVariance: vi.fn(),
  createBudget: vi.fn(),
  createLoan: vi.fn(),
  createSchedule: vi.fn(),
  listBudgets: vi.fn(),
  listLoans: vi.fn(),
  listSchedules: vi.fn(),
  loanPaymentDraft: vi.fn(),
  loanAmortization: vi.fn(),
  postLoanPayment: vi.fn(),
  postSchedule: vi.fn(),
  projectedCash: vi.fn(),
  projectedInstances: vi.fn(),
  skipSchedule: vi.fn(),
}));

import { listAccounts } from "$lib/api/accounts";
import { listCategories } from "$lib/api/metadata";
import {
  budgetVariance,
  createBudget,
  createLoan,
  createSchedule,
  listBudgets,
  listLoans,
  listSchedules,
  loanPaymentDraft,
  loanAmortization,
  postLoanPayment,
  projectedCash,
  projectedInstances,
} from "$lib/api/planning";
import { activeBookId, bookContext } from "$lib/book-context";
import PlanningPage from "./+page.svelte";

function mockAccount(id: number, name: string, accountType = "asset") {
  return {
    id,
    book_id: 1,
    parent_id: null,
    account_type: accountType,
    name,
    commodity_id: 1,
    institution_id: null,
    institution_name: null,
    country_id: null,
    country_name: null,
    number_last4: null,
    is_closed: false,
    is_hidden: false,
    is_system: false,
    system_role: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function mockCategory(id: number, name: string) {
  return {
    id,
    book_id: 1,
    parent_id: null,
    name,
    kind: "expense",
    color: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

beforeEach(() => {
  bookContext.set({
    books: [{ id: 1, slug: "personal", name: "Personal", base_currency_code: "USD" }],
    activeBook: { id: 1, slug: "personal", name: "Personal", base_currency_code: "USD" },
    loading: false,
    error: null,
  });
  activeBookId.set(1);
  vi.mocked(listAccounts).mockResolvedValue([
    mockAccount(2, "Checking"),
    mockAccount(3, "Savings"),
    mockAccount(4, "Mortgage", "liability"),
  ]);
  vi.mocked(listCategories).mockResolvedValue([
    mockCategory(11, "Groceries"),
    mockCategory(12, "Utilities"),
  ]);
  vi.mocked(listBudgets).mockResolvedValue([]);
  vi.mocked(listSchedules).mockResolvedValue([]);
  vi.mocked(listLoans).mockResolvedValue([]);
  vi.mocked(projectedInstances).mockResolvedValue([]);
  vi.mocked(projectedCash).mockResolvedValue([]);
  vi.mocked(budgetVariance).mockResolvedValue([]);
  vi.mocked(loanAmortization).mockResolvedValue([]);
  vi.mocked(loanPaymentDraft).mockResolvedValue({
    transaction: {
      book_id: 1,
      txn_date: "2026-05-18",
      payee_id: null,
      memo: null,
      status: "uncleared",
      reference: null,
      import_id: null,
      splits: [],
    },
  });
  vi.mocked(postLoanPayment).mockResolvedValue({
    id: 123,
    book_id: 1,
    previous_tx_id: null,
    occurred_date: "2026-05-18",
    posted_date: "2026-05-18",
    payee_id: null,
    memo: null,
    status: "uncleared",
    reference: null,
    import_id: null,
    created_at: "2026-05-18T00:00:00Z",
    splits: [],
  });
});

afterEach(() => {
  bookContext.set({ books: [], activeBook: null, loading: false, error: null });
  activeBookId.set(null);
  vi.resetAllMocks();
});

async function renderAndWait() {
  render(PlanningPage);
  await waitFor(() => expect(screen.getByText(/Budget Targets/)).toBeInTheDocument());
}

describe("Planning — Budget form (C4a)", () => {
  it("submits a valid budget with the resolved category and amount", async () => {
    vi.mocked(createBudget).mockResolvedValue({
      id: 1,
      book_id: 1,
      name: "Monthly budget",
      period: "monthly",
      starts_on: "2026-05-01",
      ends_on: null,
      commodity_id: 1,
      is_active: true,
      targets: [],
      created_at: "2026-05-15T00:00:00Z",
    } as unknown as Awaited<ReturnType<typeof createBudget>>);

    await renderAndWait();
    await fireEvent.click(screen.getByRole("button", { name: "Add Target" }));

    await waitFor(() => expect(vi.mocked(createBudget)).toHaveBeenCalledOnce());
    const payload = vi.mocked(createBudget).mock.calls[0][0];
    expect(payload.name).toBe("Monthly budget");
    expect(payload.targets[0].amount_minor).toBe(50000);
    expect(payload.targets[0].category_id).toBe(11);
  });

  it("blocks submit and surfaces error when budget name is whitespace", async () => {
    await renderAndWait();
    const nameInput = screen.getByLabelText("Budget name") as HTMLInputElement;
    await fireEvent.input(nameInput, { target: { value: "   " } });
    await fireEvent.click(screen.getByRole("button", { name: "Add Target" }));

    expect(vi.mocked(createBudget)).not.toHaveBeenCalled();
    expect(await screen.findByText(/Budget name is required/)).toBeInTheDocument();
  });

  it("blocks submit and surfaces error when budget amount is zero or negative", async () => {
    await renderAndWait();
    const amountInput = screen.getByLabelText("Target amount minor") as HTMLInputElement;
    await fireEvent.input(amountInput, { target: { value: "0" } });
    await fireEvent.click(screen.getByRole("button", { name: "Add Target" }));

    expect(vi.mocked(createBudget)).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Target amount must be at least 1/),
    ).toBeInTheDocument();
  });

  it("clears the error message when a subsequent valid submit succeeds", async () => {
    vi.mocked(createBudget).mockResolvedValue({
      id: 1, book_id: 1, name: "x", period: "monthly", starts_on: "2026-05-01",
      ends_on: null, commodity_id: 1, is_active: true, targets: [],
      created_at: "2026-05-15T00:00:00Z",
    } as unknown as Awaited<ReturnType<typeof createBudget>>);
    await renderAndWait();
    const amountInput = screen.getByLabelText("Target amount minor") as HTMLInputElement;
    await fireEvent.input(amountInput, { target: { value: "0" } });
    await fireEvent.click(screen.getByRole("button", { name: "Add Target" }));
    expect(
      await screen.findByText(/Target amount must be at least 1/),
    ).toBeInTheDocument();

    await fireEvent.input(amountInput, { target: { value: "75000" } });
    await fireEvent.click(screen.getByRole("button", { name: "Add Target" }));
    await waitFor(() => expect(vi.mocked(createBudget)).toHaveBeenCalledOnce());
    await waitFor(() => {
      expect(screen.queryByText(/Target amount must be at least 1/)).not.toBeInTheDocument();
    });
  });
});

describe("Planning — Schedule form (C4b)", () => {
  beforeEach(() => {
    vi.mocked(createSchedule).mockResolvedValue({
      id: 99,
      book_id: 1,
      name: "Recurring transfer",
      frequency: "monthly",
      start_date: "2026-05-15",
    } as unknown as Awaited<ReturnType<typeof createSchedule>>);
  });

  async function openScheduleTab() {
    await renderAndWait();
    await fireEvent.click(screen.getByRole("tab", { name: "Schedules" }));
  }

  it("submits a schedule with debit + credit splits matching the entered amount", async () => {
    await openScheduleTab();
    await fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(vi.mocked(createSchedule)).toHaveBeenCalledOnce());
    const payload = vi.mocked(createSchedule).mock.calls[0][0];
    expect(payload.frequency).toBe("monthly");
    expect(payload.splits).toHaveLength(2);
    expect(payload.splits[0].amount_minor).toBe(-10000);
    expect(payload.splits[1].amount_minor).toBe(10000);
  });

  it("blocks submit when schedule amount is zero", async () => {
    await openScheduleTab();
    const amountInput = screen.getByLabelText("Amount minor") as HTMLInputElement;
    await fireEvent.input(amountInput, { target: { value: "0" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create" }));

    expect(vi.mocked(createSchedule)).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Schedule amount must be at least 1/),
    ).toBeInTheDocument();
  });

  it("blocks submit when schedule name is empty", async () => {
    await openScheduleTab();
    const nameInput = screen.getByLabelText("Schedule name") as HTMLInputElement;
    await fireEvent.input(nameInput, { target: { value: "" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create" }));

    expect(vi.mocked(createSchedule)).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Schedule name is required/),
    ).toBeInTheDocument();
  });

  it("propagates the selected frequency to the API payload", async () => {
    await openScheduleTab();
    const frequencySelect = screen.getAllByRole("combobox").find((el) => {
      return (el as HTMLSelectElement).value === "monthly";
    }) as HTMLSelectElement;
    expect(frequencySelect).toBeDefined();
    await fireEvent.change(frequencySelect, { target: { value: "weekly" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(vi.mocked(createSchedule)).toHaveBeenCalledOnce());
    expect(vi.mocked(createSchedule).mock.calls[0][0].frequency).toBe("weekly");
  });
});

describe("Planning — Loan form (C4c)", () => {
  beforeEach(() => {
    vi.mocked(createLoan).mockResolvedValue({
      id: 42,
      book_id: 1,
      name: "Mortgage",
      principal_minor: 25000000,
      annual_rate_bps: 650,
      term_months: 360,
    } as unknown as Awaited<ReturnType<typeof createLoan>>);
  });

  async function openLoansTab() {
    await renderAndWait();
    await fireEvent.click(screen.getByRole("tab", { name: "Loans" }));
  }

  it("submits a loan with the provided principal, rate, and term", async () => {
    await openLoansTab();
    await fireEvent.click(screen.getByRole("button", { name: "Create Loan" }));

    await waitFor(() => expect(vi.mocked(createLoan)).toHaveBeenCalledOnce());
    const payload = vi.mocked(createLoan).mock.calls[0][0];
    expect(payload.principal_minor).toBe(25000000);
    expect(payload.annual_rate_bps).toBe(650);
    expect(payload.term_months).toBe(360);
  });

  it("blocks submit when principal is zero", async () => {
    await openLoansTab();
    const principal = screen.getByLabelText("Principal minor") as HTMLInputElement;
    await fireEvent.input(principal, { target: { value: "0" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create Loan" }));

    expect(vi.mocked(createLoan)).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Principal must be at least 1/),
    ).toBeInTheDocument();
  });

  it("blocks submit when term exceeds the 600-month cap", async () => {
    await openLoansTab();
    const term = screen.getByLabelText("Term months") as HTMLInputElement;
    await fireEvent.input(term, { target: { value: "601" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create Loan" }));

    expect(vi.mocked(createLoan)).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Term \(months\) must be at most 600/),
    ).toBeInTheDocument();
  });

  it("blocks submit when rate (bps) is negative", async () => {
    await openLoansTab();
    const rate = screen.getByLabelText("Rate basis points") as HTMLInputElement;
    await fireEvent.input(rate, { target: { value: "-1" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create Loan" }));

    expect(vi.mocked(createLoan)).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/Rate \(bps\) must be at least 0/),
    ).toBeInTheDocument();
  });
});
