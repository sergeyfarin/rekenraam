import { describe, expect, it } from "vitest";
import {
  exactMatchByName,
  fuzzyMatch,
  fuzzyOptions,
  FUZZY_OPTIONS_LIMIT,
  normalizeName,
} from "./fuzzy";

describe("normalizeName", () => {
  it("trims and lowercases", () => {
    expect(normalizeName("  Hello World  ")).toBe("hello world");
    expect(normalizeName("FOO")).toBe("foo");
    expect(normalizeName("")).toBe("");
    expect(normalizeName("   ")).toBe("");
  });

  it("does not strip accents (pinned: ASCII-case-only normalization)", () => {
    expect(normalizeName("Café")).toBe("café");
    expect(normalizeName("Zürich")).toBe("zürich");
  });

  it("does not collapse internal whitespace", () => {
    expect(normalizeName("a   b")).toBe("a   b");
  });
});

describe("fuzzyMatch", () => {
  it("returns true when query is empty (matches everything)", () => {
    expect(fuzzyMatch("", "anything")).toBe(true);
    expect(fuzzyMatch("   ", "anything")).toBe(true);
    expect(fuzzyMatch("", "")).toBe(true);
  });

  it("matches a contiguous case-insensitive substring", () => {
    expect(fuzzyMatch("apple", "Apple Pie")).toBe(true);
    expect(fuzzyMatch("APPLE", "apple")).toBe(true);
  });

  it("matches an ordered non-contiguous subsequence", () => {
    expect(fuzzyMatch("aple", "Apple Pie")).toBe(true);
    expect(fuzzyMatch("abc", "a-b-c")).toBe(true);
    expect(fuzzyMatch("abc", "axbycz")).toBe(true);
  });

  it("rejects when query characters appear in wrong order", () => {
    expect(fuzzyMatch("ba", "abc")).toBe(false);
    expect(fuzzyMatch("cba", "abc")).toBe(false);
  });

  it("rejects when a query character is missing from the candidate", () => {
    expect(fuzzyMatch("xyz", "abc")).toBe(false);
    expect(fuzzyMatch("apxle", "apple")).toBe(false);
  });

  it("treats leading/trailing whitespace in query as ignored", () => {
    expect(fuzzyMatch("  pp  ", "apple")).toBe(true);
  });

  it("works with non-ASCII characters that survive lowercasing", () => {
    expect(fuzzyMatch("café", "Café Rio")).toBe(true);
    expect(fuzzyMatch("ürc", "Zürich")).toBe(true);
  });

  it("returns false when the candidate is empty and the query is not", () => {
    expect(fuzzyMatch("a", "")).toBe(false);
  });
});

describe("fuzzyOptions", () => {
  const items = [
    { id: 1, name: "Apple Pie" },
    { id: 2, name: "Apricot" },
    { id: 3, name: "Banana" },
    { id: 4, name: "Pineapple" },
  ];

  it("returns all items when query is empty", () => {
    expect(fuzzyOptions(items, "")).toEqual(items);
  });

  it("filters items matching the fuzzy query", () => {
    expect(fuzzyOptions(items, "ap").map((i) => i.id)).toEqual([1, 2, 4]);
  });

  it("preserves input order", () => {
    expect(fuzzyOptions(items, "n").map((i) => i.id)).toEqual([3, 4]);
  });

  it("caps results at FUZZY_OPTIONS_LIMIT (30)", () => {
    expect(FUZZY_OPTIONS_LIMIT).toBe(30);
    const many = Array.from({ length: 50 }, (_, i) => ({ id: i + 1, name: `Item ${i + 1}` }));
    const result = fuzzyOptions(many, "");
    expect(result).toHaveLength(30);
    expect(result[0]).toEqual({ id: 1, name: "Item 1" });
    expect(result[29]).toEqual({ id: 30, name: "Item 30" });
  });

  it("returns empty array when nothing matches", () => {
    expect(fuzzyOptions(items, "zzz")).toEqual([]);
  });
});

describe("exactMatchByName", () => {
  const items = [
    { id: 1, name: "Acme Corp" },
    { id: 2, name: "Beta LLC" },
    { id: 3, name: "  Gamma  " },
  ];

  it("returns undefined for an empty query", () => {
    expect(exactMatchByName(items, "")).toBeUndefined();
    expect(exactMatchByName(items, "   ")).toBeUndefined();
  });

  it("matches case-insensitively", () => {
    expect(exactMatchByName(items, "acme corp")).toEqual(items[0]);
    expect(exactMatchByName(items, "ACME CORP")).toEqual(items[0]);
  });

  it("matches with surrounding whitespace ignored on both sides", () => {
    expect(exactMatchByName(items, "  acme corp  ")).toEqual(items[0]);
    expect(exactMatchByName(items, "gamma")).toEqual(items[2]);
  });

  it("returns undefined when no item matches", () => {
    expect(exactMatchByName(items, "delta")).toBeUndefined();
  });

  it("returns the first match in input order on duplicate names", () => {
    const dups = [
      { id: 10, name: "Same" },
      { id: 11, name: "Same" },
    ];
    expect(exactMatchByName(dups, "same")).toEqual({ id: 10, name: "Same" });
  });
});
