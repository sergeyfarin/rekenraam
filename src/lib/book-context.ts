import { get, writable } from "svelte/store";
import { listBooks, type BookSummary } from "$lib/api/books";
import { getPreferences, updatePreferences } from "$lib/api/preferences";

export type BookContextState = {
  books: BookSummary[];
  activeBook: BookSummary | null;
  loading: boolean;
  error: string | null;
};

const initialState: BookContextState = {
  books: [],
  activeBook: null,
  loading: false,
  error: null,
};

export const bookContext = writable<BookContextState>(initialState);
export const activeBookId = writable<number | null>(null);

let requestSeq = 0;

export async function initializeBookContext(): Promise<BookContextState> {
  const seq = ++requestSeq;
  bookContext.update((state) => ({ ...state, loading: true, error: null }));
  try {
    const [books, preferences] = await Promise.all([listBooks(), getPreferences()]);
    const preferredId = preferences.default_book_id;
    const activeBook =
      books.find((book) => book.id === 1) ??
      books.find((book) => book.id === preferredId) ??
      books[0] ??
      null;
    const next = { books, activeBook, loading: false, error: null };
    if (seq === requestSeq) {
      bookContext.set(next);
      activeBookId.set(activeBook?.id ?? null);
    }
    return next;
  } catch (error) {
    const message = error instanceof Error ? error.message : "Failed to load books";
    const next = { ...get(bookContext), loading: false, error: message };
    if (seq === requestSeq) {
      bookContext.set(next);
      activeBookId.set(next.activeBook?.id ?? null);
    }
    return next;
  }
}

export async function setActiveBook(bookId: number): Promise<void> {
  const state = get(bookContext);
  const activeBook = state.books.find((book) => book.id === bookId);
  if (!activeBook) {
    throw new Error("Book not found");
  }
  bookContext.set({ ...state, activeBook, error: null });
  activeBookId.set(activeBook.id);
  await updatePreferences({ default_book_id: activeBook.id });
}

export function requireActiveBookId(): number {
  const bookId = get(activeBookId);
  if (bookId === null) {
    throw new Error("Active book is not initialized.");
  }
  return bookId;
}
