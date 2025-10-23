package filter

import (
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/theplant/relay"
	"github.com/theplant/relay/internal/hook"
)

// HandleOperatorInput provides input information for operator handling
type HandleOperatorInput struct {
	FilterName    string         // Name of the filter (e.g., "Status", "CreatedAt")
	FilterType    reflect.Type   // Type of the filter
	FilterMap     map[string]any // The filter map being built (can be modified directly)
	OperatorName  string         // Name of the operator (e.g., "Eq", "Gte")
	OperatorValue reflect.Value  // Value of the operator
	OperatorType  reflect.Type   // Type of the operator value
}

// HandleOperatorOutput represents the result of operator handling
type HandleOperatorOutput struct{}

// HandleOperatorFunc is a function that handles operator field transformation.
// The handler should directly modify input.FilterMap.
// To pass control to the next handler in the chain, call next(input).
type HandleOperatorFunc func(input *HandleOperatorInput) (*HandleOperatorOutput, error)

// ParseProtoFilterOptions holds configuration options for ParseProtoFilter
type ParseProtoFilterOptions struct {
	handleOperatorHook func(next HandleOperatorFunc) HandleOperatorFunc
}

// ParseProtoFilterOption is a function that configures ParseProtoFilterOptions
type ParseProtoFilterOption func(*ParseProtoFilterOptions)

// WithHandleOperatorHook adds custom operator handler hooks.
// Hooks are applied in the order they are added.
// The default handler is always at the end of the chain.
func WithHandleOperatorHook(hooks ...func(next HandleOperatorFunc) HandleOperatorFunc) ParseProtoFilterOption {
	return func(o *ParseProtoFilterOptions) {
		o.handleOperatorHook = hook.Prepend(o.handleOperatorHook, hooks...)
	}
}

// ParseProtoFilter parses a proto filter message to a map
// It uses Go reflection to automatically handle type conversions:
// - Proto enums -> strings (with validation)
// - Timestamps -> time.Time
// - Recursively processes nested filters (And/Or/Not)
// Custom transformers can be provided via options to override default behavior.
func ParseProtoFilter[T proto.Message](protoFilter T, opts ...ParseProtoFilterOption) (map[string]any, error) {
	if lo.IsNil(protoFilter) {
		return nil, nil
	}

	options := &ParseProtoFilterOptions{}
	for _, opt := range opts {
		opt(options)
	}

	filterMap, err := ToMap(protoFilter)
	if err != nil {
		return nil, err
	}

	structValue := reflect.Indirect(reflect.ValueOf(protoFilter))
	structType := structValue.Type()
	if err := fixFilterMap(filterMap, structType, structValue, options); err != nil {
		return nil, err
	}

	return filterMap, nil
}

func fixFilterMap(m map[string]any, schemaType reflect.Type, schemaValue reflect.Value, options *ParseProtoFilterOptions) error {
	if m == nil {
		return nil
	}

	for fieldName, value := range m {
		if value == nil {
			continue
		}

		switch fieldName {
		case "And", "Or":
			if err := fixLogicalFilterList(m, fieldName, schemaType, schemaValue, options); err != nil {
				return err
			}
			continue
		case "Not":
			if err := fixLogicalFilterSingle(m, fieldName, schemaType, schemaValue, options); err != nil {
				return err
			}
			continue
		}

		if err := fixNestedFilterMap(m, fieldName, schemaType, schemaValue, options); err != nil {
			return err
		}
	}

	return nil
}

func fixLogicalFilterList(m map[string]any, key string, schemaType reflect.Type, schemaValue reflect.Value, options *ParseProtoFilterOptions) error {
	value := m[key]
	if value == nil {
		return nil
	}

	filterList, ok := value.([]any)
	if !ok {
		return errors.Errorf("logical filter %s exists in map but is not []any, got %T", key, value)
	}

	field, ok := schemaType.FieldByName(key)
	if !ok {
		return errors.Errorf("field %s not found in schema type %s", key, schemaType.Name())
	}

	fieldType := field.Type
	if fieldType.Kind() != reflect.Slice {
		return errors.Errorf("logical filter field %s should be a slice, got %s", key, fieldType.Kind())
	}

	elemType := indirectType(fieldType.Elem())
	if elemType != schemaType {
		return errors.Errorf("logical filter %s element type %s does not match parent filter type %s",
			key, elemType.Name(), schemaType.Name())
	}

	fieldValue := schemaValue.FieldByName(key)
	for i, item := range filterList {
		subMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		elemValue := fieldValue.Index(i)
		if err := fixFilterMap(subMap, schemaType, reflect.Indirect(elemValue), options); err != nil {
			return err
		}
	}
	return nil
}

func fixLogicalFilterSingle(m map[string]any, key string, schemaType reflect.Type, schemaValue reflect.Value, options *ParseProtoFilterOptions) error {
	value := m[key]
	if value == nil {
		return nil
	}

	subMap, ok := value.(map[string]any)
	if !ok {
		return errors.Errorf("logical filter %s exists in map but is not map[string]any, got %T", key, value)
	}

	field, ok := schemaType.FieldByName(key)
	if !ok {
		return errors.Errorf("field %s not found in schema type %s", key, schemaType.Name())
	}

	fieldType := indirectType(field.Type)
	if fieldType != schemaType {
		return errors.Errorf("logical filter %s type %s does not match parent filter type %s",
			key, fieldType.Name(), schemaType.Name())
	}

	fieldValue := schemaValue.FieldByName(key)
	return fixFilterMap(subMap, schemaType, reflect.Indirect(fieldValue), options)
}

