package filter

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
)

// KeyType represents the type of a filter key
type KeyType string

const (
	// KeyTypeField represents a model field key (Name, Code, CategoryID, etc.)
	// These keys should be aligned with model fields
	KeyTypeField KeyType = "FIELD"

	// KeyTypeLogical represents a logical operator key (And, Or, Not)
	// These keys are not model fields but contain nested field keys
	KeyTypeLogical KeyType = "LOGICAL"

	// KeyTypeOperator represents a filter operator key (Eq, Contains, In, Gt, etc.)
	// These keys define how to filter a field
	KeyTypeOperator KeyType = "OPERATOR"

	// KeyTypeModifier represents a modifier key that changes operator behavior (Fold, etc.)
	// These keys modify how operators work (e.g., case-insensitive comparison)
	KeyTypeModifier KeyType = "MODIFIER"
)

// TagKey is the tag key used to marshal and unmarshal filter to and from map[string]any.
const TagKey = "~~~filter~~~"

// use strcut field name as key and force emit empty
var jsoniterForFilter = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	TagKey:                 TagKey,
}.Froze()

// ToMap converts a filter to a map[string]any.
func ToMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	data, err := jsoniterForFilter.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "marshal filter")
	}
	var filterMap map[string]any
	if err := jsoniterForFilter.Unmarshal(data, &filterMap); err != nil {
		return nil, errors.Wrap(err, "unmarshal filter to map")
	}
	PruneMap(filterMap)
	return filterMap, nil
}

// PruneMap recursively removes nil values, empty slices, and empty nested maps.
func PruneMap(m map[string]any) {
	prune(m)
}

// prune recursively processes a value and returns the pruned result.
// Returns nil if the value should be removed (nil, empty map, or empty slice).
func prune(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			pruned := prune(item)
			if pruned == nil {
				delete(val, k)
			} else {
				val[k] = pruned
			}
		}
		if len(val) == 0 {
			return nil
		}
		return val

	case []any:
		if len(val) == 0 {
			return nil
		}
		pruned := make([]any, 0, len(val))
		for _, item := range val {
			prunedItem := prune(item)
			if prunedItem != nil {
				pruned = append(pruned, prunedItem)
			}
		}
		if len(pruned) == 0 {
			return nil
		}
		return pruned

	default:
		return v
	}
}
