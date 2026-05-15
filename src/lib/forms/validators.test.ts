import { describe, expect, it } from "vitest";
import {
  combine,
  isValidIsoDate,
  validateIntegerInRange,
  validateIsoDate,
  validateNonEmptyString,
  validateNonZeroAmountMinor,
  validatePositiveAmountMinor,
  validateScaleAwareAmountMinor,
} from "./validators";

describe("validateNonEmptyString", () => {
  it("accepts a non-empty string and returns it trimmed", () => {
    const r = validateNonEmptyString("  hello  ");
    expect(r).toEqual({ ok: true, value: "hello" });
  });

  it("rejects an empty string", () => {
    expect(validateNonEmptyString("")).toEqual({ ok: false, error: "Value is required" });
  });

  it("rejects a whitespace-only string", () => {
    expect(validateNonEmptyString("   \t\n")).toEqual({ ok: false, error: "Value is required" });
  });

  it("rejects null and undefined", () => {
    expect(validateNonEmptyString(null).ok).toBe(false);
    expect(validateNonEmptyString(undefined).ok).toBe(false);
  });

  it("uses the provided field name in error messages", () => {
    const r = validateNonEmptyString("", "Payee");
    expect(r).toEqual({ ok: false, error: "Payee is required" });
  });
});

describe("validatePositiveAmountMinor", () => {
  it("accepts a positive decimal at scale 2", () => {
    expect(validatePositiveAmountMinor("12.34", 2)).toEqual({ ok: true, value: 1234 });
  });

  it("accepts a positive integer at scale 0", () => {
    expect(validatePositiveAmountMinor("500", 0)).toEqual({ ok: true, value: 500 });
  });

  it("rejects zero", () => {
    expect(validatePositiveAmountMinor("0", 2)).toEqual({
      ok: false,
      error: "Amount must be greater than zero",
    });
  });

  it("rejects empty string (parses to 0)", () => {
    expect(validatePositiveAmountMinor("", 2).ok).toBe(false);
  });

  it("rejects a negative amount", () => {
    expect(validatePositiveAmountMinor("-1.00", 2)).toEqual({
      ok: false,
      error: "Amount must be greater than zero",
    });
  });

  it("rejects non-numeric junk", () => {
    expect(validatePositiveAmountMinor("abc", 2)).toEqual({
      ok: false,
      error: "Amount must be a valid number",
    });
  });

  it("rejects scientific notation (parser does not allow exponents)", () => {
    expect(validatePositiveAmountMinor("1e3", 2).ok).toBe(false);
  });

  it("honors the provided field name", () => {
    expect(validatePositiveAmountMinor("0", 2, "Principal")).toEqual({
      ok: false,
      error: "Principal must be greater than zero",
    });
  });
});

describe("validateNonZeroAmountMinor", () => {
  it("accepts a negative amount", () => {
    expect(validateNonZeroAmountMinor("-5.00", 2)).toEqual({ ok: true, value: -500 });
  });

  it("rejects zero", () => {
    expect(validateNonZeroAmountMinor("0", 2)).toEqual({
      ok: false,
      error: "Amount cannot be zero",
    });
  });

  it("rejects empty string (parses to 0)", () => {
    expect(validateNonZeroAmountMinor("", 2).ok).toBe(false);
  });
});

describe("validateScaleAwareAmountMinor", () => {
  it("scale 8 (crypto) accepts long fractional input and truncates", () => {
    expect(validateScaleAwareAmountMinor("0.12345678", 8)).toEqual({
      ok: true,
      value: 12345678,
    });
  });

  it("scale 4 (FX) accepts 4-place precision", () => {
    expect(validateScaleAwareAmountMinor("1.2345", 4)).toEqual({ ok: true, value: 12345 });
  });

  it("scale 0 (JPY) treats input as bare integer", () => {
    expect(validateScaleAwareAmountMinor("1500", 0)).toEqual({ ok: true, value: 1500 });
  });

  it("accepts zero", () => {
    expect(validateScaleAwareAmountMinor("0", 2)).toEqual({ ok: true, value: 0 });
  });

  it("rejects unparseable input", () => {
    expect(validateScaleAwareAmountMinor("--5", 2).ok).toBe(false);
  });
});

