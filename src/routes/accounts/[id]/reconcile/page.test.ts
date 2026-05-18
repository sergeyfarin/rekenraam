import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";

vi.mock("$lib/api/accounts", () => ({
  listAccounts: vi.fn(),
}));
vi.mock("$lib/api/metadata", () => ({
  listPayees: vi.fn(),
}));
vi.mock("$lib/api/reconciliation", () => ({
  finishReconciliation: vi.fn(),
  getReconciliationHistory: vi.fn(),
  startReconciliation: vi.fn(),
}));

import { listAccounts } from "$lib/api/accounts";
import { listPayees } from "$lib/api/metadata";
import {
  finishReconciliation,
  getReconciliationHistory,
  startReconciliation,
} from "$lib/api/reconciliation";
import { goto } from "$app/navigation";
import { page as pageState } from "$app/state";
import { activeBookId, bookContext } from "$lib/book-context";
import ReconcilePage from "./+page.svelte";

function mockAccount(id: number, name: string, commodity_id = 1) {
  return {
    id,
    book_id: 1,
    parent_id: null,
    account_type: "asset",
    name,
    commodity_id,
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

function mockTx(id: number, accountId: number, amountMinor: number, status = "uncleared") {
  return {
    transaction: {
      id,
      txn_date: "2026-05-10",
      payee_id: null,
      memo: null,
      status,
      reference: null,
    },
    splits: [
      {
        id: id * 10,
        tx_id: id,
        account_id: accountId,
        commodity_id: 1,
        amount_minor: amountMinor,
        category_id: null,
        tag_id: null,
        person_id: null,
        project_id: null,
        share_bps: null,
        memo: null,
      },
    ],
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
  pageState.params = { id: "100" };
  vi.mocked(goto).mockClear();

  vi.mocked(listAccounts).mockResolvedValue([
    mockAccount(100, "Checking"),
    mockAccount(200, "Offset"),
  ]);
  vi.mocked(listPayees).mockResolvedValue([]);
  vi.mocked(getReconciliationHistory).mockResolvedValue({
    balancings: [
      {
        id: 1,
        book_id: 1,
        account_id: 100,
        as_of_date: "2026-04-30",
        balance_minor: 100000,
        memo: null,
        created_at: "2026-04-30T00:00:00Z",
        voided_at: null,
        void_reason: null,
      },
    ],
    balance_checks: [],
    balance_adjustments: [],
  });
});

afterEach(() => {
  pageState.params = {};
  bookContext.set({ books: [], activeBook: null, loading: false, error: null });
  activeBookId.set(null);
  vi.resetAllMocks();
});

async function startWith(opts: { selectedClearedIds?: number[] } = {}) {
  vi.mocked(startReconciliation).mockResolvedValue({
    account: mockAccount(100, "Checking"),
    commodity: { id: 1, name: "USD", symbol: "$", scale: 2 },
    last_balancing: null,
    statement_start_date: "2026-05-01",
    statement_end_date: "2026-05-31",
    remembered_offset_account_id: null,
    candidates: [
      mockTx(1001, 100, 5000, opts.selectedClearedIds?.includes(1001) ? "cleared" : "uncleared"),
      mockTx(1002, 100, 3000, opts.selectedClearedIds?.includes(1002) ? "cleared" : "uncleared"),
      mockTx(1003, 100, -2000, opts.selectedClearedIds?.includes(1003) ? "cleared" : "uncleared"),
    ],
  });

  render(ReconcilePage);
  await waitFor(() => {
    expect(screen.getByLabelText("Statement ending balance")).toBeInTheDocument();
  });
}

describe("Reconcile — setup step (C2)", () => {
  it("renders the setup form with prior reconciliation summary", async () => {
    await startWith();
    expect(screen.getByText("Reconcile: Checking")).toBeInTheDocument();
    expect(screen.getByText(/Previous reconciliation/)).toBeInTheDocument();
    expect(screen.getByLabelText("Statement ending balance")).toBeInTheDocument();
  });

  it("Start button is disabled until a balance is entered", async () => {
    await startWith();
    const startBtn = screen.getByRole("button", { name: "Start" });
    expect(startBtn).toBeDisabled();
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1060.00" },
    });
    expect(startBtn).not.toBeDisabled();
  });

  it("submitting the setup form transitions to the working step", async () => {
    await startWith();
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1060.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => expect(vi.mocked(startReconciliation)).toHaveBeenCalledOnce());
    // Working step renders the Opening / Selected / Cleared / Difference grid
    await waitFor(() => {
      expect(screen.getByText("Opening")).toBeInTheDocument();
      expect(screen.getByText("Difference")).toBeInTheDocument();
    });
  });
});

