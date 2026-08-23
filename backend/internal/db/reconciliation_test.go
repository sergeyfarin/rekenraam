package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// distinctPositiveCount backs the guard in currentReconciliationPostingsByID,
// which rejects a request whose requested posting ids do not all resolve to
// eligible rows. Only the count matters there, so these cases pin the counting
// rule rather than any ordering: duplicates collapse, and non-positive ids are
// not counted even though the query still builds a placeholder for them.
//
// The app layer already rejects non-positive ids in cleanPositiveInt64IDs and
// the other caller derives ids from primary keys, so the non-positive cases
// below are defence in depth. They are pinned anyway: this is a reconciliation
// guard, and a silent widening here would let a malformed request through.
func TestDistinctPositiveCount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, distinctPositiveCount(nil))
	assert.Equal(t, 0, distinctPositiveCount([]int64{}))
	assert.Equal(t, 3, distinctPositiveCount([]int64{1, 2, 3}))
	assert.Equal(t, 3, distinctPositiveCount([]int64{3, 1, 2}), "order must not change the count")
	assert.Equal(t, 2, distinctPositiveCount([]int64{7, 7, 4}), "duplicates collapse")
	assert.Equal(t, 1, distinctPositiveCount([]int64{5, 5, 5}))
	assert.Equal(t, 0, distinctPositiveCount([]int64{0, -1, -9}), "non-positive ids are not counted")
	assert.Equal(t, 1, distinctPositiveCount([]int64{0, -1, 6}))
}

// The caller must keep its own slice intact: currentReconciliationPostingsByID
// counts the ids after it has already built the query's placeholders and args
// from that same slice.
func TestDistinctPositiveCountDoesNotMutateItsInput(t *testing.T) {
	t.Parallel()

	ids := []int64{9, 3, 9, 1}
	assert.Equal(t, 3, distinctPositiveCount(ids))
	assert.Equal(t, []int64{9, 3, 9, 1}, ids)
}
