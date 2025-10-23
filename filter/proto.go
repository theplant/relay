package filter

import (
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/theplant/relay"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ParseProtoFilter parses a proto filter message to a map
// It uses Go reflection to automatically handle type conversions:
// - Proto enums -> strings (with validation)
// - Timestamps -> time.Time
// - Recursively processes nested filters (And/Or/Not)
func ParseProtoFilter[T proto.Message](protoFilter T) (map[string]any, error) {
	if lo.IsNil(protoFilter) {
		return nil, nil
	}

	filterMap, err := ToMap(protoFilter)
	if err != nil {
		return nil, err
	}

	structValue := reflect.Indirect(reflect.ValueOf(protoFilter))
	structType := structValue.Type()
	if err := fixFilterMap(filterMap, structType, structValue); err != nil {
		return nil, err
	}

	return filterMap, nil
}

func fixFilterMap(m map[string]any, schemaType reflect.Type, schemaValue reflect.Value) error {
	if m == nil {
		return nil
	}

	for fieldName, value := range m {
		if value == nil {
			continue
		}

		switch fieldName {
		case "And", "Or":
			if err := fixLogicalFilterList(m, fieldName, schemaType, schemaValue); err != nil {
				return err
			}
			continue
		case "Not":
			if err := fixLogicalFilterSingle(m, fieldName, schemaType, schemaValue); err != nil {
				return err
			}
			continue
		}

		if err := fixNestedFilterMap(m, fieldName, schemaType, schemaValue); err != nil {
			return err
		}
	}

	return nil
}

func fixLogicalFilterList(m map[string]any, key string, schemaType reflect.Type, schemaValue reflect.Value) error {
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
		if err := fixFilterMap(subMap, schemaType, reflect.Indirect(elemValue)); err != nil {
			return err
		}
	}
	return nil
}

func fixLogicalFilterSingle(m map[string]any, key string, schemaType reflect.Type, schemaValue reflect.Value) error {
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
	return fixFilterMap(subMap, schemaType, reflect.Indirect(fieldValue))
}

func fixNestedFilterMap(m map[string]any, key string, parentType reflect.Type, parentValue reflect.Value) error {
	value := m[key]
	if value == nil {
		return nil
	}

	nestedMap, ok := value.(map[string]any)
	if !ok {
		return errors.Errorf("field %s: expected nested filter map, got %T", key, value)
	}

	field, ok := parentType.FieldByName(key)
	if !ok {
		return nil
	}

	nestedType := indirectType(field.Type)
	nestedValue := reflect.Indirect(parentValue.FieldByName(key))

	if isRelationshipFilter(parentType, nestedType) {
		if err := fixFilterMap(nestedMap, nestedType, nestedValue); err != nil {
			return errors.Wrapf(err, "failed to fix relationship filter field %s", key)
		}
		return nil
	}

	for fieldName, value := range nestedMap {
		if value == nil {
			continue
		}

		field, ok := nestedType.FieldByName(fieldName)
		if !ok {
			continue
		}

		fieldType := indirectType(field.Type)
		fieldValue := nestedValue.FieldByName(fieldName)

		if fieldType == reflect.TypeOf(timestamppb.Timestamp{}) {
			if !fieldValue.IsValid() || (fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil()) {
				continue
			}
			ts, ok := fieldValue.Interface().(*timestamppb.Timestamp)
			if !ok {
				return errors.Errorf("field %s: expected timestamp value, got %T", fieldName, fieldValue.Interface())
			}
			nestedMap[fieldName] = ts.AsTime()
			continue
		}

		if isEnumType(fieldType) {
			if !fieldValue.IsValid() || (fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil()) {
				continue
			}
			// Indirect to get the actual enum value if it's a pointer
			enumValue := reflect.Indirect(fieldValue).Interface()
			protoEnum, ok := enumValue.(protoreflect.Enum)
			if !ok {
				return errors.Errorf("field %s: expected enum value, got %T", fieldName, enumValue)
			}
			converted, err := relay.ParseProtoEnum(protoEnum)
			if err != nil {
				return errors.Wrapf(err, "field %s", fieldName)
			}
			nestedMap[fieldName] = converted
			continue
		}

		if fieldType.Kind() == reflect.Slice {
			elemType := fieldType.Elem()
			if isEnumType(elemType) {
				if !fieldValue.IsValid() || fieldValue.Len() == 0 {
					continue
				}
				converted, err := convertProtoEnumSlice(fieldValue)
				if err != nil {
					return errors.Wrapf(err, "field %s", fieldName)
				}
				nestedMap[fieldName] = converted
				continue
			}
		}
	}

	return nil
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