describe("Reconcile — working step balance derivation (C2)", () => {
  async function startAndEnterWorking(balanceInput: string) {
    await startWith({ selectedClearedIds: [1001, 1002] }); // 5000 + 3000 = 8000 pre-checked
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: balanceInput },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.getByText("Opening")).toBeInTheDocument();
    });
  }

  it("difference = 0 (balanced) hides the offset-account dropdown", async () => {
    // opening 1000.00 + checked 80.00 = 1080.00 cleared, statement 1080.00 → diff 0
    await startAndEnterWorking("1080.00");
    expect(screen.queryByLabelText("Offset account")).not.toBeInTheDocument();
  });

  it("difference != 0 shows the offset-account dropdown with peer accounts", async () => {
    // opening 1000.00 + checked 80.00 = 1080.00, statement 1085.00 → diff 5.00
    await startAndEnterWorking("1085.00");
    await waitFor(() => {
      expect(screen.getByLabelText("Offset account")).toBeInTheDocument();
    });
  });

  it("toggling a candidate check changes the Selected total", async () => {
    await startAndEnterWorking("1080.00");
    const checkboxes = screen.getAllByLabelText("Mark as reconciled");
    expect(checkboxes).toHaveLength(3);

    // Locate the Selected cell by its label, then walk to the sibling value
    // <p> so we don't catch matching totals in Opening/Cleared/Difference.
    const selectedLabel = screen.getByText("Selected");
    const selectedValue = selectedLabel.nextElementSibling as HTMLElement;
    // Initial selected: tx 1001 ($50.00) + tx 1002 ($30.00) = $80.00
    expect(selectedValue.textContent).toMatch(/80\.00/);

    // Uncheck tx 1001 → selected drops to $30.00
    await fireEvent.click(checkboxes[0]);
    await waitFor(() => {
      expect(selectedValue.textContent).toMatch(/30\.00/);
    });
  });
});

describe("Reconcile — finish behavior (C2)", () => {
  it("Finish disabled while offset is required but unset", async () => {
    await startWith({ selectedClearedIds: [1001, 1002] });
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1085.00" }, // unbalanced
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.getByLabelText("Offset account")).toBeInTheDocument();
    });
    const finishBtn = screen.getByRole("button", { name: "Finish reconciliation" });
    expect(finishBtn).toBeDisabled();
  });

  it("Finish with balanced state posts without an offset account", async () => {
    vi.mocked(finishReconciliation).mockResolvedValue({
      balance_check: {} as never,
      balancing: {} as never,
      adjustment: null,
      difference_minor: 0,
      reconciled_count: 2,
      uncleared_count: 0,
    });

    await startWith({ selectedClearedIds: [1001, 1002] });
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1080.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.queryByLabelText("Offset account")).not.toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Finish reconciliation" }));
    await waitFor(() => expect(vi.mocked(finishReconciliation)).toHaveBeenCalledOnce());
    const [, payload] = vi.mocked(finishReconciliation).mock.calls[0];
    expect(payload.offset_account_id).toBeNull();
    expect(payload.confirm_unbalanced).toBe(false);
    expect(payload.statement_balance_minor).toBe(108000);
  });

  it("Finish with unbalanced + selected offset posts with offset_account_id and confirm_unbalanced=true", async () => {
    vi.mocked(finishReconciliation).mockResolvedValue({
      balance_check: {} as never,
      balancing: {} as never,
      adjustment: null,
      difference_minor: 500,
      reconciled_count: 2,
      uncleared_count: 0,
    });

    await startWith({ selectedClearedIds: [1001, 1002] });
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1085.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.getByLabelText("Offset account")).toBeInTheDocument();
    });

    await fireEvent.change(screen.getByLabelText("Offset account"), {
      target: { value: "200" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Finish reconciliation" }));
    await waitFor(() => expect(vi.mocked(finishReconciliation)).toHaveBeenCalledOnce());
    const [, payload] = vi.mocked(finishReconciliation).mock.calls[0];
    expect(payload.offset_account_id).toBe(200);
    expect(payload.confirm_unbalanced).toBe(true);
  });

  it("after a successful Finish, navigates to /accounts/{id}", async () => {
    vi.mocked(finishReconciliation).mockResolvedValue({
      balance_check: {} as never,
      balancing: {} as never,
      adjustment: null,
      difference_minor: 0,
      reconciled_count: 0,
      uncleared_count: 0,
    });

    await startWith({ selectedClearedIds: [1001, 1002] });
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1080.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.queryByLabelText("Offset account")).not.toBeInTheDocument();
    });
    await fireEvent.click(screen.getByRole("button", { name: "Finish reconciliation" }));
    await waitFor(() => expect(vi.mocked(goto)).toHaveBeenCalledWith("/accounts/100"));
  });

  it("Finish surfaces error message when the API throws", async () => {
    vi.mocked(finishReconciliation).mockRejectedValue(new Error("balance mismatch"));

    await startWith({ selectedClearedIds: [1001, 1002] });
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1080.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.queryByLabelText("Offset account")).not.toBeInTheDocument();
    });
    await fireEvent.click(screen.getByRole("button", { name: "Finish reconciliation" }));
    expect(await screen.findByText(/balance mismatch/)).toBeInTheDocument();
  });
});

describe("Reconcile — Select all / none (C2)", () => {
  it("Select all checks every candidate", async () => {
    await startWith();
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1060.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.getByText(/0 \/ 3 selected/)).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Select all" }));
    await waitFor(() => {
      expect(screen.getByText(/3 \/ 3 selected/)).toBeInTheDocument();
    });
  });

  it("Select none clears all checks", async () => {
    await startWith({ selectedClearedIds: [1001, 1002] });
    await fireEvent.input(screen.getByLabelText("Statement ending balance"), {
      target: { value: "1080.00" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => {
      expect(screen.getByText(/2 \/ 3 selected/)).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Select none" }));
    await waitFor(() => {
      expect(screen.getByText(/0 \/ 3 selected/)).toBeInTheDocument();
    });
  });
});
