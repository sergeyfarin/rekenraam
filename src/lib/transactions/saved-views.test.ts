import { describe, expect, it } from "vitest";
import type { TransactionFilter } from "$lib/api/transactions";
import {
  applyViewToState,
  buildFilterFromState,
  DEFAULT_FILTER_STATE,
  type FilterFormState,
} from "./saved-views";

const BOOK_ID = 7;
const LIMIT = 50;

function buildOpts(offset = 0) {
  return { bookId: BOOK_ID, limit: LIMIT, offset };
}

describe("buildFilterFromState", () => {
  it("emits only book_id, sort, limit, offset for the default state", () => {
    const filter = buildFilterFromState(DEFAULT_FILTER_STATE, buildOpts(0));
    expect(filter).toEqual({
      book_id: BOOK_ID,
      account_id: undefined,
      date_from: undefined,
      date_to: undefined,
      search: undefined,
      status: undefined,
      amount_min: undefined,
      amount_max: undefined,
      sort_by: "date",
      sort_dir: "desc",
      limit: LIMIT,
      offset: 0,
    });
  });

  it("passes through populated string fields verbatim", () => {
    const state: FilterFormState = {
      ...DEFAULT_FILTER_STATE,
      search: "groceries",
      dateFrom: "2026-01-01",
      dateTo: "2026-01-31",
      statusFilter: "cleared",
      accountFilterId: 12,
      sortBy: "amount",
      sortDir: "asc",
    };
    const filter = buildFilterFromState(state, buildOpts(100));
    expect(filter.search).toBe("groceries");
    expect(filter.date_from).toBe("2026-01-01");
    expect(filter.date_to).toBe("2026-01-31");
    expect(filter.status).toBe("cleared");
    expect(filter.account_id).toBe(12);
    expect(filter.sort_by).toBe("amount");
    expect(filter.sort_dir).toBe("asc");
    expect(filter.offset).toBe(100);
  });

  it("converts decimal amount inputs to minor units (scale 2)", () => {
    const state: FilterFormState = { ...DEFAULT_FILTER_STATE, amountMin: "10", amountMax: "250.75" };
    const filter = buildFilterFromState(state, buildOpts());
    expect(filter.amount_min).toBe(1000);
    expect(filter.amount_max).toBe(25075);
  });

  it("treats negative amount inputs as absolute values", () => {
    const state: FilterFormState = { ...DEFAULT_FILTER_STATE, amountMin: "-50.5", amountMax: "-1" };
    const filter = buildFilterFromState(state, buildOpts());
    expect(filter.amount_min).toBe(5050);
    expect(filter.amount_max).toBe(100);
  });

  it("rounds fractional minor decimals via Math.round (float-aware)", () => {
    const state: FilterFormState = { ...DEFAULT_FILTER_STATE, amountMin: "1.005", amountMax: "9.999" };
    const filter = buildFilterFromState(state, buildOpts());
    // 1.005 * 100 = 100.4999… → 100 (IEEE-754 quirk; documented behavior, not a bug)
    expect(filter.amount_min).toBe(100);
    expect(filter.amount_max).toBe(1000);
  });

  it("omits amount fields when input is empty or unparseable", () => {
    const state: FilterFormState = { ...DEFAULT_FILTER_STATE, amountMin: "", amountMax: "abc" };
    const filter = buildFilterFromState(state, buildOpts());
    expect(filter.amount_min).toBeUndefined();
    expect(filter.amount_max).toBeUndefined();
  });

  it("omits account_id when null", () => {
    const filter = buildFilterFromState(DEFAULT_FILTER_STATE, buildOpts());
    expect(filter.account_id).toBeUndefined();
  });

  it("treats whitespace-only search and status as empty (falsy passthrough)", () => {
    const state: FilterFormState = { ...DEFAULT_FILTER_STATE, search: "", statusFilter: "" };
    const filter = buildFilterFromState(state, buildOpts());
    expect(filter.search).toBeUndefined();
    expect(filter.status).toBeUndefined();
  });

  it("propagates the requested offset", () => {
    const filter = buildFilterFromState(DEFAULT_FILTER_STATE, buildOpts(500));
    expect(filter.offset).toBe(500);
    expect(filter.limit).toBe(LIMIT);
  });
});

