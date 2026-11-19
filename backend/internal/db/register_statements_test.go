package db

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #595. A register cursor can stop on a statement row. Three-part cursors stay
// posting cursors, so a cursor issued before statement rows still pages.
func TestRegisterCursorDecodesPostingAndStatementKinds(t *testing.T) {
	t.Parallel()

	posting, err := DecodeRegisterCursor(EncodeRegisterCursor("2026-03-31", 2, 41))
	require.NoError(t, err)
	assert.Equal(t, RegisterCursor{Date: "2026-03-31", DaySequence: 2, ID: 41}, posting)

	statement, err := DecodeRegisterCursor(EncodeRegisterStatementCursor("2026-03-31", 0, 7))
	require.NoError(t, err)
	assert.Equal(t, RegisterCursor{Date: "2026-03-31", DaySequence: 0, ID: 7, Statement: true}, statement)

	empty, err := DecodeRegisterCursor("  ")
	require.NoError(t, err)
	assert.Equal(t, RegisterCursor{}, empty)

	for name, raw := range map[string]string{
		"unknown kind": "2026-03-31|1|5|x",
		"five parts":   "2026-03-31|1|5|s|s",
		"two parts":    "2026-03-31|1",
		"zero id":      "2026-03-31|1|0|s",
		"negative seq": "2026-03-31|-1|5|s",
	} {
		_, err := DecodeRegisterCursor(base64.RawURLEncoding.EncodeToString([]byte(raw)))
		assert.Error(t, err, name)
	}
	_, err = DecodeRegisterCursor("not base64!")
	assert.Error(t, err)
}
