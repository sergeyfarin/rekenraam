package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
)

// moneyCoefficient keeps the existing int64 investment arithmetic while
// carrying its coefficient as a decimal string across JSON. Numeric input is
// accepted for older API clients; every response uses a string.
type moneyCoefficient int64

func (v moneyCoefficient) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(v), 10))
}

func (v *moneyCoefficient) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	var raw string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
	} else {
		raw = string(data)
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || strconv.FormatInt(parsed, 10) != raw {
		return errors.New("money coefficient must be a canonical int64 decimal string")
	}
	*v = moneyCoefficient(parsed)
	return nil
}

func moneyCoefficientPointer(value *int64) *moneyCoefficient {
	if value == nil {
		return nil
	}
	converted := moneyCoefficient(*value)
	return &converted
}

func moneyInt64Pointer(value *moneyCoefficient) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}
