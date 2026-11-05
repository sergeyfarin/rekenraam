export type Missing = { counts: Record<string, number>; keys: Record<string, string[]> };
export const missingPath: URL;
export function computeMissing(): Missing;
export function render(missing: Missing): string;
