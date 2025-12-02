package protofilter

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theplant/relay/filter"
	testdatav1 "github.com/theplant/relay/protorelay/testdata/gen/testdata/v1"
)

func TestKeyTypeParameter(t *testing.T) {
	type keyInfo struct {
		key     string
		keyType filter.KeyType
	}
	var processedKeys []keyInfo

	recordingTransform := func(input *filter.TransformInput) (*filter.TransformOutput, error) {
		var lastKey string
		if len(input.KeyPath) > 0 {
			lastKey = input.KeyPath[len(input.KeyPath)-1]
		}

		var keyType filter.KeyType
		lowerKey := strings.ToLower(lastKey)

		switch lowerKey {
		case "and", "or", "not":
			keyType = filter.KeyTypeLogical
		case "fold":
			keyType = filter.KeyTypeModifier
		default:
			nonLogicalDepth := 0
			for i := 0; i < len(input.KeyPath)-1; i++ {
				part := input.KeyPath[i]

				if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
					continue
				}

				baseKey := part
				if idx := strings.Index(part, "["); idx >= 0 {
					baseKey = part[:idx]
				}
				lowerBaseKey := strings.ToLower(baseKey)
				if lowerBaseKey != "and" && lowerBaseKey != "or" && lowerBaseKey != "not" {
					nonLogicalDepth++
				}
			}

			if nonLogicalDepth == 0 {
				keyType = filter.KeyTypeField
			} else {
				keyType = filter.KeyTypeOperator
			}
		}

		processedKeys = append(processedKeys, keyInfo{
			key:     filter.Capitalize(lastKey),
			keyType: keyType,
		})

		return &filter.TransformOutput{Key: filter.Capitalize(lastKey), Value: input.Value}, nil
	}

	t.Run("simple field with operators", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			Name: &testdatav1.ProductFilter_NameFilter{
				Eq:       lo.ToPtr("Product A"),
				Contains: lo.ToPtr("A"),
			},
		}

		camelCaseMap, err := toRawMap(protoFilter)
		require.NoError(t, err)
		_, err = filter.Transform(camelCaseMap, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Eq", keyType: filter.KeyTypeOperator},
			{key: "Contains", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
		}
		assert.ElementsMatch(t, expected, processedKeys)
	})

	t.Run("logical operators with nested fields", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			And: []*testdatav1.ProductFilter{
				{
					Name: &testdatav1.ProductFilter_NameFilter{
						Eq: lo.ToPtr("Product A"),
					},
				},
				{
					Code: &testdatav1.ProductFilter_CodeFilter{
						In: []string{"P001", "P002"},
					},
				},
			},
		}

		camelCaseMap, err := toRawMap(protoFilter)
		require.NoError(t, err)
		_, err = filter.Transform(camelCaseMap, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "And", keyType: filter.KeyTypeLogical},
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Eq", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
			{key: "Code", keyType: filter.KeyTypeField},
			{key: "In", keyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, processedKeys)
	})

	t.Run("or operator with nested fields", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			Or: []*testdatav1.ProductFilter{
				{
					Name: &testdatav1.ProductFilter_NameFilter{
						Contains: lo.ToPtr("A"),
					},
				},
				{
					Code: &testdatav1.ProductFilter_CodeFilter{
						Eq: lo.ToPtr("P001"),
					},
				},
			},
		}

		camelCaseMap, err := toRawMap(protoFilter)
		require.NoError(t, err)
		_, err = filter.Transform(camelCaseMap, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "Or", keyType: filter.KeyTypeLogical},
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Contains", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
			{key: "Code", keyType: filter.KeyTypeField},
			{key: "Eq", keyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, processedKeys)
	})

	t.Run("not operator with nested field", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			Not: &testdatav1.ProductFilter{
				Name: &testdatav1.ProductFilter_NameFilter{
					Eq: lo.ToPtr("Product A"),
				},
			},
		}

		camelCaseMap, err := toRawMap(protoFilter)
		require.NoError(t, err)
		_, err = filter.Transform(camelCaseMap, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "Not", keyType: filter.KeyTypeLogical},
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Eq", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
		}
		assert.ElementsMatch(t, expected, processedKeys)
	})

	t.Run("complex nested structure", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			And: []*testdatav1.ProductFilter{
				{
					Or: []*testdatav1.ProductFilter{
						{
							Name: &testdatav1.ProductFilter_NameFilter{
								Contains: lo.ToPtr("A"),
							},
						},
						{
							Name: &testdatav1.ProductFilter_NameFilter{
								Contains: lo.ToPtr("B"),
							},
						},
					},
				},
				{
					Code: &testdatav1.ProductFilter_CodeFilter{
						In:    []string{"P001", "P002"},
						NotIn: []string{"P999"},
					},
				},
			},
		}

		camelCaseMap, err := toRawMap(protoFilter)
		require.NoError(t, err)
		_, err = filter.Transform(camelCaseMap, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "And", keyType: filter.KeyTypeLogical},
			{key: "Or", keyType: filter.KeyTypeLogical},
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Contains", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Contains", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
			{key: "Code", keyType: filter.KeyTypeField},
			{key: "In", keyType: filter.KeyTypeOperator},
			{key: "NotIn", keyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, processedKeys)
	})

	t.Run("verify fold is modifier key", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			Name: &testdatav1.ProductFilter_NameFilter{
				Contains: lo.ToPtr("test"),
				Fold:     true,
			},
		}

		camelCaseMap, err := toRawMap(protoFilter)
		require.NoError(t, err)
		_, err = filter.Transform(camelCaseMap, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "Name", keyType: filter.KeyTypeField},
			{key: "Contains", keyType: filter.KeyTypeOperator},
			{key: "Fold", keyType: filter.KeyTypeModifier},
		}
		assert.Subset(t, processedKeys, expected)
	})
}
