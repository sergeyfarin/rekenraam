import { describe, expect, it } from "vitest";
import { deriveReconciliationState, sumCheckedAmounts } from "./state";

describe("deriveReconciliationState", () => {
  it("computes cleared balance as opening + checked amount", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 10000,
      checkedAmountMinor: 2500,
      statementBalanceMinor: 12500,
    });
    expect(state.clearedBalanceMinor).toBe(12500);
  });

  it("reports zero difference and no offset when cleared equals statement", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 10000,
      checkedAmountMinor: 2500,
      statementBalanceMinor: 12500,
    });
    expect(state.differenceMinor).toBe(0);
    expect(state.needsOffset).toBe(false);
  });

  it("reports positive difference when cleared exceeds statement", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 10000,
      checkedAmountMinor: 2501,
      statementBalanceMinor: 12500,
    });
    expect(state.differenceMinor).toBe(1);
    expect(state.needsOffset).toBe(true);
  });

  it("reports negative difference when cleared falls short of statement", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 10000,
      checkedAmountMinor: 2499,
      statementBalanceMinor: 12500,
    });
    expect(state.differenceMinor).toBe(-1);
    expect(state.needsOffset).toBe(true);
  });

  it("flags a one-cent mismatch as needing offset", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 0,
      checkedAmountMinor: 0,
      statementBalanceMinor: 1,
    });
    expect(state.differenceMinor).toBe(-1);
    expect(state.needsOffset).toBe(true);
  });

  it("returns difference = null and no offset when statement is unknown", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 10000,
      checkedAmountMinor: 2500,
      statementBalanceMinor: null,
    });
    expect(state.clearedBalanceMinor).toBe(12500);
    expect(state.differenceMinor).toBeNull();
    expect(state.needsOffset).toBe(false);
  });

  it("treats zero opening and zero checked with zero statement as balanced", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: 0,
      checkedAmountMinor: 0,
      statementBalanceMinor: 0,
    });
    expect(state.clearedBalanceMinor).toBe(0);
    expect(state.differenceMinor).toBe(0);
    expect(state.needsOffset).toBe(false);
  });

  it("handles a negative opening balance (e.g. liability or overdraft)", () => {
    const state = deriveReconciliationState({
      openingBalanceMinor: -5000,
      checkedAmountMinor: 2000,
      statementBalanceMinor: -3000,
    });
    expect(state.clearedBalanceMinor).toBe(-3000);
    expect(state.differenceMinor).toBe(0);
    expect(state.needsOffset).toBe(false);
  });
});

describe("sumCheckedAmounts", () => {
  const candidates = [
    { id: 1, splitAmountMinor: 100 },
    { id: 2, splitAmountMinor: 200 },
    { id: 3, splitAmountMinor: -50 },
  ];

  it("returns 0 when no candidates are checked", () => {
    expect(sumCheckedAmounts(candidates, new Set<number>())).toBe(0);
  });

  it("returns 0 when candidates is empty", () => {
    expect(sumCheckedAmounts([], new Set([1, 2]))).toBe(0);
  });

  it("sums only checked candidates", () => {
    expect(sumCheckedAmounts(candidates, new Set([1, 2]))).toBe(300);
    expect(sumCheckedAmounts(candidates, new Set([1, 3]))).toBe(50);
  });

  it("ignores checked ids that aren't in the candidate list", () => {
    expect(sumCheckedAmounts(candidates, new Set([1, 99]))).toBe(100);
  });

  it("works for all candidates checked", () => {
    expect(sumCheckedAmounts(candidates, new Set([1, 2, 3]))).toBe(250);
  });
});
