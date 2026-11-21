import { accountNormalCoefficient, parseDecimalAmount, type ScaledAmount } from '#lib/money/amount.ts';

/** Parse an account-normal balance and convert it to the API's debit-positive sign. */
export function parseStatementBalance(input: string, accountClass = 'asset'): ScaledAmount | null {
	const parsed = parseDecimalAmount(input);
	return parsed && { ...parsed, value: accountNormalCoefficient(parsed.value, accountClass) };
}
