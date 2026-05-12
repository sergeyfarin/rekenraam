import { describe, expect, it } from "vitest";
import { isSplitsBalanced, sumSplitsInMinor } from "./split-balance";

describe("sumSplitsInMinor", () => {
  it("returns 0 for an empty list", () => {
    expect(sumSplitsInMinor([])).toBe(0);
  });

  it("sums a balanced two-leg split at scale 2", () => {
    const splits = [
      { amount: "10.00", scale: 2 },
      { amount: "-10.00", scale: 2 },
    ];
    expect(sumSplitsInMinor(splits)).toBe(0);
  });

  it("returns a non-zero total for an unbalanced two-leg split", () => {
    const splits = [
      { amount: "10.00", scale: 2 },
      { amount: "-9.99", scale: 2 },
    ];
    expect(sumSplitsInMinor(splits)).toBe(1);
  });

  it("sums a balanced three-leg split", () => {
    const splits = [
      { amount: "30.00", scale: 2 },
      { amount: "-20.00", scale: 2 },
      { amount: "-10.00", scale: 2 },
    ];
    expect(sumSplitsInMinor(splits)).toBe(0);
  });

  it("sums correctly across mixed scales (legs are summed in raw minor units)", () => {
    // 1.00 at scale 2 = 100 minor; -1.0000 at scale 4 = -10000 minor.
    // The current page-level helper sums raw minor units without commodity
    // awareness; this test pins that behavior.
    const splits = [
      { amount: "1.00", scale: 2 },
      { amount: "-1.0000", scale: 4 },
    ];
    expect(sumSplitsInMinor(splits)).toBe(-9900);
  });

  it("returns null when any split's amount fails to parse", () => {
    const splits = [
      { amount: "10.00", scale: 2 },
      { amount: "abc", scale: 2 },
    ];
    expect(sumSplitsInMinor(splits)).toBeNull();
  });

  it("treats an empty amount string as zero (not null)", () => {
    // parseAmountToMinor returns 0 (not null) for an empty input.
    const splits = [
      { amount: "", scale: 2 },
      { amount: "10.00", scale: 2 },
    ];
    expect(sumSplitsInMinor(splits)).toBe(1000);
  });

  it("propagates null short-circuiting on the first unparseable split", () => {
    const splits = [
      { amount: "1,000", scale: 2 },
      { amount: "10.00", scale: 2 },
    ];
    expect(sumSplitsInMinor(splits)).toBeNull();
  });
});

describe("isSplitsBalanced", () => {
  it("returns true for an empty list (sum 0)", () => {
    expect(isSplitsBalanced([])).toBe(true);
  });

  it("returns true for a balanced two-leg split", () => {
    expect(
      isSplitsBalanced([
        { amount: "5.00", scale: 2 },
        { amount: "-5.00", scale: 2 },
      ]),
    ).toBe(true);
  });

  it("returns false for a one-cent imbalance", () => {
    expect(
      isSplitsBalanced([
        { amount: "5.00", scale: 2 },
        { amount: "-4.99", scale: 2 },
      ]),
    ).toBe(false);
  });

  it("returns false when any split fails to parse", () => {
    expect(
      isSplitsBalanced([
        { amount: "5.00", scale: 2 },
        { amount: "not-a-number", scale: 2 },
      ]),
    ).toBe(false);
  });
});
