package filter

import (
	"strings"

	"github.com/pkg/errors"
)

// ComplexityLimits defines limits for filter complexity.
// A value of 0 means no limit for that metric.
type ComplexityLimits struct {
	MaxDepth            int // Maximum nesting depth of relationship filters
	MaxTotalFields      int // Maximum total number of field filters
	MaxLogicalOperators int // Maximum number of logical operators (And/Or/Not)
	MaxLogicalDepth     int // Maximum nesting depth of logical operators
	MaxOrBranches       int // Maximum branches in a single Or operator
}

// ComplexityResult contains the calculated complexity metrics of a filter.
type ComplexityResult struct {
	Depth            int // Deepest nesting level reached
	TotalFields      int // Total number of field filters
	LogicalOperators int // Total number of logical operators
	LogicalDepth     int // Deepest logical operator nesting
	OrBranches       int // Maximum branches found in any Or operator
}

// Predefined complexity limits
var (
	// DefaultLimits provides reasonable defaults for most use cases.
	DefaultLimits = &ComplexityLimits{
		MaxDepth:            3,
		MaxTotalFields:      10,
		MaxLogicalOperators: 5,
		MaxLogicalDepth:     2,
		MaxOrBranches:       3,
	}

	// StrictLimits provides tighter limits for security-sensitive contexts.
	StrictLimits = &ComplexityLimits{
		MaxDepth:            2,
		MaxTotalFields:      5,
		MaxLogicalOperators: 3,
		MaxLogicalDepth:     1,
		MaxOrBranches:       2,
	}

	// RelaxedLimits provides looser limits for trusted/internal use.
	RelaxedLimits = &ComplexityLimits{
		MaxDepth:            5,
		MaxTotalFields:      20,
		MaxLogicalOperators: 10,
		MaxLogicalDepth:     3,
		MaxOrBranches:       5,
	}
)

// CheckComplexity validates that a filter map doesn't exceed the specified limits.
// Returns an error describing which limit was exceeded, or nil if within limits.
// If limits is nil, no validation is performed.
func CheckComplexity(filterMap map[string]any, limits *ComplexityLimits) error {
	if limits == nil {
		return nil
	}

	result := CalculateComplexity(filterMap)

	if limits.MaxDepth > 0 && result.Depth > limits.MaxDepth {
		return errors.Errorf("filter depth %d exceeds limit %d", result.Depth, limits.MaxDepth)
	}
	if limits.MaxTotalFields > 0 && result.TotalFields > limits.MaxTotalFields {
		return errors.Errorf("filter field count %d exceeds limit %d", result.TotalFields, limits.MaxTotalFields)
	}
	if limits.MaxLogicalOperators > 0 && result.LogicalOperators > limits.MaxLogicalOperators {
		return errors.Errorf("filter logical operator count %d exceeds limit %d", result.LogicalOperators, limits.MaxLogicalOperators)
	}
	if limits.MaxLogicalDepth > 0 && result.LogicalDepth > limits.MaxLogicalDepth {
		return errors.Errorf("filter logical nesting depth %d exceeds limit %d", result.LogicalDepth, limits.MaxLogicalDepth)
	}
	if limits.MaxOrBranches > 0 && result.OrBranches > limits.MaxOrBranches {
		return errors.Errorf("filter Or branches %d exceeds limit %d", result.OrBranches, limits.MaxOrBranches)
	}

	return nil
}

// CalculateComplexity analyzes a filter map and returns its complexity metrics.
func CalculateComplexity(filterMap map[string]any) *ComplexityResult {
	result := &ComplexityResult{}
	calculateComplexityRecursive(filterMap, 1, 0, result)
	return result
}

func calculateComplexityRecursive(m map[string]any, depth int, logicalDepth int, result *ComplexityResult) {
	if depth > result.Depth {
		result.Depth = depth
	}
	if logicalDepth > result.LogicalDepth {
		result.LogicalDepth = logicalDepth
	}

	for key, value := range m {
		if value == nil {
			continue
		}

		lowerKey := strings.ToLower(key)

		switch lowerKey {
		case "and", "or", "not":
			result.LogicalOperators++
			handleLogicalComplexity(lowerKey, value, depth, logicalDepth+1, result)
		default:
			valueMap, ok := value.(map[string]any)
			if !ok {
				continue
			}

			if isRelationshipFilterMap(valueMap) {
				// Relationship filter - recurse with increased depth
				calculateComplexityRecursive(valueMap, depth+1, logicalDepth, result)
			} else {
				// Field filter - count it
				result.TotalFields++
			}
		}
	}
}

func handleLogicalComplexity(op string, value any, depth int, logicalDepth int, result *ComplexityResult) {
	if logicalDepth > result.LogicalDepth {
		result.LogicalDepth = logicalDepth
	}

	switch op {
	case "and", "or":
		list, ok := value.([]any)
		if !ok {
			return
		}

		if op == "or" && len(list) > result.OrBranches {
			result.OrBranches = len(list)
		}

		for _, item := range list {
			subMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			calculateComplexityRecursive(subMap, depth, logicalDepth, result)
		}

	case "not":
		subMap, ok := value.(map[string]any)
		if !ok {
			return
		}
		calculateComplexityRecursive(subMap, depth, logicalDepth, result)
	}
}
