package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateRequestKeyBodyRefusesMalformedAndAmbiguousJSON(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"object":                      `{"a":1,"b":{"c":[1,{"d":2}],"e":"a"},"f":[{"a":1},{"a":2}]}`,
		"keys equal to values":        `{"a":"A","A_":"a","list":["a","A"]}`,
		"same name in sibling object": `{"x":{"name":1},"y":{"NAME":2}}`,
	} {
		assert.NoError(t, ValidateRequestKeyBody([]byte(body)), name)
	}
	for name, body := range map[string]string{
		"empty":                    ``,
		"trailing bracket":         `{"a":1}}`,
		"two values":               `{"a":1} {"a":1}`,
		"duplicate member":         `{"a":1,"a":2}`,
		"case-folded member":       `{"amount":"1","Amount":"2"}`,
		"unicode-folded member":    "{\"k\":1,\"K\":2}",
		"nested duplicate":         `{"x":[{"y":{"q":1,"Q":2}}]}`,
		"duplicate after an array": `{"a":[1,2],"b":3,"A":4}`,
	} {
		assert.Error(t, ValidateRequestKeyBody([]byte(body)), name)
	}
}

func TestRequestHashIsTheExactRouteAndBody(t *testing.T) {
	t.Parallel()
	body := []byte(`{"a":1,"b":2}`)
	assert.Equal(t, RequestHash("POST /x", body), RequestHash("POST /x", []byte(`{"a":1,"b":2}`)))
	assert.NotEqual(t, RequestHash("POST /x", body), RequestHash("POST /x", []byte(`{"b":2,"a":1}`)))
	assert.NotEqual(t, RequestHash("POST /x", body), RequestHash("POST /x", []byte(`{"a":1,"b":2} `)))
	assert.NotEqual(t, RequestHash("POST /x", body), RequestHash("POST /y", body))
}
