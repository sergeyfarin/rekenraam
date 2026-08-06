package exact

import (
	"errors"
	"fmt"
	"math/big"
)

// MaxPow10Scale caps the exponent accepted by Pow10. No legitimate financial
// quantity or alignment delta should exceed this; a larger value indicates a
// logic bug in the caller, not a data problem.
const MaxPow10Scale = 60

// pow10Cache holds precomputed 10^i for i in [0, MaxPow10Scale].
var pow10Cache = func() [MaxPow10Scale + 1]*big.Int {
	var t [MaxPow10Scale + 1]*big.Int
	t[0] = big.NewInt(1)
	for i := 1; i <= MaxPow10Scale; i++ {
		t[i] = new(big.Int).Mul(t[i-1], big.NewInt(10))
	}
	return t
}()

// Pow10 returns a fresh 10^scale. It panics outside [0, MaxPow10Scale] because
// every caller derives the exponent from a scale difference that is already
// bounded by the commodity scale ceilings.
func Pow10(scale int) *big.Int {
	if scale < 0 || scale > MaxPow10Scale {
		panic(fmt.Sprintf("exact.Pow10: scale %d out of range [0, %d]", scale, MaxPow10Scale))
	}
	return new(big.Int).Set(pow10Cache[scale])
}

// ErrInt64Range reports a scaled value that cannot be stored in an int64
// column. Callers wrap it with the name of the value that overflowed.
var ErrInt64Range = errors.New("value exceeds int64 range")

// ScaledInt is a mutable accumulator for exact scaled quantities: an arbitrary
// precision coefficient plus a decimal scale. It is the single place where
// coefficient + scale arithmetic is implemented.
//
// Use it for every sum, difference, or comparison of money and quantities that
// may arrive at different scales. Adding raw int64 coefficients whose scales
// differ silently corrupts the result by factors of ten — that defect class
// (audit 2026-07-13, F1–F6) is exactly what this type exists to prevent.
//
// The zero value is not usable; construct with NewScaledInt, ScaledIntFromBig
// or ScaledIntFromInt64. Operations align to the deeper (larger) scale, so no
// precision is ever discarded implicitly.
type ScaledInt struct {
	value *big.Int
	scale int
}

// NewScaledInt returns a zero accumulator at scale 0.
func NewScaledInt() *ScaledInt {
	return &ScaledInt{value: big.NewInt(0)}
}

// ScaledIntFromBig copies value into a new accumulator at the given scale.
func ScaledIntFromBig(value *big.Int, scale int) *ScaledInt {
	s := NewScaledInt()
	s.Add(value, scale)
	return s
}

// ScaledIntFromInt64 returns a new accumulator holding value at the given scale.
func ScaledIntFromInt64(value int64, scale int) *ScaledInt {
	return ScaledIntFromBig(big.NewInt(value), scale)
}

// ScaledIntFromCoefficient returns a new accumulator holding value at the given
// scale.
func ScaledIntFromCoefficient(value Coefficient, scale int) *ScaledInt {
	return ScaledIntFromBig(value.BigInt(), scale)
}

// Scale returns the accumulator's current scale.
func (s *ScaledInt) Scale() int { return s.scale }

// BigInt returns a copy of the coefficient at the current scale.
func (s *ScaledInt) BigInt() *big.Int { return new(big.Int).Set(s.value) }

// Sign reports the sign of the accumulated value (-1, 0 or +1).
func (s *ScaledInt) Sign() int { return s.value.Sign() }

// Align deepens the accumulator to scale, multiplying the coefficient so the
// represented value is unchanged. Shallower scales are ignored: alignment only
// ever adds precision, never drops it.
func (s *ScaledInt) Align(scale int) {
	if scale < 0 {
		panic(fmt.Sprintf("exact.ScaledInt.Align: negative scale %d", scale))
	}
	if scale <= s.scale {
		return
	}
	s.value.Mul(s.value, Pow10(scale-s.scale))
	s.scale = scale
}

// Add accumulates value at the given scale, aligning both sides to the deeper
// scale first. value is not mutated.
func (s *ScaledInt) Add(value *big.Int, scale int) {
	if value == nil {
		return
	}
	s.Align(scale)
	addend := new(big.Int).Set(value)
	if scale < s.scale {
		addend.Mul(addend, Pow10(s.scale-scale))
	}
	s.value.Add(s.value, addend)
}

// AddCoefficient accumulates a coefficient at the given scale.
func (s *ScaledInt) AddCoefficient(value Coefficient, scale int) {
	s.Add(value.BigInt(), scale)
}

// AddInt64 accumulates an int64 coefficient at the given scale.
func (s *ScaledInt) AddInt64(value int64, scale int) {
	s.Add(big.NewInt(value), scale)
}

// AddScaled accumulates another accumulator. A nil operand is a no-op.
func (s *ScaledInt) AddScaled(other *ScaledInt) {
	if other == nil {
		return
	}
	s.Add(other.value, other.scale)
}

// SubScaled subtracts another accumulator. A nil operand is a no-op.
func (s *ScaledInt) SubScaled(other *ScaledInt) {
	if other == nil {
		return
	}
	s.Add(new(big.Int).Neg(other.value), other.scale)
}

// Negated returns a new accumulator holding the negated value at the same scale.
func (s *ScaledInt) Negated() *ScaledInt {
	return &ScaledInt{value: new(big.Int).Neg(s.value), scale: s.scale}
}

// Cmp compares two scaled values by magnitude, independent of their scales,
// returning -1, 0 or +1. Neither operand is mutated.
func (s *ScaledInt) Cmp(other *ScaledInt) int {
	left, right := s.value, other.value
	switch {
	case s.scale < other.scale:
		left = new(big.Int).Mul(left, Pow10(other.scale-s.scale))
	case s.scale > other.scale:
		right = new(big.Int).Mul(right, Pow10(s.scale-other.scale))
	}
	return left.Cmp(right)
}

// TruncatedTo returns a copy of the value restated at scale. Deepening is
// lossless; shallowing truncates toward zero, so only call it with a scale the
// caller has decided is the correct output precision.
func (s *ScaledInt) TruncatedTo(scale int) *ScaledInt {
	if scale < 0 {
		panic(fmt.Sprintf("exact.ScaledInt.TruncatedTo: negative scale %d", scale))
	}
	restated := &ScaledInt{value: new(big.Int).Set(s.value), scale: s.scale}
	switch {
	case scale > s.scale:
		restated.Align(scale)
	case scale < s.scale:
		restated.value.Quo(restated.value, Pow10(s.scale-scale))
		restated.scale = scale
	}
	return restated
}

// Int64 returns the coefficient as an int64, or ErrInt64Range if it does not
// fit. Callers storing into an int64 column must use this rather than
// BigInt().Int64(), which wraps silently.
func (s *ScaledInt) Int64() (int64, error) {
	if !s.value.IsInt64() {
		return 0, ErrInt64Range
	}
	return s.value.Int64(), nil
}

// Coefficient returns the accumulated value as an exact.Coefficient, rejecting
// results wider than MaxCoefficientDigits.
func (s *ScaledInt) Coefficient() (Coefficient, error) {
	return FromBig(s.value)
}

func (s *ScaledInt) String() string {
	return fmt.Sprintf("%se-%d", s.value.String(), s.scale)
}
