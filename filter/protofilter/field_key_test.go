package protofilter

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theplant/relay/filter"
	testdatav1 "github.com/theplant/relay/protorelay/testdata/gen/testdata/v1"
)

func TestKeyTypeParameter(t *testing.T) {
	// Track which keys were processed and their KeyType values
	type keyInfo struct {
		key     string
		keyType filter.KeyType
	}
	var processedKeys []keyInfo

	// Create a custom transform function that records all key transformations
	recordingTransform := func(input *TransformKeyInput) (*TransformKeyOutput, error) {
		processedKeys = append(processedKeys, keyInfo{
			key:     input.Key,
			keyType: input.KeyType,
		})
		// Just capitalize first letter for transformation
		return capitalizeFirst(input)
	}

	t.Run("simple field with operators", func(t *testing.T) {
		processedKeys = nil

		protoFilter := &testdatav1.ProductFilter{
			Name: &testdatav1.ProductFilter_NameFilter{
				Eq:       lo.ToPtr("Product A"),
				Contains: lo.ToPtr("A"),
			},
		}

		_, err := toMap(protoFilter, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "name", keyType: filter.KeyTypeField},
			{key: "eq", keyType: filter.KeyTypeOperator},
			{key: "contains", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
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

		_, err := toMap(protoFilter, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "and", keyType: filter.KeyTypeLogical},
			{key: "name", keyType: filter.KeyTypeField},
			{key: "eq", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
			{key: "code", keyType: filter.KeyTypeField},
			{key: "in", keyType: filter.KeyTypeOperator},
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

		_, err := toMap(protoFilter, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "or", keyType: filter.KeyTypeLogical},
			{key: "name", keyType: filter.KeyTypeField},
			{key: "contains", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
			{key: "code", keyType: filter.KeyTypeField},
			{key: "eq", keyType: filter.KeyTypeOperator},
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

		_, err := toMap(protoFilter, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "not", keyType: filter.KeyTypeLogical},
			{key: "name", keyType: filter.KeyTypeField},
			{key: "eq", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
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

		_, err := toMap(protoFilter, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "and", keyType: filter.KeyTypeLogical},
			{key: "or", keyType: filter.KeyTypeLogical},
			{key: "name", keyType: filter.KeyTypeField},
			{key: "contains", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
			{key: "name", keyType: filter.KeyTypeField},
			{key: "contains", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
			{key: "code", keyType: filter.KeyTypeField},
			{key: "in", keyType: filter.KeyTypeOperator},
			{key: "notIn", keyType: filter.KeyTypeOperator},
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

		_, err := toMap(protoFilter, recordingTransform)
		require.NoError(t, err)

		expected := []keyInfo{
			{key: "name", keyType: filter.KeyTypeField},
			{key: "contains", keyType: filter.KeyTypeOperator},
			{key: "fold", keyType: filter.KeyTypeModifier},
		}
		assert.Subset(t, processedKeys, expected)
	})
}
