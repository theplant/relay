package filter

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
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
	for k, v := range m {
		if v == nil {
			delete(m, k)
			continue
		}

		if nestedMap, ok := v.(map[string]any); ok {
			PruneMap(nestedMap)
			if len(nestedMap) == 0 {
				delete(m, k)
			}
			continue
		}

		if slice, ok := v.([]any); ok {
			if len(slice) == 0 {
				delete(m, k)
			}
		}
	}
}
