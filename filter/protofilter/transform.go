package protofilter

import (
	"reflect"
	"strings"
	"sync"

	"github.com/samber/lo"

	"github.com/theplant/relay/filter"
)

// TransformKeyInput provides input information for key transformation
type TransformKeyInput struct {
	Key     string         // The original camelCase key from protojson
	KeyType filter.KeyType // The type of this key
}

// TransformKeyOutput represents the result of key transformation
type TransformKeyOutput struct {
	Key string // The transformed key
}

// TransformKeyFunc is a function that transforms map keys
type TransformKeyFunc func(*TransformKeyInput) (*TransformKeyOutput, error)

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(input *TransformKeyInput) (*TransformKeyOutput, error) {
	s := input.Key
	if s == "" {
		return &TransformKeyOutput{Key: s}, nil
	}
	return &TransformKeyOutput{Key: strings.ToUpper(s[:1]) + s[1:]}, nil
}

type fieldMapping struct {
	exactMatch map[string]struct{} // Set of field names that exist in model (for exact match check)
	snakeMatch map[string]string   // key: snake_case -> model field name
}

// AlignWith creates a transform hook that aligns transformed keys with model field names.
// It first applies the default transformation, then checks if the result matches model fields.
// Uses a two-step matching strategy:
//  1. Exact match: CategoryId -> CategoryId (if exists in model)
//  2. Snake_case match: CategoryId -> category_id -> CategoryID
//
// Only field keys are aligned; logical operators, operators, and modifiers use default capitalization.
//
// Example:
//
//	protofilter.ToMap(filter, protofilter.WithTransformKeyHook(
//	    protofilter.AlignWith(Product{}),
//	))
func AlignWith(model any) func(next TransformKeyFunc) TransformKeyFunc {
	modelType := reflect.TypeOf(model)
	if modelType == nil {
		panic("model cannot be nil")
	}

	// Get or build field mapping for this model type
	mapping := getOrBuildFieldMapping(modelType)

	return func(next TransformKeyFunc) TransformKeyFunc {
		return func(input *TransformKeyInput) (*TransformKeyOutput, error) {
			// First, apply default transformation
			output, err := next(input)
			if err != nil {
				return nil, err
			}

			// Only align field keys; other key types use default result
			if input.KeyType != filter.KeyTypeField {
				return output, nil
			}

			// Step 1: Try exact match with transformed key
			if _, exists := mapping.exactMatch[output.Key]; exists {
				return output, nil // Use the default transformed key as-is
			}

			// Step 2: Try snake_case match
			snakeKey := lo.SnakeCase(output.Key)
			if modelFieldName, ok := mapping.snakeMatch[snakeKey]; ok {
				return &TransformKeyOutput{Key: modelFieldName}, nil
			}

			// Not found in model, use default transformation result
			return output, nil
		}
	}
}

var fieldMappingCache sync.Map // map[reflect.Type]*fieldMapping

// getOrBuildFieldMapping gets or builds the field mapping for a model type
func getOrBuildFieldMapping(modelType reflect.Type) *fieldMapping {
	if cached, ok := fieldMappingCache.Load(modelType); ok {
		return cached.(*fieldMapping)
	}

	mapping := buildFieldMapping(modelType)
	fieldMappingCache.Store(modelType, mapping)
	return mapping
}

// buildFieldMapping builds field name mappings for a struct type
func buildFieldMapping(modelType reflect.Type) *fieldMapping {
	mapping := &fieldMapping{
		exactMatch: make(map[string]struct{}),
		snakeMatch: make(map[string]string),
	}

	t := indirectType(modelType)
	if t.Kind() != reflect.Struct {
		return mapping
	}

	collectFields(t, mapping)
	return mapping
}

func collectFields(t reflect.Type, mapping *fieldMapping) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs by recursively collecting their fields
		if field.Anonymous {
			embeddedType := indirectType(field.Type)
			if embeddedType.Kind() == reflect.Struct {
				collectFields(embeddedType, mapping)
			}
			continue
		}

		// Exact match: mark field name as existing
		mapping.exactMatch[field.Name] = struct{}{}

		// Snake_case match: detect conflicts
		snakeKey := lo.SnakeCase(field.Name)
		if existingField, exists := mapping.snakeMatch[snakeKey]; exists {
			panic(strings.Join([]string{
				"AlignWith: model has conflicting snake_case field names:",
				"field '" + existingField + "' and '" + field.Name + "'",
				"both convert to '" + snakeKey + "'.",
				"This model is not suitable for AlignWith.",
			}, " "))
		}
		mapping.snakeMatch[snakeKey] = field.Name
		// Example: CategoryID -> category_id -> CategoryID
	}
}
