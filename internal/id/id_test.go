package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash_Deterministic(t *testing.T) {
	a := Hash("func main")
	b := Hash("func main")
	assert.Equal(t, a, b)
	assert.Len(t, a, 2)
}

func TestHash_DifferentInputs(t *testing.T) {
	a := Hash("func main")
	b := Hash("func handleRequest")
	assert.NotEqual(t, a, b)
}

func TestAssignIDs_NoDuplicates(t *testing.T) {
	ids := AssignIDs([]string{"func main", "type Config", "func handleRequest"})
	assert.Len(t, ids, 3)
	seen := map[string]bool{}
	for _, id := range ids {
		assert.False(t, seen[id], "duplicate ID: %s", id)
		seen[id] = true
	}
}

func TestAssignIDs_CollisionExtends(t *testing.T) {
	ids := AssignIDs([]string{"func String", "func String"})
	assert.Len(t, ids[0], 3)
	assert.Len(t, ids[1], 3)
	assert.NotEqual(t, ids[0], ids[1])
}