describe("isValidIsoDate", () => {
  it("accepts a typical YYYY-MM-DD date", () => {
    expect(isValidIsoDate("2026-05-15")).toBe(true);
  });

  it("requires two-digit month and day padding", () => {
    expect(isValidIsoDate("2026-5-15")).toBe(false);
    expect(isValidIsoDate("2026-05-5")).toBe(false);
  });

  it("rejects non-string input", () => {
    expect(isValidIsoDate(null as unknown as string)).toBe(false);
    expect(isValidIsoDate(undefined as unknown as string)).toBe(false);
    expect(isValidIsoDate(20260515 as unknown as string)).toBe(false);
  });

  it("rejects empty string", () => {
    expect(isValidIsoDate("")).toBe(false);
  });

  it("accepts leap day in a leap year (2024-02-29)", () => {
    expect(isValidIsoDate("2024-02-29")).toBe(true);
  });

  it("rejects leap day in a non-leap year (2025-02-29)", () => {
    expect(isValidIsoDate("2025-02-29")).toBe(false);
  });

  it("rejects month overflow (month 13)", () => {
    expect(isValidIsoDate("2026-13-01")).toBe(false);
  });

  it("rejects day overflow for the month (April 31)", () => {
    expect(isValidIsoDate("2026-04-31")).toBe(false);
  });

  it("rejects month or day of zero", () => {
    expect(isValidIsoDate("2026-00-15")).toBe(false);
    expect(isValidIsoDate("2026-05-00")).toBe(false);
  });

  it("accepts year boundaries (century leap rules: 2000 yes, 1900 no)", () => {
    expect(isValidIsoDate("2000-02-29")).toBe(true);
    expect(isValidIsoDate("1900-02-29")).toBe(false);
  });
});

describe("validateIsoDate", () => {
  it("returns the date verbatim when valid", () => {
    expect(validateIsoDate("2026-05-15")).toEqual({ ok: true, value: "2026-05-15" });
  });

  it("rejects an empty string with 'required' message", () => {
    expect(validateIsoDate("")).toEqual({ ok: false, error: "Date is required" });
  });

  it("rejects null and undefined", () => {
    expect(validateIsoDate(null).ok).toBe(false);
    expect(validateIsoDate(undefined).ok).toBe(false);
  });

  it("rejects malformed date with 'must be a valid ISO date' message", () => {
    expect(validateIsoDate("nope")).toEqual({
      ok: false,
      error: "Date must be a valid ISO date (YYYY-MM-DD)",
    });
  });

  it("honors the provided field name", () => {
    expect(validateIsoDate("", "Start date")).toEqual({
      ok: false,
      error: "Start date is required",
    });
  });
});

describe("validateIntegerInRange", () => {
  it("accepts an integer with no bounds set", () => {
    expect(validateIntegerInRange(42)).toEqual({ ok: true, value: 42 });
  });

  it("rejects null and undefined", () => {
    expect(validateIntegerInRange(null).ok).toBe(false);
    expect(validateIntegerInRange(undefined).ok).toBe(false);
  });

  it("rejects non-integer floats", () => {
    expect(validateIntegerInRange(1.5)).toEqual({ ok: false, error: "Value must be an integer" });
  });

  it("rejects NaN and Infinity", () => {
    expect(validateIntegerInRange(NaN).ok).toBe(false);
    expect(validateIntegerInRange(Infinity).ok).toBe(false);
  });

  it("enforces the minimum", () => {
    expect(validateIntegerInRange(0, { min: 1, fieldName: "Rate" })).toEqual({
      ok: false,
      error: "Rate must be at least 1",
    });
  });

  it("enforces the maximum", () => {
    expect(validateIntegerInRange(10001, { max: 10000, fieldName: "Basis points" })).toEqual({
      ok: false,
      error: "Basis points must be at most 10000",
    });
  });

  it("accepts the inclusive boundaries", () => {
    expect(validateIntegerInRange(1, { min: 1, max: 1 })).toEqual({ ok: true, value: 1 });
  });
});

describe("combine", () => {
  it("collapses all-ok results into a single ok with the value map", () => {
    const result = combine({
      name: validateNonEmptyString("Mortgage"),
      date: validateIsoDate("2026-05-15"),
      amount: validatePositiveAmountMinor("100.00", 2),
    });
    expect(result).toEqual({
      ok: true,
      value: { name: "Mortgage", date: "2026-05-15", amount: 10000 },
    });
  });

  it("collects every error when any result fails", () => {
    const result = combine({
      name: validateNonEmptyString("", "Name"),
      date: validateIsoDate("bad", "Start"),
      amount: validatePositiveAmountMinor("0", 2, "Amount"),
    });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.error).toContain("Name is required");
      expect(result.error).toContain("Start must be a valid ISO date");
      expect(result.error).toContain("Amount must be greater than zero");
    }
  });

  it("returns ok on an empty input map", () => {
    expect(combine({})).toEqual({ ok: true, value: {} });
  });
});
