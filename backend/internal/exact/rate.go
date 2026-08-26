package exact

import (
	"fmt"
	"math/big"
)

// Converting money between commodities is a multiplication and a division, and
// both have to be exact until the single rounding step at the end. Doing it in
// two steps rounds twice; doing it in floating point rounds unpredictably.
//
// This is the shared helper the ledger invariants require for that: "division
// and implied prices round half-up via the shared helpers; never write ad-hoc
// rounding". app.scaledDivision predates it and is int64-bound, which is why
// this exists rather than that being reused.

// MulDivRound computes (value × multiplier) ÷ divisor, expressed at
// resultScale, rounded half-up on magnitude.
//
// Every operand carries its own scale, and none of them is assumed to match:
// an amount at 2 places, a rate at 6, and a base quantity at 0 is an ordinary
// combination. The whole computation happens in one big.Int expression so the
// only rounding is the final one.
//
// Half-up on magnitude, not toward positive infinity: -0.5 rounds to -1, so a
// credit and a debit of the same size convert to the same size. Rounding
// toward zero here would make a converted trial balance fail to balance in a
// way nobody could find.
func MulDivRound(
	value *big.Int, valueScale int,
	multiplier *big.Int, multiplierScale int,
	divisor *big.Int, divisorScale int,
	resultScale int,
) (*big.Int, error) {
	if value == nil || multiplier == nil || divisor == nil {
		return nil, fmt.Errorf("exact.MulDivRound: nil operand")
	}
	if divisor.Sign() == 0 {
		return nil, fmt.Errorf("exact.MulDivRound: division by zero")
	}
	if valueScale < 0 || multiplierScale < 0 || divisorScale < 0 || resultScale < 0 {
		return nil, fmt.Errorf("exact.MulDivRound: negative scale")
	}

	// value×10^-vs × multiplier×10^-ms ÷ (divisor×10^-ds), at 10^-rs, is
	//   (value × multiplier) ÷ divisor × 10^(rs + ds - vs - ms)
	// Scale whichever side keeps every digit available at the rounding
	// boundary, rather than truncating the numerator early.
	numerator := new(big.Int).Mul(value, multiplier)
	denominator := new(big.Int).Set(divisor)

	exponent := resultScale + divisorScale - valueScale - multiplierScale
	if exponent >= 0 {
		numerator.Mul(numerator, Pow10(exponent))
	} else {
		denominator.Mul(denominator, Pow10(-exponent))
	}

	sign := numerator.Sign() * denominator.Sign()
	absNumerator := new(big.Int).Abs(numerator)
	absDenominator := new(big.Int).Abs(denominator)

	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(absNumerator, absDenominator, remainder)
	// remainder×2 >= denominator means the fraction is at or past one half.
	if new(big.Int).Lsh(remainder, 1).Cmp(absDenominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if sign < 0 {
		quotient.Neg(quotient)
	}

	return quotient, nil
}
