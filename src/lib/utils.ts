import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * Structured error returned by Tauri commands that use AppError.
 * Matches the Rust `#[serde(tag = "type", content = "data")]` serialization.
 */
type AppError =
  | { type: "Validation"; data: { field: string; message: string } }
  | { type: "NotFound"; data: { entity: string; id: string } }
  | { type: "Conflict"; data: { message: string } }
  | { type: "Database"; data: { message: string } }
  | { type: "Internal"; data: { message: string } };

/**
 * Convert a Tauri command error to a human-readable string.
 * Handles structured AppError objects as well as plain strings/errors.
 */
export function formatError(e: unknown): string {
  if (e === null || e === undefined) return "Unknown error";
  if (typeof e === "string") return e;
  if (typeof e === "object") {
    const err = e as Record<string, unknown>;
    if (typeof err["type"] === "string" && err["data"] !== undefined) {
      const ae = e as AppError;
      switch (ae.type) {
        case "Validation":
          return `${ae.data.field}: ${ae.data.message}`;
        case "NotFound":
          return `${ae.data.entity} not found (id: ${ae.data.id})`;
        case "Conflict":
          return ae.data.message;
        case "Database":
          return `Database error: ${ae.data.message}`;
        case "Internal":
          return ae.data.message;
      }
    }
    // Fallback: try JSON serialization
    try { return JSON.stringify(e); } catch { /* ignore */ }
  }
  return String(e);
}

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, "child"> : T;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, "children"> : T;
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>;
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null };
