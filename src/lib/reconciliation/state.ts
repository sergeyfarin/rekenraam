export interface ReconciliationStateInput {
  openingBalanceMinor: number;
  checkedAmountMinor: number;
  statementBalanceMinor: number | null;
}

export interface ReconciliationState {
  clearedBalanceMinor: number;
  differenceMinor: number | null;
  needsOffset: boolean;
}

export function deriveReconciliationState(input: ReconciliationStateInput): ReconciliationState {
  const clearedBalanceMinor = input.openingBalanceMinor + input.checkedAmountMinor;
  const differenceMinor =
    input.statementBalanceMinor === null
      ? null
      : clearedBalanceMinor - input.statementBalanceMinor;
  const needsOffset = differenceMinor !== null && differenceMinor !== 0;
  return { clearedBalanceMinor, differenceMinor, needsOffset };
}

export interface CheckedCandidate<TxId> {
  id: TxId;
  splitAmountMinor: number;
}

export function sumCheckedAmounts<TxId>(
  candidates: ReadonlyArray<CheckedCandidate<TxId>>,
  checkedIds: ReadonlySet<TxId>,
): number {
  let total = 0;
  for (const candidate of candidates) {
    if (checkedIds.has(candidate.id)) {
      total += candidate.splitAmountMinor;
    }
  }
  return total;
}
