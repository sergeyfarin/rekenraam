import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import TransactionSplitEditor from "./TransactionSplitEditor.svelte";
import { emptySplitDraft, type SplitDraft } from "$lib/transactions/split-draft";

import type { CommoditySummary } from "$lib/api/metadata";

type Account = { id: number; name: string; commodity_id: number };

const USD: CommoditySummary = {
  id: 1,
  book_id: 1,
  kind: "currency",
  symbol: "$",
  name: "USD",
  scale: 2,
  metadata: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const accounts: Account[] = [
  { id: 10, name: "Checking", commodity_id: 1 },
  { id: 20, name: "Savings", commodity_id: 1 },
  { id: 30, name: "Groceries", commodity_id: 1 },
];

function makeSplit(overrides: Partial<SplitDraft> = {}): SplitDraft {
  return { ...emptySplitDraft(), ...overrides };
}

function baseProps(splits: SplitDraft[]) {
  return {
    open: true,
    splits,
    accounts,
    categories: [],
    tags: [],
    people: [],
    projects: [],
    commodities: [USD],
  };
}

describe("TransactionSplitEditor", () => {
  it("renders one row per split", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "10.00" }),
      makeSplit({ account_id: 20, amount: "-10.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    expect(screen.getByText("Split transaction")).toBeInTheDocument();
    // Each row's Amount input uses the "0.00" placeholder — count those for a
    // row count that isn't polluted by the 4 datalist-backed inputs per row.
    expect(screen.getAllByPlaceholderText("0.00")).toHaveLength(2);
  });

  it("shows ✓ Balanced when splits sum to zero", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "25.00" }),
      makeSplit({ account_id: 20, amount: "-25.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    expect(screen.getByText(/✓ Balanced/)).toBeInTheDocument();
  });

  it("shows unbalanced amount with sign when splits do not sum to zero", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "30.00" }),
      makeSplit({ account_id: 20, amount: "-25.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    // +5.00 (positive overshoot) with USD symbol and "(unbalanced)" suffix
    expect(screen.getByText(/\+5\.00 \$ \(unbalanced\)/)).toBeInTheDocument();
  });

  it("renders no balance indicator when a split has no account selected", () => {
    const splits = [
      makeSplit({ account_id: null, amount: "10.00" }),
      makeSplit({ account_id: 20, amount: "-10.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    expect(screen.queryByText(/Balance:/)).not.toBeInTheDocument();
  });

  it("disables the Remove button when only 2 splits remain (UX guard)", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "10.00" }),
      makeSplit({ account_id: 20, amount: "-10.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    const removeButtons = screen.getAllByRole("button", { name: "Remove" });
    expect(removeButtons).toHaveLength(2);
    for (const button of removeButtons) {
      expect(button).toBeDisabled();
    }
  });

  it("enables the Remove button when 3+ splits exist", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "10.00" }),
      makeSplit({ account_id: 20, amount: "-5.00" }),
      makeSplit({ account_id: 30, amount: "-5.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    const removeButtons = screen.getAllByRole("button", { name: "Remove" });
    expect(removeButtons).toHaveLength(3);
    for (const button of removeButtons) {
      expect(button).not.toBeDisabled();
    }
  });

  it("Add split appends a fresh empty draft to the splits array", async () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "10.00" }),
      makeSplit({ account_id: 20, amount: "-10.00" }),
    ];
    const { rerender } = render(TransactionSplitEditor, baseProps(splits));
    expect(screen.getAllByPlaceholderText("0.00")).toHaveLength(2);

    await fireEvent.click(screen.getByRole("button", { name: "Add split" }));

    // After adding, there should be 3 amount inputs. Svelte 5 propagates the
    // mutation through $bindable, so the DOM reflects the new row directly.
    expect(screen.getAllByPlaceholderText("0.00")).toHaveLength(3);
    // Avoid unused-var warning on rerender
    void rerender;
  });

  it("Remove on a 3-row split drops the targeted row", async () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "10.00", memo: "first" }),
      makeSplit({ account_id: 20, amount: "-5.00", memo: "second" }),
      makeSplit({ account_id: 30, amount: "-5.00", memo: "third" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    const removeButtons = screen.getAllByRole("button", { name: "Remove" });
    expect(removeButtons).toHaveLength(3);

    await fireEvent.click(removeButtons[1]); // remove middle row
    expect(screen.getAllByRole("button", { name: "Remove" })).toHaveLength(2);
  });

  it("Done button is rendered (toggles open via bindable)", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "1.00" }),
      makeSplit({ account_id: 20, amount: "-1.00" }),
    ];
    render(TransactionSplitEditor, baseProps(splits));
    expect(screen.getByRole("button", { name: "Done" })).toBeInTheDocument();
  });

  it("does not render dialog content when open=false", () => {
    const splits = [
      makeSplit({ account_id: 10, amount: "10.00" }),
      makeSplit({ account_id: 20, amount: "-10.00" }),
    ];
    render(TransactionSplitEditor, { ...baseProps(splits), open: false });
    expect(screen.queryByText("Split transaction")).not.toBeInTheDocument();
  });
});
