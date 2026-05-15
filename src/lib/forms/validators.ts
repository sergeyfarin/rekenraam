import { parseAmountToMinor } from "$lib/money";

export type ValidationResult<T> =
  | { ok: true; value: T }
  | { ok: false; error: string };

export function ok<T>(value: T): ValidationResult<T> {
  return { ok: true, value };
}

export function err<T = never>(error: string): ValidationResult<T> {
  return { ok: false, error };
}

export function validateNonEmptyString(
  value: string | null | undefined,
  fieldName = "Value",
): ValidationResult<string> {
  if (value === null || value === undefined) return err(`${fieldName} is required`);
  const trimmed = value.trim();
  if (!trimmed) return err(`${fieldName} is required`);
  return ok(trimmed);
}

export function validatePositiveAmountMinor(
  value: string,
  scale: number,
  fieldName = "Amount",
): ValidationResult<number> {
  const minor = parseAmountToMinor(value, scale);
  if (minor === null) return err(`${fieldName} must be a valid number`);
  if (minor <= 0) return err(`${fieldName} must be greater than zero`);
  return ok(minor);
}

export function validateNonZeroAmountMinor(
  value: string,
  scale: number,
  fieldName = "Amount",
): ValidationResult<number> {
  const minor = parseAmountToMinor(value, scale);
  if (minor === null) return err(`${fieldName} must be a valid number`);
  if (minor === 0) return err(`${fieldName} cannot be zero`);
  return ok(minor);
}

export function validateScaleAwareAmountMinor(
  value: string,
  scale: number,
  fieldName = "Amount",
): ValidationResult<number> {
  const minor = parseAmountToMinor(value, scale);
  if (minor === null) return err(`${fieldName} must be a valid number`);
  return ok(minor);
}

const ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

export function isValidIsoDate(value: string): boolean {
  if (typeof value !== "string" || !ISO_DATE_RE.test(value)) return false;
  const [y, m, d] = value.split("-").map(Number);
  if (m < 1 || m > 12 || d < 1 || d > 31) return false;
  const dt = new Date(Date.UTC(y, m - 1, d));
  return (
    dt.getUTCFullYear() === y &&
    dt.getUTCMonth() === m - 1 &&
    dt.getUTCDate() === d
  );
}

export function validateIsoDate(
  value: string | null | undefined,
  fieldName = "Date",
): ValidationResult<string> {
  if (!value) return err(`${fieldName} is required`);
  if (!isValidIsoDate(value)) return err(`${fieldName} must be a valid ISO date (YYYY-MM-DD)`);
  return ok(value);
}

export function validateIntegerInRange(
  value: number | null | undefined,
  options: { min?: number; max?: number; fieldName?: string } = {},
): ValidationResult<number> {
  const { min, max, fieldName = "Value" } = options;
  if (value === null || value === undefined) return err(`${fieldName} is required`);
  if (!Number.isFinite(value) || !Number.isInteger(value)) {
    return err(`${fieldName} must be an integer`);
  }
  if (min !== undefined && value < min) return err(`${fieldName} must be at least ${min}`);
  if (max !== undefined && value > max) return err(`${fieldName} must be at most ${max}`);
  return ok(value);
}

export function combine<T extends Record<string, ValidationResult<unknown>>>(
  results: T,
): ValidationResult<{ [K in keyof T]: T[K] extends ValidationResult<infer U> ? U : never }> {
  const values = {} as { [K in keyof T]: unknown };
  const errors: string[] = [];
  for (const key of Object.keys(results) as (keyof T)[]) {
    const result = results[key];
    if (result.ok) {
      values[key] = result.value;
    } else {
      errors.push(`${String(key)}: ${result.error}`);
    }
  }
  if (errors.length > 0) return err(errors.join("; "));
  return ok(values as { [K in keyof T]: T[K] extends ValidationResult<infer U> ? U : never });
}
