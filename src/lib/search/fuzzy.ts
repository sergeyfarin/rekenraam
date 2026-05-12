export function normalizeName(value: string): string {
  return value.trim().toLowerCase();
}

export function fuzzyMatch(query: string, candidate: string): boolean {
  const q = normalizeName(query);
  if (!q) return true;
  const c = normalizeName(candidate);
  let cursor = 0;
  for (const ch of c) {
    if (ch === q[cursor]) {
      cursor += 1;
      if (cursor === q.length) return true;
    }
  }
  return false;
}

export const FUZZY_OPTIONS_LIMIT = 30;

export function fuzzyOptions<T extends { id: number; name: string }>(
  items: ReadonlyArray<T>,
  query: string,
): T[] {
  return items.filter((item) => fuzzyMatch(query, item.name)).slice(0, FUZZY_OPTIONS_LIMIT);
}

export function exactMatchByName<T extends { id: number; name: string }>(
  items: ReadonlyArray<T>,
  query: string,
): T | undefined {
  const needle = normalizeName(query);
  if (!needle) return undefined;
  return items.find((item) => normalizeName(item.name) === needle);
}
