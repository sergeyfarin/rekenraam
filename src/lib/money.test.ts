import { describe, expect, it } from "vitest";
import { formatMinorWithScale, parseAmountToMinor } from "./money";

describe("formatMinorWithScale", () => {
  it("formats positive amounts at scale 2", () => {
    expect(formatMinorWithScale(0, 2)).toBe("0.00");
    expect(formatMinorWithScale(1, 2)).toBe("0.01");
    expect(formatMinorWithScale(99, 2)).toBe("0.99");
    expect(formatMinorWithScale(100, 2)).toBe("1.00");
    expect(formatMinorWithScale(12345, 2)).toBe("123.45");
  });

  it("formats negative amounts at scale 2", () => {
    expect(formatMinorWithScale(-1, 2)).toBe("-0.01");
    expect(formatMinorWithScale(-100, 2)).toBe("-1.00");
    expect(formatMinorWithScale(-12345, 2)).toBe("-123.45");
  });

  it("formats scale 0 as bare integer with no decimal point", () => {
    expect(formatMinorWithScale(0, 0)).toBe("0");
    expect(formatMinorWithScale(42, 0)).toBe("42");
    expect(formatMinorWithScale(-42, 0)).toBe("-42");
  });

  it("treats negative scale the same as zero", () => {
    expect(formatMinorWithScale(42, -1)).toBe("42");
  });

  it("pads fractional digits for higher scale", () => {
    expect(formatMinorWithScale(5, 4)).toBe("0.0005");
    expect(formatMinorWithScale(50000, 4)).toBe("5.0000");
    expect(formatMinorWithScale(123456789, 8)).toBe("1.23456789");
  });

  it("does not insert thousands separators", () => {
    expect(formatMinorWithScale(1000000, 2)).toBe("10000.00");
  });
});

describe("parseAmountToMinor", () => {
  it("returns 0 for empty input", () => {
    expect(parseAmountToMinor("", 2)).toBe(0);
  });

  it("parses positive decimal at scale 2", () => {
    expect(parseAmountToMinor("0", 2)).toBe(0);
    expect(parseAmountToMinor("0.01", 2)).toBe(1);
    expect(parseAmountToMinor("1.00", 2)).toBe(100);
    expect(parseAmountToMinor("123.45", 2)).toBe(12345);
  });

  it("parses negative decimal at scale 2", () => {
    expect(parseAmountToMinor("-0.01", 2)).toBe(-1);
    expect(parseAmountToMinor("-123.45", 2)).toBe(-12345);
  });

  it("accepts explicit plus sign", () => {
    expect(parseAmountToMinor("+5", 2)).toBe(500);
    expect(parseAmountToMinor("+0.25", 2)).toBe(25);
  });

  it("trims surrounding whitespace", () => {
    expect(parseAmountToMinor("  5.00  ", 2)).toBe(500);
  });

  it("truncates fractional digits past scale (no rounding)", () => {
    expect(parseAmountToMinor("1.239", 2)).toBe(123);
    expect(parseAmountToMinor("1.999", 2)).toBe(199);
  });

  it("pads fractional digits shorter than scale", () => {
    expect(parseAmountToMinor("1.5", 2)).toBe(150);
    expect(parseAmountToMinor("1.5", 4)).toBe(15000);
  });

  it("accepts trailing dot", () => {
    expect(parseAmountToMinor("3.", 2)).toBe(300);
  });

  it("handles scale 0 as integer", () => {
    expect(parseAmountToMinor("42", 0)).toBe(42);
    expect(parseAmountToMinor("-42", 0)).toBe(-42);
    expect(parseAmountToMinor("", 0)).toBe(0);
  });

  it("handles high scale", () => {
    expect(parseAmountToMinor("1.23456789", 8)).toBe(123456789);
  });

  it("returns null on rejected input", () => {
    expect(parseAmountToMinor("abc", 2)).toBeNull();
    expect(parseAmountToMinor("1,234", 2)).toBeNull();
    expect(parseAmountToMinor("1.2.3", 2)).toBeNull();
    expect(parseAmountToMinor("1e3", 2)).toBeNull();
    expect(parseAmountToMinor("$5", 2)).toBeNull();
  });

  it("round-trips with formatMinorWithScale at scale 2", () => {
    const cases = [0, 1, -1, 100, -100, 12345, -12345];
    for (const minor of cases) {
      const formatted = formatMinorWithScale(minor, 2);
      expect(parseAmountToMinor(formatted, 2)).toBe(minor);
    }
  });

  it("round-trips with formatMinorWithScale at scale 4", () => {
    const cases = [0, 1, 9999, 10000, -10000, 12345678];
    for (const minor of cases) {
      const formatted = formatMinorWithScale(minor, 4);
      expect(parseAmountToMinor(formatted, 4)).toBe(minor);
    }
  });
});
