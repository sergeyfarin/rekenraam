import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import { ApiError } from "$lib/api/client";

/**
 * Structured legacy error shape retained while old migration-reference flows are retired.
 */
type AppError =
  | { type: "Validation"; data: { field: string; message: string } }
  | { type: "NotFound"; data: { entity: string; id: string } }
  | { type: "Conflict"; data: { message: string } }
  | { type: "Database"; data: { message: string } }
  | { type: "Internal"; data: { message: string } };

/**
 * Convert an unknown API or legacy command error to a human-readable string.
 */
export function formatError(e: unknown): string {
  if (e === null || e === undefined) return "Unknown error";
  if (typeof e === "string") return e;
  if (e instanceof ApiError) return formatApiError(e);
  if (e instanceof Error) return e.message || e.name;
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
    try {
      const serialized = JSON.stringify(e);
      if (serialized && serialized !== "{}") {
        return serialized;
      }
    } catch {
      // Ignore JSON parse failures and fall through.
    }
  }
  return String(e);
}

export function formatApiError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401) return "Your session has expired. Sign in again to continue.";
    if (e.status === 403) return "You do not have permission to perform this action.";
    if (e.status === 409) return e.detail || "This change conflicts with current data. Refresh and try again.";
    if (e.status === 422 && e.validationErrors.length > 0) {
      return e.validationErrors
        .map((item) => {
          const path = item.loc?.filter((part) => part !== "body").join(".");
          return path ? `${path}: ${item.msg}` : item.msg;
        })
        .join("; ");
    }
    if (e.status >= 500) {
      return e.requestId
        ? `Server error (${e.requestId}). Try again or check the server logs.`
        : "Server error. Try again or check the server logs.";
    }
    return e.detail;
  }

  if (e instanceof TypeError && /fetch|network|failed/i.test(e.message)) {
    return "Backend unavailable. Check the API service and try again.";
  }

  return formatError(e);
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
