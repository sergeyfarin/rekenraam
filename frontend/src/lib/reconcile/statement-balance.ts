import { parseDecimalAmount, type ScaledAmount } from '$lib/money/amount';

/**
 * Parse a user-entered statement balance into a lossless debit-positive integer
 * coefficient + scale.
 *
 * A thin alias over the shared money parser (`$lib/money/amount`), kept so the
 * reconcile form reads in its own vocabulary. All parsing rules — canonical
 * coefficients, unsigned zero, thousands grouping, digit and scale ceilings —
 * live there and are tested there.
 */
export function parseStatementBalance(input: string): ScaledAmount | null {
	return parseDecimalAmount(input);
}
