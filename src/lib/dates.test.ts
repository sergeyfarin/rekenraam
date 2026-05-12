import { describe, expect, it } from "vitest";
import { parseSmartDate } from "./dates";

const TODAY = "2026-05-12";

describe("parseSmartDate — empty / shorthand", () => {
  it("returns null on empty or whitespace", () => {
    expect(parseSmartDate("", TODAY)).toBeNull();
    expect(parseSmartDate("   ", TODAY)).toBeNull();
  });

  it("expands 't' and 'today' (case-insensitive) to today", () => {
    expect(parseSmartDate("t", TODAY)).toBe(TODAY);
    expect(parseSmartDate("T", TODAY)).toBe(TODAY);
    expect(parseSmartDate("today", TODAY)).toBe(TODAY);
    expect(parseSmartDate("Today", TODAY)).toBe(TODAY);
    expect(parseSmartDate("TODAY", TODAY)).toBe(TODAY);
  });
});

describe("parseSmartDate — ISO YYYY-MM-DD", () => {
  it("passes through a valid ISO date unchanged", () => {
    expect(parseSmartDate("2026-05-12", TODAY)).toBe("2026-05-12");
    expect(parseSmartDate("1999-12-31", TODAY)).toBe("1999-12-31");
  });

  it("zero-pads single-digit month/day in ISO form", () => {
    expect(parseSmartDate("2026-5-1", TODAY)).toBe("2026-05-01");
    expect(parseSmartDate("2026-05-1", TODAY)).toBe("2026-05-01");
  });

  it("accepts Feb 29 in a leap year", () => {
    expect(parseSmartDate("2024-02-29", TODAY)).toBe("2024-02-29");
  });

  it("rejects Feb 29 in a non-leap year", () => {
    expect(parseSmartDate("2025-02-29", TODAY)).toBeNull();
    expect(parseSmartDate("2023-02-29", TODAY)).toBeNull();
  });

  it("rejects invalid month or day", () => {
    expect(parseSmartDate("2026-13-01", TODAY)).toBeNull();
    expect(parseSmartDate("2026-00-15", TODAY)).toBeNull();
    expect(parseSmartDate("2026-04-31", TODAY)).toBeNull();
    expect(parseSmartDate("2026-02-30", TODAY)).toBeNull();
  });
});

describe("parseSmartDate — separator-delimited two-part (M/D, D/M auto)", () => {
  it("resolves unambiguous MM/DD as month/day", () => {
    expect(parseSmartDate("5/12", TODAY)).toBe("2026-05-12");
    expect(parseSmartDate("1/15", TODAY)).toBe("2026-01-15");
  });

  it("resolves when first part is > 12 by swapping to day-first", () => {
    expect(parseSmartDate("13/5", TODAY)).toBe("2026-05-13");
    expect(parseSmartDate("25/12", TODAY)).toBe("2025-12-25");
  });

  it("resolves when second part is > 12 by keeping month-first", () => {
    expect(parseSmartDate("5/13", TODAY)).toBe("2026-05-13");
    expect(parseSmartDate("12/25", TODAY)).toBe("2025-12-25");
  });

  it("accepts dash and dot separators", () => {
    expect(parseSmartDate("5-12", TODAY)).toBe("2026-05-12");
    expect(parseSmartDate("5.12", TODAY)).toBe("2026-05-12");
  });

  it("guesses prior year when candidate is more than 60 days in the future", () => {
    // From May 12: Dec 25 is ~227d away → 2025; Aug 1 is ~81d away → 2025.
    expect(parseSmartDate("12/25", TODAY)).toBe("2025-12-25");
    expect(parseSmartDate("8/1", TODAY)).toBe("2025-08-01");
  });

  it("keeps current year when candidate is within 60 days in the future", () => {
    // From May 12: Jul 1 is ~50d away → 2026.
    expect(parseSmartDate("7/1", TODAY)).toBe("2026-07-01");
  });

  it("guesses current year when candidate is in the recent past", () => {
    expect(parseSmartDate("1/1", TODAY)).toBe("2026-01-01");
  });

  it("rejects invalid month/day combinations", () => {
    expect(parseSmartDate("13/45", TODAY)).toBeNull();
    expect(parseSmartDate("2/30", TODAY)).toBeNull();
  });
});