func fixNestedFilterMap(m map[string]any, key string, parentType reflect.Type, parentValue reflect.Value, options *ParseProtoFilterOptions) error {
	value := m[key]
	if value == nil {
		return nil
	}

	filterMap, ok := value.(map[string]any)
	if !ok {
		return errors.Errorf("field %s: expected nested filter map, got %T", key, value)
	}

	filterField, ok := parentType.FieldByName(key)
	if !ok {
		return nil
	}

	filterType := indirectType(filterField.Type)
	filterValue := reflect.Indirect(parentValue.FieldByName(key))

	if isRelationshipFilter(parentType, filterType) {
		if err := fixFilterMap(filterMap, filterType, filterValue, options); err != nil {
			return errors.Wrapf(err, "failed to fix relationship filter field %s", key)
		}
		return nil
	}

	// Collect operator names first to avoid modifying map while iterating
	operatorNames := lo.Keys(filterMap)
	for _, operatorName := range operatorNames {
		value := filterMap[operatorName]
		if value == nil {
			continue
		}

		operatorField, ok := filterType.FieldByName(operatorName)
		if !ok {
			continue
		}

		operatorValue := filterValue.FieldByName(operatorName)

		if !operatorValue.IsValid() || (operatorValue.Kind() == reflect.Ptr && operatorValue.IsNil()) {
			continue
		}

		handleOperator := defaultHandleOperator
		if options != nil && options.handleOperatorHook != nil {
			handleOperator = options.handleOperatorHook(handleOperator)
		}

		_, err := handleOperator(&HandleOperatorInput{
			FilterName:    key,
			FilterType:    filterField.Type, // original field type, not indirect type
			FilterMap:     filterMap,
			OperatorName:  operatorName,
			OperatorValue: operatorValue,
			OperatorType:  operatorField.Type, // original field type, not indirect type
		})
		if err != nil {
			return errors.Wrapf(err, "operator %s", operatorName)
		}
	}

	return nil
}

// defaultHandleOperator is the default operator handler that handles standard type conversions.
// It converts:
// - Timestamps to time.Time
// - Enums to strings
// - Enum slices to string slices
func defaultHandleOperator(input *HandleOperatorInput) (*HandleOperatorOutput, error) {
	// Handle timestamp
	if input.OperatorType == reflect.TypeOf((*timestamppb.Timestamp)(nil)) {
		ts, ok := input.OperatorValue.Interface().(*timestamppb.Timestamp)
		if !ok {
			return nil, errors.Errorf("expected timestamp value, got %T", input.OperatorValue.Interface())
		}
		input.FilterMap[input.OperatorName] = ts.AsTime()
		return &HandleOperatorOutput{}, nil
	}

	// Handle enum
	if isEnumType(input.OperatorType) {
		enumValue := reflect.Indirect(input.OperatorValue).Interface()
		protoEnum, ok := enumValue.(protoreflect.Enum)
		if !ok {
			return nil, errors.Errorf("expected enum value, got %T", enumValue)
		}
		converted, err := relay.ParseProtoEnum(protoEnum)
		if err != nil {
			return nil, err
		}
		input.FilterMap[input.OperatorName] = converted
		return &HandleOperatorOutput{}, nil
	}

	// Handle enum slice
	if input.OperatorType.Kind() == reflect.Slice {
		elemType := input.OperatorType.Elem()
		if isEnumType(elemType) {
			if input.OperatorValue.Len() == 0 {
				return &HandleOperatorOutput{}, nil
			}
			converted, err := convertProtoEnumSlice(input.OperatorValue)
			if err != nil {
				return nil, err
			}
			input.FilterMap[input.OperatorName] = converted
			return &HandleOperatorOutput{}, nil
		}
	}

	return &HandleOperatorOutput{}, nil
}

func convertProtoEnumSlice(sliceValue reflect.Value) ([]any, error) {
	result := make([]any, 0, sliceValue.Len())
	for i := 0; i < sliceValue.Len(); i++ {
		elemValue := sliceValue.Index(i).Interface()
		protoEnum, ok := elemValue.(protoreflect.Enum)
		if !ok {
			return nil, errors.Errorf("index %d: expected enum value, got %T", i, elemValue)
		}
		converted, err := relay.ParseProtoEnum(protoEnum)
		if err != nil {
			return nil, errors.Wrapf(err, "index %d", i)
		}
		result = append(result, converted)
	}
	return result, nil
}

func indirectType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}

// isRelationshipFilter checks if a filter field is a relationship filter (external)
// rather than a field-level filter (internal nested message).
//
// Relationship filters: CategoryFilter - independent types with And/Or/Not logic
// Field-level filters: ProductFilter_StatusFilter - nested messages with only operators
func isRelationshipFilter(parentType, fieldType reflect.Type) bool {
	msgType := reflect.TypeOf((*proto.Message)(nil)).Elem()
	if !fieldType.Implements(msgType) {
		return false
	}

	parentName := parentType.Name()
	fieldName := fieldType.Name()
	prefix := parentName + "_"
	return !strings.HasPrefix(fieldName, prefix)
}

func isEnumType(t reflect.Type) bool {
	enumType := reflect.TypeOf((*protoreflect.Enum)(nil)).Elem()
	return t.Implements(enumType)
}
