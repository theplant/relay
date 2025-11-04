package protofilter

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/theplant/relay/filter"
)

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

	// 3. Transform keys (top level keys are field keys)
	result, err := transformMapKeys(camelCaseMap, transformKey, true)
	if err != nil {
		return nil, err
	}

	// 4. Clean up nil values, empty slices, and empty maps
	filter.PruneMap(result)

	return result, nil
}

// transformMapKeys recursively transforms all map keys using the provided function
func transformMapKeys(m map[string]any, transform TransformKeyFunc, isFieldKey bool) (map[string]any, error) {
	if m == nil {
		return nil, nil
	}

	result := make(map[string]any, len(m))

	for key, value := range m {
		output, err := transform(&TransformKeyInput{
			Key:        key,
			IsFieldKey: isFieldKey,
		})
		if err != nil {
			return nil, err
		}

		// Determine if nested keys are field keys
		// - If current key is and/or/not (proto camelCase), nested keys are also field keys
		// - Otherwise, nested keys are operator keys
		nextIsFieldKey := (key == "and" || key == "or" || key == "not")

		transformedValue, err := transformValue(value, transform, nextIsFieldKey)
		if err != nil {
			return nil, err
		}
		result[output.Key] = transformedValue
	}

	return result, nil
}

// transformValue recursively transforms map keys in any value
func transformValue(v any, transform TransformKeyFunc, isFieldKey bool) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		return transformMapKeys(val, transform, isFieldKey)
	case []any:
		result := make([]any, 0, len(val))
		for _, item := range val {
			transformedItem, err := transformValue(item, transform, isFieldKey)
			if err != nil {
				return nil, err
			}
			result = append(result, transformedItem)
		}
		return result, nil
	default:
		return v, nil
	}
}
