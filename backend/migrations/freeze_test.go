package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The supported household installation freezes migrations 0001–0016.
// Append a checksum here for every later migration when it joins the chain.
var frozenMigrationChecksums = map[string]string{
	"0001_initial_schema.sql":                      "52f023a20c0f2df9451417029601ebd7bf371f1a7307b6fd0b4397b11d535459",
	"0002_category_version_delete_guard.sql":       "fa8ce2730765611d5a24d1627b4541db4d8019a570d167234985f96f0ba5235d",
	"0003_journal_entry_transfer_sides.sql":        "48ef1b18c230aedd3ebe85c6a6d1c2689fc6cbd4af37ef3afaa6ae5505ffd680",
	"0004_import_posting_links.sql":                "2cd6eeec17ea571febbfd1e4a39ed5fccd6cd3edb9bc8959b4497bcec5435c03",
	"0005_import_posting_link_history.sql":         "7e029b2e4b80e600ca9f12e1a19d33b909490015197a45a232162e4857a48704",
	"0006_import_posting_link_evidence.sql":        "0e0a6f4a4464ea4d77d32edd1dadd2387b36fcd0202137b259b8e464d1488e8a",
	"0007_investment_allocation.sql":               "11afdb718b1ea00ad59d5858faaa836275a60cd0c3b95b232300cb1a3d2474b7",
	"0008_allocation_history_references.sql":       "f660f0611f1297d91873a6b47594b05d3e109a370070beeec95c9e6f2b87eda6",
	"0009_retire_pricing_policy_base.sql":          "bb0e912ff316d3faade2c623c533462b5f496fe309403a9676395cd83be9bfd3",
	"0010_fx_work_after_holding_posting.sql":       "26f75af25ae8b8bb630c4b521bfab549b80da113b787a2c60f823f85353f96c8",
	"0011_fx_work_after_holding_demand_source.sql": "c5df5c938b47d8a9e89cf53e968732d5bd2fdcd39dcf449d3f260c902ab9c368",
	"0012_investment_split_facts_lookup.sql":       "8d887f79634e3d5cd6d66ea396ac483796be31ecc0c394d7f7bce4fe2d8d4092",
	"0013_reconciliation_policy.sql":               "d6c7f6445199cc8159b5a5c8cea55d3077278b95a3a6a02befec054f6d00302f",
	"0014_massive_split_source.sql":                "bf05a55c87678064ff17a990f4160dc0756a16ccf7171222d0ebd4ab0d774248",
	"0015_request_keys.sql":                        "527c00cac33a3bd3fc5a187eb3ae48ce24efd98d73edc250d43d3fcae22ef8ea",
	"0016_category_disable_day.sql":                "443074aaa93675230123bad07f58bcb5f862f0a7d2dee7f335eb484345cc0e22",
	"0017_home_layout.sql":                         "f9ac4b87616ad567c4d57921dd555d55498fd90e66a94373405c1522b6e33818",
}

func TestPinnedMigrationChecksums(t *testing.T) {
	names, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		assert.Containsf(t, frozenMigrationChecksums, name, "append-only migrations: pin the checksum for %s", name)
	}

	for name, want := range frozenMigrationChecksums {
		t.Run(name, func(t *testing.T) {
			contents, err := FS.ReadFile(name)
			require.NoErrorf(t, err, "append-only migrations: restore %s at its original path", name)
			sum := sha256.Sum256(contents)
			assert.Equalf(t, want, hex.EncodeToString(sum[:]),
				"append-only migrations: restore %s and put the change in the next migration", name)
		})
	}
}
