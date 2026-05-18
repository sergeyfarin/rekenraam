import { get } from "svelte/store";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { activeBookId, bookContext, initializeBookContext, requireActiveBookId, setActiveBook } from "$lib/book-context";
import { listBooks } from "$lib/api/books";
import { getPreferences, updatePreferences } from "$lib/api/preferences";

vi.mock("$lib/api/books", () => ({
  listBooks: vi.fn(),
}));

vi.mock("$lib/api/preferences", () => ({
  getPreferences: vi.fn(),
  updatePreferences: vi.fn(),
}));

const books = [
  { id: 1, slug: "personal", name: "Personal", base_currency_code: "USD" },
  { id: 2, slug: "household", name: "Household", base_currency_code: "EUR" },
];

beforeEach(() => {
  bookContext.set({ books: [], activeBook: null, loading: false, error: null });
  activeBookId.set(null);
  vi.mocked(listBooks).mockResolvedValue(books);
  vi.mocked(getPreferences).mockResolvedValue({
    default_book_id: null,
    locale: "en-US",
    date_format: "iso",
    number_format: "system",
    theme: "system",
  });
  vi.mocked(updatePreferences).mockResolvedValue({
    default_book_id: 2,
    locale: "en-US",
    date_format: "iso",
    number_format: "system",
    theme: "system",
  });
});

describe("book context", () => {
  it("falls back to the first readable book", async () => {
    await initializeBookContext();

    expect(get(activeBookId)).toBe(1);
    expect(get(bookContext).activeBook?.slug).toBe("personal");
  });

  it("honors the preferred default book when available", async () => {
    vi.mocked(getPreferences).mockResolvedValueOnce({
      default_book_id: 2,
      locale: "en-US",
      date_format: "iso",
      number_format: "system",
      theme: "system",
    });

    await initializeBookContext();

    expect(get(activeBookId)).toBe(1);
  });

  it("uses the preferred default when book 1 is not readable", async () => {
    vi.mocked(listBooks).mockResolvedValueOnce([books[1]]);
    vi.mocked(getPreferences).mockResolvedValueOnce({
      default_book_id: 2,
      locale: "en-US",
      date_format: "iso",
      number_format: "system",
      theme: "system",
    });

    await initializeBookContext();

    expect(get(activeBookId)).toBe(2);
  });

  it("persists active book changes", async () => {
    await initializeBookContext();
    await setActiveBook(2);

    expect(get(activeBookId)).toBe(2);
    expect(vi.mocked(updatePreferences)).toHaveBeenCalledWith({ default_book_id: 2 });
  });

  it("throws when no active book has been initialized", () => {
    expect(() => requireActiveBookId()).toThrow("Active book is not initialized.");
  });
});
