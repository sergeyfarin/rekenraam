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
	"0001_initial_schema.sql": "82b465589d4e8ca53e6b5833e1cb672c25428d0e46331260ef5a4e974cf99a8c",
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
