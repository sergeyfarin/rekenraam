import { describe, expect, it } from "vitest";
import { buildDividendPayload, buildTradePayload } from "./forms";

describe("investment form builders", () => {
  it("builds buy payloads with minor-unit scaling", () => {
    expect(
      buildTradePayload(42, "buy", {
        txn_date: "2026-05-18",
        investment_account_id: "10",
        cash_account_id: "11",
        commodity_id: "2",
        quantity: "12.3456",
        cash_amount: "105.75",
        memo: "",
      }, 4)
    ).toEqual({
      book_id: 42,
      txn_date: "2026-05-18",
      investment_account_id: 10,
      cash_account_id: 11,
      commodity_id: 2,
      quantity_minor: 123456,
      cash_amount_minor: 10575,
      memo: null,
      payee_id: null,
      status: null,
    });
  });

  it("builds sell payloads with explicit lot strategy", () => {
    expect(
      buildTradePayload(42, "sell", {
        txn_date: "2026-05-18",
        investment_account_id: 10,
        cash_account_id: 11,
        commodity_id: 2,
        quantity: "1.5000",
        cash_amount: "20.00",
        memo: "trim me ",
      }, 4)
    ).toEqual({
      book_id: 42,
      txn_date: "2026-05-18",
      investment_account_id: 10,
      cash_account_id: 11,
      commodity_id: 2,
      quantity_minor: 15000,
      cash_amount_minor: 2000,
      lot_strategy: "fifo",
      lot_allocations: null,
      allow_short: false,
      memo: "trim me",
      payee_id: null,
      status: null,
    });
  });

  it("builds dividend payloads aligned with the backend contract only", () => {
    const payload = buildDividendPayload(42, {
      txn_date: "2026-05-18",
      cash_account_id: "11",
      income_account_id: "12",
      amount: "25.50",
      memo: "",
    });

    expect(payload).toEqual({
      book_id: 42,
      txn_date: "2026-05-18",
      cash_account_id: 11,
      income_account_id: 12,
      amount_minor: 2550,
      memo: null,
      payee_id: null,
      status: null,
    });
    expect(payload).not.toHaveProperty("investment_account_id");
    expect(payload).not.toHaveProperty("commodity_id");
  });
});