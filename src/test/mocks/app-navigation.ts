import { vi } from "vitest";

export const goto = vi.fn(async (_url: string | URL) => {});
export const invalidate = vi.fn(async () => {});
export const invalidateAll = vi.fn(async () => {});
export const beforeNavigate = vi.fn();
export const afterNavigate = vi.fn();
export const preloadData = vi.fn(async () => ({ type: "loaded" }));
export const preloadCode = vi.fn(async () => {});
export const pushState = vi.fn();
export const replaceState = vi.fn();
export const disableScrollHandling = vi.fn();
