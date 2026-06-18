package exact

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	MaxCoefficientDigits = 38
	MaxStandardScale     = 12
	MaxCryptoScale       = 24
)

func MaxScaleForCommodityKind(kind string) int {
	if kind == "crypto" {
		return MaxCryptoScale
	}
	return MaxStandardScale
}

// Coefficient is a signed, base-10 integer coefficient for an exact scaled
// quantity. Its JSON and SQLite representations are strings so values remain
// lossless outside Go's and JavaScript's native integer ranges.
type Coefficient string

func New(value int64) Coefficient {
	return Coefficient(strconv.FormatInt(value, 10))
}

func FromBig(value *big.Int) (Coefficient, error) {
	if value == nil {
		return "", errors.New("coefficient is required")
	}
	return Parse(value.String())
}

func MustParse(value string) Coefficient {
	coefficient, err := Parse(value)
	if err != nil {
		panic(err)
	}
	return coefficient
}

func Parse(value string) (Coefficient, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("coefficient is required")
	}
	integer := new(big.Int)
	if _, ok := integer.SetString(value, 10); !ok {
		return "", errors.New("coefficient must be a signed base-10 integer")
	}
	canonical := integer.String()
	digits := strings.TrimPrefix(canonical, "-")
	if len(digits) > MaxCoefficientDigits {
		return "", fmt.Errorf("coefficient must contain at most %d digits", MaxCoefficientDigits)
	}
	return Coefficient(canonical), nil
}

func (c Coefficient) String() string {
	if c == "" {
		return "0"
	}
	return string(c)
}

func (c Coefficient) BigInt() *big.Int {
	value := new(big.Int)
	value.SetString(c.String(), 10)
	return value
}

func (c Coefficient) Sign() int { return c.BigInt().Sign() }

func (c Coefficient) Negated() Coefficient {
	return Coefficient(new(big.Int).Neg(c.BigInt()).String())
}

func (c Coefficient) Cmp(other Coefficient) int {
	return c.BigInt().Cmp(other.BigInt())
}

func (c Coefficient) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *Coefficient) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("coefficient destination is nil")
	}
	data = bytes.TrimSpace(data)
	var raw string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
	} else {
		raw = string(data)
	}
	parsed, err := Parse(raw)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

func (c Coefficient) Value() (driver.Value, error) {
	return c.String(), nil
}

func (c *Coefficient) Scan(source any) error {
	if c == nil {
		return errors.New("coefficient destination is nil")
	}
	var raw string
	switch value := source.(type) {
	case int64:
		raw = strconv.FormatInt(value, 10)
	case string:
		raw = value
	case []byte:
		raw = string(value)
	case nil:
		return errors.New("coefficient cannot scan NULL")
	default:
		return fmt.Errorf("coefficient cannot scan %T", source)
	}
	parsed, err := Parse(raw)
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}
