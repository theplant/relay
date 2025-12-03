package protofilter

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theplant/relay/filter"
	testdatav1 "github.com/theplant/relay/protorelay/testdata/gen/testdata/v1"
)

func TestTransformHook(t *testing.T) {
	type inputSnapshot struct {
		KeyPath []string
		KeyType filter.KeyType
	}
	var recordedInputs []inputSnapshot

	recordingHook := func(next filter.TransformFunc) filter.TransformFunc {
		return func(input *filter.TransformInput) (*filter.TransformOutput, error) {
			recordedInputs = append(recordedInputs, inputSnapshot{
				KeyPath: append([]string(nil), input.KeyPath...),
				KeyType: input.KeyType,
			})
			return next(input)
		}
	}

	t.Run("simple field with operators", func(t *testing.T) {
		recordedInputs = nil

		protoFilter := &testdatav1.ProductFilter{
			Name: &testdatav1.ProductFilter_NameFilter{
				Eq:       lo.ToPtr("Product A"),
				Contains: lo.ToPtr("A"),
			},
		}

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Name", "Eq"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"Name", "Fold"}, KeyType: filter.KeyTypeModifier},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("logical operators with nested fields", func(t *testing.T) {
		recordedInputs = nil

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

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"And"}, KeyType: filter.KeyTypeLogical},
			{KeyPath: []string{"And", "[0]", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[0]", "Name", "Eq"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"And", "[0]", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
			{KeyPath: []string{"And", "[1]", "Code"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[1]", "Code", "In"}, KeyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("or operator with nested fields", func(t *testing.T) {
		recordedInputs = nil

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

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"Or"}, KeyType: filter.KeyTypeLogical},
			{KeyPath: []string{"Or", "[0]", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Or", "[0]", "Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"Or", "[0]", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
			{KeyPath: []string{"Or", "[1]", "Code"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Or", "[1]", "Code", "Eq"}, KeyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("not operator with nested field", func(t *testing.T) {
		recordedInputs = nil

		protoFilter := &testdatav1.ProductFilter{
			Not: &testdatav1.ProductFilter{
				Name: &testdatav1.ProductFilter_NameFilter{
					Eq: lo.ToPtr("Product A"),
				},
			},
		}

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"Not"}, KeyType: filter.KeyTypeLogical},
			{KeyPath: []string{"Not", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Not", "Name", "Eq"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"Not", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("complex nested structure", func(t *testing.T) {
		recordedInputs = nil

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

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"And"}, KeyType: filter.KeyTypeLogical},
			{KeyPath: []string{"And", "[0]", "Or"}, KeyType: filter.KeyTypeLogical},
			{KeyPath: []string{"And", "[0]", "Or", "[0]", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[0]", "Or", "[0]", "Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"And", "[0]", "Or", "[0]", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
			{KeyPath: []string{"And", "[0]", "Or", "[1]", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[0]", "Or", "[1]", "Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"And", "[0]", "Or", "[1]", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
			{KeyPath: []string{"And", "[1]", "Code"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[1]", "Code", "In"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"And", "[1]", "Code", "NotIn"}, KeyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("verify fold is modifier key", func(t *testing.T) {
		recordedInputs = nil

		protoFilter := &testdatav1.ProductFilter{
			Name: &testdatav1.ProductFilter_NameFilter{
				Contains: lo.ToPtr("test"),
				Fold:     true,
			},
		}

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"Name", "Fold"}, KeyType: filter.KeyTypeModifier},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("relationship filter", func(t *testing.T) {
		recordedInputs = nil

		protoFilter := &testdatav1.ProductFilter{
			Category: &testdatav1.CategoryFilter{
				Name: &testdatav1.CategoryFilter_NameFilter{
					Contains: lo.ToPtr("Electronics"),
				},
				Code: &testdatav1.CategoryFilter_CodeFilter{
					In: []string{"ELEC", "TECH"},
				},
			},
		}

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"Category"}, KeyType: filter.KeyTypeRelationship},
			{KeyPath: []string{"Category", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Category", "Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"Category", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
			{KeyPath: []string{"Category", "Code"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"Category", "Code", "In"}, KeyType: filter.KeyTypeOperator},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})

	t.Run("relationship with logical operators", func(t *testing.T) {
		recordedInputs = nil

		protoFilter := &testdatav1.ProductFilter{
			And: []*testdatav1.ProductFilter{
				{
					Name: &testdatav1.ProductFilter_NameFilter{
						Contains: lo.ToPtr("Phone"),
					},
				},
				{
					Category: &testdatav1.CategoryFilter{
						Name: &testdatav1.CategoryFilter_NameFilter{
							Eq: lo.ToPtr("Electronics"),
						},
					},
				},
			},
		}

		_, err := ToMap(protoFilter, WithTransformHook(recordingHook))
		require.NoError(t, err)

		expected := []inputSnapshot{
			{KeyPath: []string{"And"}, KeyType: filter.KeyTypeLogical},
			{KeyPath: []string{"And", "[0]", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[0]", "Name", "Contains"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"And", "[0]", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
			{KeyPath: []string{"And", "[1]", "Category"}, KeyType: filter.KeyTypeRelationship},
			{KeyPath: []string{"And", "[1]", "Category", "Name"}, KeyType: filter.KeyTypeField},
			{KeyPath: []string{"And", "[1]", "Category", "Name", "Eq"}, KeyType: filter.KeyTypeOperator},
			{KeyPath: []string{"And", "[1]", "Category", "Name", "Fold"}, KeyType: filter.KeyTypeModifier},
		}
		assert.ElementsMatch(t, expected, recordedInputs)
	})
}
