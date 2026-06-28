package gateway

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeCustomModels(t *testing.T) {
	existing := []customModelEntry{{ID: "a", Name: "A", Kind: "llm"}}
	add := []customModelEntry{{ID: "b", Name: "B"}, {ID: "a", Name: "A2", Kind: "embedding"}}
	merged := mergeCustomModels(existing, add)
	require.Len(t, merged, 2)
	require.Equal(t, "a", merged[0].ID)
	require.Equal(t, "A2", merged[0].Name)
	require.Equal(t, "embedding", merged[0].Kind)
	require.Equal(t, "b", merged[1].ID)
	require.Equal(t, "llm", merged[1].Kind)
}

func TestNormalizeCustomModelKind(t *testing.T) {
	require.Equal(t, "llm", normalizeCustomModelKind(""))
	require.Equal(t, "llm", normalizeCustomModelKind("bogus"))
	require.Equal(t, "embedding", normalizeCustomModelKind("embedding"))
}
