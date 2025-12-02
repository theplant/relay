package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateComplexity(t *testing.T) {
	tests := []struct {
		name     string
		filter   map[string]any
		expected *ComplexityResult
	}{
		{
			name:   "empty filter",
			filter: map[string]any{},
			expected: &ComplexityResult{
				Depth:            1,
				TotalFields:      0,
				LogicalOperators: 0,
				LogicalDepth:     0,
				OrBranches:       0,
			},
		},
		{
			name: "simple field filter",
			filter: map[string]any{
				"Name": map[string]any{"Eq": "test"},
			},
			expected: &ComplexityResult{
				Depth:            1,
				TotalFields:      1,
				LogicalOperators: 0,
				LogicalDepth:     0,
				OrBranches:       0,
			},
		},
		{
			name: "multiple field filters",
			filter: map[string]any{
				"Name":   map[string]any{"Eq": "test"},
				"Status": map[string]any{"In": []string{"ACTIVE", "PENDING"}},
				"Code":   map[string]any{"Contains": "ABC"},
			},
			expected: &ComplexityResult{
				Depth:            1,
				TotalFields:      3,
				LogicalOperators: 0,
				LogicalDepth:     0,
				OrBranches:       0,
			},
		},
		{
			name: "relationship filter - depth 2",
			filter: map[string]any{
				"Category": map[string]any{
					"Name": map[string]any{"Eq": "Electronics"},
				},
			},
			expected: &ComplexityResult{
				Depth:            2,
				TotalFields:      1,
				LogicalOperators: 0,
				LogicalDepth:     0,
				OrBranches:       0,
			},
		},
		{
			name: "nested relationship filter - depth 3",
			filter: map[string]any{
				"Category": map[string]any{
					"Parent": map[string]any{
						"Name": map[string]any{"Eq": "Root"},
					},
				},
			},
			expected: &ComplexityResult{
				Depth:            3,
				TotalFields:      1,
				LogicalOperators: 0,
				LogicalDepth:     0,
				OrBranches:       0,
			},
		},
		{
			name: "simple Or with 3 branches",
			filter: map[string]any{
				"Or": []any{
					map[string]any{"Name": map[string]any{"Eq": "A"}},
					map[string]any{"Name": map[string]any{"Eq": "B"}},
					map[string]any{"Name": map[string]any{"Eq": "C"}},
				},
			},
			expected: &ComplexityResult{
				Depth:            1,
				TotalFields:      3,
				LogicalOperators: 1,
				LogicalDepth:     1,
				OrBranches:       3,
			},
		},
		{
			name: "nested logical operators",
			filter: map[string]any{
				"And": []any{
					map[string]any{
						"Or": []any{
							map[string]any{"Name": map[string]any{"Eq": "A"}},
							map[string]any{"Name": map[string]any{"Eq": "B"}},
						},
					},
					map[string]any{
						"Not": map[string]any{
							"Status": map[string]any{"Eq": "DELETED"},
						},
					},
				},
			},
			expected: &ComplexityResult{
				Depth:            1,
				TotalFields:      3,
				LogicalOperators: 3,
				LogicalDepth:     2,
				OrBranches:       2,
			},
		},
		{
			name: "complex filter with relationship and logical",
			filter: map[string]any{
				"Name": map[string]any{"Contains": "test"},
				"Category": map[string]any{
					"Or": []any{
						map[string]any{"Name": map[string]any{"Eq": "A"}},
						map[string]any{"Code": map[string]any{"Eq": "B"}},
					},
				},
			},
			expected: &ComplexityResult{
				Depth:            2,
				TotalFields:      3,
				LogicalOperators: 1,
				LogicalDepth:     1,
				OrBranches:       2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateComplexity(tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckComplexity(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		filter := map[string]any{
			"Name":   map[string]any{"Eq": "test"},
			"Status": map[string]any{"In": []string{"ACTIVE"}},
		}
		err := CheckComplexity(filter, DefaultLimits)
		require.NoError(t, err)
	})

	t.Run("exceeds depth limit", func(t *testing.T) {
		filter := map[string]any{
			"A": map[string]any{
				"B": map[string]any{
					"C": map[string]any{
						"Name": map[string]any{"Eq": "test"},
					},
				},
			},
		}
		err := CheckComplexity(filter, &ComplexityLimits{MaxDepth: 2})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "depth")
	})

	t.Run("exceeds field count limit", func(t *testing.T) {
		filter := map[string]any{
			"A": map[string]any{"Eq": "1"},
			"B": map[string]any{"Eq": "2"},
			"C": map[string]any{"Eq": "3"},
		}
		err := CheckComplexity(filter, &ComplexityLimits{MaxTotalFields: 2})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "field count")
	})

	t.Run("exceeds logical operator limit", func(t *testing.T) {
		filter := map[string]any{
			"And": []any{
				map[string]any{"Or": []any{
					map[string]any{"Name": map[string]any{"Eq": "A"}},
				}},
			},
		}
		err := CheckComplexity(filter, &ComplexityLimits{MaxLogicalOperators: 1})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logical operator count")
	})

	t.Run("exceeds logical depth limit", func(t *testing.T) {
		filter := map[string]any{
			"And": []any{
				map[string]any{
					"Or": []any{
						map[string]any{
							"And": []any{
								map[string]any{"Name": map[string]any{"Eq": "A"}},
							},
						},
					},
				},
			},
		}
		err := CheckComplexity(filter, &ComplexityLimits{MaxLogicalDepth: 2})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logical nesting depth")
	})

	t.Run("exceeds Or branches limit", func(t *testing.T) {
		filter := map[string]any{
			"Or": []any{
				map[string]any{"A": map[string]any{"Eq": "1"}},
				map[string]any{"B": map[string]any{"Eq": "2"}},
				map[string]any{"C": map[string]any{"Eq": "3"}},
				map[string]any{"D": map[string]any{"Eq": "4"}},
			},
		}
		err := CheckComplexity(filter, &ComplexityLimits{MaxOrBranches: 3})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Or branches")
	})

	t.Run("nil limits skips checking", func(t *testing.T) {
		filter := map[string]any{
			"A": map[string]any{
				"B": map[string]any{
					"C": map[string]any{
						"D": map[string]any{
							"E": map[string]any{
								"Name": map[string]any{"Eq": "test"},
							},
						},
					},
				},
			},
		}
		err := CheckComplexity(filter, nil)
		require.NoError(t, err)
	})

	t.Run("zero limit means unlimited for that metric", func(t *testing.T) {
		filter := map[string]any{
			"A": map[string]any{
				"B": map[string]any{
					"C": map[string]any{
						"D": map[string]any{
							"Name": map[string]any{"Eq": "test"},
						},
					},
				},
			},
		}
		err := CheckComplexity(filter, &ComplexityLimits{MaxDepth: 0}) // 0 means no limit for depth
		require.NoError(t, err)
	})
}

func TestPredefinedLimits(t *testing.T) {
	t.Run("StrictLimits are stricter than DefaultLimits", func(t *testing.T) {
		assert.Less(t, StrictLimits.MaxDepth, DefaultLimits.MaxDepth)
		assert.Less(t, StrictLimits.MaxTotalFields, DefaultLimits.MaxTotalFields)
		assert.Less(t, StrictLimits.MaxLogicalOperators, DefaultLimits.MaxLogicalOperators)
	})

	t.Run("RelaxedLimits are looser than DefaultLimits", func(t *testing.T) {
		assert.Greater(t, RelaxedLimits.MaxDepth, DefaultLimits.MaxDepth)
		assert.Greater(t, RelaxedLimits.MaxTotalFields, DefaultLimits.MaxTotalFields)
		assert.Greater(t, RelaxedLimits.MaxLogicalOperators, DefaultLimits.MaxLogicalOperators)
	})
}