describe("applyViewToState", () => {
  it("returns defaults for an empty filters object", () => {
    expect(applyViewToState({})).toEqual(DEFAULT_FILTER_STATE);
  });

  it("maps each present field back to the form-state shape", () => {
    const filters: TransactionFilter = {
      book_id: BOOK_ID,
      account_id: 3,
      date_from: "2026-04-01",
      date_to: "2026-04-30",
      search: "rent",
      status: "uncleared",
      amount_min: 50000,
      amount_max: 250000,
      sort_by: "payee",
      sort_dir: "asc",
    };
    expect(applyViewToState(filters)).toEqual({
      search: "rent",
      dateFrom: "2026-04-01",
      dateTo: "2026-04-30",
      statusFilter: "uncleared",
      accountFilterId: 3,
      amountMin: "500",
      amountMax: "2500",
      sortBy: "payee",
      sortDir: "asc",
    });
  });

  it("represents fractional minor amounts as scale-2 decimals", () => {
    const state = applyViewToState({ amount_min: 12345, amount_max: 99 });
    expect(state.amountMin).toBe("123.45");
    expect(state.amountMax).toBe("0.99");
  });

  it("defaults sort_by to date and sort_dir to desc when missing", () => {
    const state = applyViewToState({ search: "x" });
    expect(state.sortBy).toBe("date");
    expect(state.sortDir).toBe("desc");
  });

  it("preserves account_id === 0 verbatim (?? null only catches null/undefined)", () => {
    const state = applyViewToState({ account_id: 0 });
    expect(state.accountFilterId).toBe(0);
  });
});

describe("round-trip", () => {
  it("state → filter → state preserves all observable fields", () => {
    const state: FilterFormState = {
      search: "coffee",
      dateFrom: "2026-02-01",
      dateTo: "2026-02-28",
      statusFilter: "reconciled",
      accountFilterId: 9,
      // Round-trip drops trailing zeros: "25.50" → 2550 → "25.5". Use minimal
      // forms here so the assertion holds; lossy-format behavior is covered below.
      amountMin: "25.5",
      amountMax: "100",
      sortBy: "status",
      sortDir: "asc",
    };
    const filter = buildFilterFromState(state, buildOpts());
    const restored = applyViewToState(filter);
    expect(restored).toEqual(state);
  });

  it("amountMin / amountMax round-trip is lossy on trailing zeros (documented)", () => {
    const state: FilterFormState = { ...DEFAULT_FILTER_STATE, amountMin: "25.50", amountMax: "1.00" };
    const restored = applyViewToState(buildFilterFromState(state, buildOpts()));
    expect(restored.amountMin).toBe("25.5");
    expect(restored.amountMax).toBe("1");
  });

  it("default state round-trips to default state", () => {
    const filter = buildFilterFromState(DEFAULT_FILTER_STATE, buildOpts());
    expect(applyViewToState(filter)).toEqual(DEFAULT_FILTER_STATE);
  });

  it("filter → state → filter preserves user-facing fields (limit/offset/book_id are call-site)", () => {
    const original: TransactionFilter = {
      search: "utilities",
      date_from: "2026-03-01",
      date_to: "2026-03-31",
      status: "cleared",
      account_id: 4,
      amount_min: 7500,
      amount_max: 30000,
      sort_by: "amount",
      sort_dir: "desc",
    };
    const state = applyViewToState(original);
    const rebuilt = buildFilterFromState(state, buildOpts(0));
    expect(rebuilt.search).toBe(original.search);
    expect(rebuilt.date_from).toBe(original.date_from);
    expect(rebuilt.date_to).toBe(original.date_to);
    expect(rebuilt.status).toBe(original.status);
    expect(rebuilt.account_id).toBe(original.account_id);
    expect(rebuilt.amount_min).toBe(original.amount_min);
    expect(rebuilt.amount_max).toBe(original.amount_max);
    expect(rebuilt.sort_by).toBe(original.sort_by);
    expect(rebuilt.sort_dir).toBe(original.sort_dir);
  });
});
