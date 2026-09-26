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
	"0001_initial_schema.sql": "5026aba4ec089af81a7bb8846a6e1b823f70afbf5de6d39ee79f0a49949258f1",
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
