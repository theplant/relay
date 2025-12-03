package protofilter

import (
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/sunfmin/reflectutils"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/theplant/relay/filter"
	"github.com/theplant/relay/protorelay"
)

type ToMapOption func(*toMapOptions)

type toMapOptions struct {
	transformHook    func(filter.TransformFunc) filter.TransformFunc
	complexityLimits *filter.ComplexityLimits
}

// WithTransformHook allows customizing the transform function
func WithTransformHook(hook func(filter.TransformFunc) filter.TransformFunc) ToMapOption {
	return func(opts *toMapOptions) {
		opts.transformHook = hook
	}
}

// WithComplexityLimits sets custom complexity limits for the filter.
// By default, filter.DefaultLimits is used.
// Pass nil to disable complexity checking.
func WithComplexityLimits(limits *filter.ComplexityLimits) ToMapOption {
	return func(opts *toMapOptions) {
		opts.complexityLimits = limits
	}
}

// ToMap converts a proto filter message to a map with proper transformations.
// It applies default transformations (enum conversion, capitalization) and any custom transforms.
// By default, complexity is checked against filter.DefaultLimits.
func ToMap[T proto.Message](protoFilter T, opts ...ToMapOption) (map[string]any, error) {
	if lo.IsNil(protoFilter) {
		return nil, nil
	}

	options := &toMapOptions{
		complexityLimits: filter.DefaultLimits,
	}
	for _, opt := range opts {
		opt(options)
	}

	// Stage 1: Convert proto message to camelCase map (no transformations)
	camelCaseMap, err := toRawMap(protoFilter)
	if err != nil {
		return nil, err
	}

	// Stage 2: Check complexity (early rejection of overly complex filters)
	if err := filter.CheckComplexity(camelCaseMap, options.complexityLimits); err != nil {
		return nil, err
	}

	// Stage 3: Capitalize keys (camelCase -> PascalCase)
	pascalCaseMap, err := filter.Transform(camelCaseMap, capitalizeTransform)
	if err != nil {
		return nil, err
	}

	// Stage 4: Apply transformations (enum conversion, custom hooks)
	transform := buildDefaultTransform(protoFilter)
	if options.transformHook != nil {
		transform = options.transformHook(transform)
	}

	return filter.Transform(pascalCaseMap, transform)
}

// toRawMap converts a proto message to a camelCase map without any transformations
func toRawMap[T proto.Message](protoFilter T) (map[string]any, error) {
	data, err := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}.Marshal(protoFilter)
	if err != nil {
		return nil, errors.Wrap(err, "marshal proto to json")
	}

	var camelCaseMap map[string]any
	if err := json.Unmarshal(data, &camelCaseMap); err != nil {
		return nil, errors.Wrap(err, "unmarshal json to map")
	}

	filter.PruneMap(camelCaseMap)

	return camelCaseMap, nil
}

// capitalizeTransform converts camelCase keys to PascalCase keys.
func capitalizeTransform(input *filter.TransformInput) (*filter.TransformOutput, error) {
	return &filter.TransformOutput{Key: filter.Capitalize(input.KeyPath.Last()), Value: input.Value}, nil
}

// buildDefaultTransform creates a default transform function that handles enum conversion.
// It captures the proto model via closure for type information queries.
// NOTE: The KeyPath uses PascalCase keys (after capitalizeTransform stage).
func buildDefaultTransform(model proto.Message) filter.TransformFunc {
	return func(input *filter.TransformInput) (*filter.TransformOutput, error) {
		outputKey := input.KeyPath.Last()
		outputValue := input.Value

		if model != nil {
			keyPath := input.KeyPath.String()
			fieldType := reflectutils.GetType(model, keyPath)
			if fieldType != nil {
				if isEnumType(fieldType) {
					fieldValue, err := reflectutils.Get(model, keyPath)
					if err != nil {
						return nil, errors.Wrapf(err, "get field value for %s", keyPath)
					}
					if !lo.IsNil(fieldValue) {
						if protoEnum, ok := fieldValue.(protoreflect.Enum); ok {
							converted, err := protorelay.ParseEnum(protoEnum)
							if err != nil {
								return nil, err
							}
							outputValue = converted
						}
					}
				}

				if fieldType.Kind() == reflect.Slice {
					elemType := fieldType.Elem()
					if isEnumType(elemType) {
						fieldValue, err := reflectutils.Get(model, keyPath)
						if err != nil {
							return nil, errors.Wrapf(err, "get field value for %s", keyPath)
						}
						if !lo.IsNil(fieldValue) {
							sliceVal := reflect.ValueOf(fieldValue)
							if sliceVal.IsValid() && sliceVal.Len() > 0 {
								converted, err := convertProtoEnumSlice(sliceVal)
								if err != nil {
									return nil, err
								}
								outputValue = converted
							}
						}
					}
				}
			}
		}

		return &filter.TransformOutput{Key: outputKey, Value: outputValue}, nil
	}
}

func convertProtoEnumSlice(sliceValue reflect.Value) ([]any, error) {
	result := make([]any, 0, sliceValue.Len())
	for i := 0; i < sliceValue.Len(); i++ {
		elemValue := sliceValue.Index(i).Interface()
		protoEnum, ok := elemValue.(protoreflect.Enum)
		if !ok {
			return nil, errors.Errorf("index %d: expected enum value, got %T", i, elemValue)
		}
		converted, err := protorelay.ParseEnum(protoEnum)
		if err != nil {
			return nil, errors.Wrapf(err, "index %d", i)
		}
		result = append(result, converted)
	}
	return result, nil
}

func isEnumType(t reflect.Type) bool {
	if t == nil {
		return false
	}
	enumType := reflect.TypeOf((*protoreflect.Enum)(nil)).Elem()
	return t.Implements(enumType)
}
