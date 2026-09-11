package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frozenMigrationChecksums is the v0.1 schema contract. Existing entries are
// immutable; future schema work adds a new numbered migration and, when that
// migration ships in a release, a new checksum here.
var frozenMigrationChecksums = map[string]string{
	"0001_initial_schema.sql": "3eb4b719e0eeda7b447466099c4287a602b38961fe99a81e24145f435a7da724",
}

func TestReleasedMigrationsAreImmutable(t *testing.T) {
	for name, want := range frozenMigrationChecksums {
		contents, err := FS.ReadFile(name)
		require.NoErrorf(t, err, "released migration %s must not be renamed or removed", name)
		sum := sha256.Sum256(contents)
		assert.Equalf(t, want, hex.EncodeToString(sum[:]),
			"released migration %s changed; restore it and add a new sequential migration", name)
	}
}
