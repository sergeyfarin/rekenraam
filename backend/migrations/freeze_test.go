package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frozenMigrationChecksums pins the current v0.1 candidate baseline. ADR 0013
// permits an explicitly declared pre-release redesign before installations;
// installed-release migrations are immutable.
var frozenMigrationChecksums = map[string]string{
	"0001_initial_schema.sql": "1b5d3c942e3fe04e19f18c6d24c1c3e44dcb5246319d4406037ee6e17228613e",
}

func TestPinnedMigrationChecksums(t *testing.T) {
	for name, want := range frozenMigrationChecksums {
		contents, err := FS.ReadFile(name)
		require.NoErrorf(t, err, "pinned migration %s must not be renamed or removed", name)
		sum := sha256.Sum256(contents)
		assert.Equalf(t, want, hex.EncodeToString(sum[:]),
			"pinned migration %s changed; restore it or document an ADR-approved pre-release redesign and update this checksum", name)
	}
}
