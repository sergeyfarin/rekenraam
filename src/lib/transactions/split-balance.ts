import { parseAmountToMinor } from "$lib/money";

export interface SplitInput {
  amount: string;
  scale: number;
}

export function sumSplitsInMinor(splits: ReadonlyArray<SplitInput>): number | null {
  let total = 0;
  for (const split of splits) {
    const minor = parseAmountToMinor(split.amount, split.scale);
    if (minor === null) return null;
    total += minor;
  }
  return total;
}

export function isSplitsBalanced(splits: ReadonlyArray<SplitInput>): boolean {
  const total = sumSplitsInMinor(splits);
  return total === 0;
}