describe("parseSmartDate — separator-delimited three-part", () => {
  it("interprets two-digit year as 2000+yy", () => {
    expect(parseSmartDate("5/12/24", TODAY)).toBe("2024-05-12");
    expect(parseSmartDate("5/12/99", TODAY)).toBe("2099-05-12");
  });

  it("interprets four-digit year as-is", () => {
    expect(parseSmartDate("5/12/2024", TODAY)).toBe("2024-05-12");
  });

  it("treats first > 31 as YYYY/MM/DD", () => {
    expect(parseSmartDate("2024/5/12", TODAY)).toBe("2024-05-12");
    expect(parseSmartDate("2024-5-12", TODAY)).toBe("2024-05-12");
    expect(parseSmartDate("2024.5.12", TODAY)).toBe("2024-05-12");
  });

  it("rejects three-part with invalid date in a non-leap year", () => {
    expect(parseSmartDate("2/29/25", TODAY)).toBeNull();
  });

  it("accepts three-part Feb 29 in a leap year", () => {
    expect(parseSmartDate("2/29/24", TODAY)).toBe("2024-02-29");
  });
});

describe("parseSmartDate — bare digits (no separator)", () => {
  it("treats 1-2 digit input as day-of-month in current or prior month", () => {
    // candidate day this month <= today: stays in current month.
    expect(parseSmartDate("10", TODAY)).toBe("2026-05-10");
    expect(parseSmartDate("12", TODAY)).toBe("2026-05-12");
  });

  it("stays in current month when candidate is at most 2 days in the future", () => {
    // today = May 12; "13" -> May 13 (diff = 0.5d from noon anchor).
    expect(parseSmartDate("13", TODAY)).toBe("2026-05-13");
    expect(parseSmartDate("14", TODAY)).toBe("2026-05-14");
  });

  it("steps back to prior month when candidate is more than 2 days in the future", () => {
    // today = May 12; "15" -> April 15 (diff = 2.5d).
    expect(parseSmartDate("15", TODAY)).toBe("2026-04-15");
    expect(parseSmartDate("30", TODAY)).toBe("2026-04-30");
  });

  it("rolls year back across a January boundary", () => {
    // today = Jan 10 2026; "15" -> Dec 15 2025 (step back from Jan).
    expect(parseSmartDate("15", "2026-01-10")).toBe("2025-12-15");
  });

  it("returns null for day 0 or day > 31", () => {
    expect(parseSmartDate("0", TODAY)).toBeNull();
    expect(parseSmartDate("32", TODAY)).toBeNull();
  });

  it("interprets 3-digit input as MDD", () => {
    expect(parseSmartDate("512", TODAY)).toBe("2026-05-12");
    expect(parseSmartDate("113", "2026-02-01")).toBe("2026-01-13");
  });

  it("interprets 4-digit input as MMDD", () => {
    expect(parseSmartDate("0512", TODAY)).toBe("2026-05-12");
    expect(parseSmartDate("1225", TODAY)).toBe("2025-12-25");
  });

  it("rejects malformed 3/4-digit input", () => {
    expect(parseSmartDate("999", TODAY)).toBeNull();
    expect(parseSmartDate("1345", TODAY)).toBeNull();
  });
});

describe("parseSmartDate — junk input", () => {
  it("returns null for non-matching strings", () => {
    expect(parseSmartDate("abc", TODAY)).toBeNull();
    expect(parseSmartDate("x/y", TODAY)).toBeNull();
    expect(parseSmartDate("5/", TODAY)).toBeNull();
    expect(parseSmartDate("/5", TODAY)).toBeNull();
    expect(parseSmartDate("5//12", TODAY)).toBeNull();
  });
});
