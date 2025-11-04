package protofilter

import (
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/theplant/relay/filter"
)

// TransformKeyFunc is a function that transforms map keys
type TransformKeyFunc func(string) string

// toMap parses a proto filter message to a map and transforms keys using the provided function.
// This approach ensures all proto special types (well-known types, oneof, etc.) are handled correctly.
func toMap[T proto.Message](protoFilter T, transformKey TransformKeyFunc) (map[string]any, error) {
	if lo.IsNil(protoFilter) {
		return nil, nil
	}

	// 1. Use protojson to serialize (handles all proto special types)
	// Use EmitUnpopulated to include fields with default values
	data, err := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}.Marshal(protoFilter)
	if err != nil {
		return nil, errors.Wrap(err, "marshal proto to json")
	}

	// 2. Unmarshal to map (keys are camelCase from protojson)
	var camelCaseMap map[string]any
	if err := json.Unmarshal(data, &camelCaseMap); err != nil {
		return nil, errors.Wrap(err, "unmarshal json to map")
	}

	// 3. Transform keys
	result := transformMapKeys(camelCaseMap, transformKey)

	// 4. Clean up nil values, empty slices, and empty maps
	filter.PruneMap(result)

	return result, nil
}

// transformMapKeys recursively transforms all map keys using the provided function
func transformMapKeys(m map[string]any, transform TransformKeyFunc) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))

	for key, value := range m {
		transformedKey := transform(key)
		result[transformedKey] = transformValue(value, transform)
	}

	return result
}

// transformValue recursively transforms map keys in any value
func transformValue(v any, transform TransformKeyFunc) any {
	switch val := v.(type) {
	case map[string]any:
		return transformMapKeys(val, transform)
	case []any:
		result := make([]any, 0, len(val))
		for _, item := range val {
			result = append(result, transformValue(item, transform))
		}
		return result
	default:
		return v
	}
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
