package protofilter_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/theplant/relay/filter/protofilter"
	testdatav1 "github.com/theplant/relay/protorelay/testdata/gen/testdata/v1"
)

func toJSON(t *testing.T, v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	return string(data)
}

func TestToMap_EmitUnpopulated(t *testing.T) {
	t.Run("unset optional fields should not appear", func(t *testing.T) {
		// Only set Status, leave Name unset
		filter := &testdatav1.ProductFilter{
			Status: &testdatav1.ProductFilter_StatusFilter{
				Eq: lo.ToPtr(testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED),
			},
			// Name is not set (nil)
		}

		result, err := protofilter.ToMap(filter)
		require.NoError(t, err)

		expected := `{
  "Status": {
    "Eq": "PUBLISHED"
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))
	})

	t.Run("set optional fields with default values should appear", func(t *testing.T) {
		// Set Name with empty string (explicit default value)
		filter := &testdatav1.ProductFilter{
			Name: &testdatav1.ProductFilter_NameFilter{
				Eq:   lo.ToPtr(""), // Explicitly set to empty string
				Fold: false,        // Default value
			},
		}

		result, err := protofilter.ToMap(filter)
		require.NoError(t, err)

		expected := `{
  "Name": {
    "Eq": "",
    "Fold": false
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))
	})

	t.Run("nested optional messages", func(t *testing.T) {
		filter := &testdatav1.ProductFilter{
			Category: &testdatav1.CategoryFilter{
				Name: &testdatav1.CategoryFilter_NameFilter{
					Eq: lo.ToPtr("Electronics"),
				},
				// Code is not set
			},
		}

		result, err := protofilter.ToMap(filter)
		require.NoError(t, err)

		expected := `{
  "Category": {
    "Name": {
      "Eq": "Electronics",
      "Fold": false
    }
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))
	})

	t.Run("And/Or logical operators with empty filters", func(t *testing.T) {
		filter := &testdatav1.ProductFilter{
			Or: []*testdatav1.ProductFilter{
				{
					Status: &testdatav1.ProductFilter_StatusFilter{
						Eq: lo.ToPtr(testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED),
					},
				},
				{
					Name: &testdatav1.ProductFilter_NameFilter{
						Eq: lo.ToPtr("Product"),
					},
				},
			},
		}

		result, err := protofilter.ToMap(filter)
		require.NoError(t, err)

		expected := `{
  "Or": [
    {
      "Status": {
        "Eq": "PUBLISHED"
      }
    },
    {
      "Name": {
        "Eq": "Product",
        "Fold": false
      }
    }
  ]
}`
		assert.JSONEq(t, expected, toJSON(t, result))
	})

	t.Run("wellknown types and enum handling", func(t *testing.T) {
		// Test timestamp (wellknown-type) and enum (single value and slice)
		createdAfter := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		updatedBefore := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

		filter := &testdatav1.ProductFilter{
			Status: &testdatav1.ProductFilter_StatusFilter{
				Eq: lo.ToPtr(testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED), // Single enum
				In: []testdatav1.ProductStatus{ // Enum slice
					testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED,
					testdatav1.ProductStatus_PRODUCT_STATUS_APPROVED,
					testdatav1.ProductStatus_PRODUCT_STATUS_PENDING_REVIEW,
				},
				NotIn: []testdatav1.ProductStatus{
					testdatav1.ProductStatus_PRODUCT_STATUS_DRAFT,
					testdatav1.ProductStatus_PRODUCT_STATUS_REJECTED,
				},
			},
			CreatedAt: &testdatav1.ProductFilter_CreatedAtFilter{
				Gte: timestamppb.New(createdAfter), // Wellknown-type: Timestamp
			},
			UpdatedAt: &testdatav1.ProductFilter_UpdatedAtFilter{
				Lt: timestamppb.New(updatedBefore), // Wellknown-type: Timestamp
			},
		}

		result, err := protofilter.ToMap(filter)
		require.NoError(t, err)

		expected := `{
  "CreatedAt": {
    "Gte": "2024-01-01T00:00:00Z"
  },
  "Status": {
    "Eq": "PUBLISHED",
    "In": ["PUBLISHED", "APPROVED", "PENDING_REVIEW"],
    "NotIn": ["DRAFT", "REJECTED"]
  },
  "UpdatedAt": {
    "Lt": "2024-12-31T23:59:59Z"
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))
	})
}

// Test model with Go naming conventions (ID, not Id)
type Product struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Name       string    `json:"name"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
	CategoryID string    `json:"categoryID"`
}

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func TestAlignWith(t *testing.T) {
	t.Run("maps proto camelCase to model PascalCase with ID acronym", func(t *testing.T) {
		filter := &testdatav1.ProductFilter{
			Status: &testdatav1.ProductFilter_StatusFilter{
				Eq: lo.ToPtr(testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED),
			},
			Name: &testdatav1.ProductFilter_NameFilter{
				Eq: lo.ToPtr("Test Product"),
			},
			CategoryId: &testdatav1.ProductFilter_CategoryIDFilter{
				Eq: lo.ToPtr("cat-123"),
			},
		}

		// Use AlignWith to align field names with Product model
		result, err := protofilter.ToMap(
			filter,
			protofilter.WithTransformKeyHook(protofilter.AlignWith(Product{})),
		)
		require.NoError(t, err)

		expected := `{
  "CategoryID": {
    "Eq": "cat-123"
  },
  "Name": {
    "Eq": "Test Product",
    "Fold": false
  },
  "Status": {
    "Eq": "PUBLISHED"
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))

		// Verify CategoryID (not CategoryId)
		_, hasCategoryID := result["CategoryID"]
		assert.True(t, hasCategoryID, "should have CategoryID (with uppercase ID)")

		_, hasCategoryId := result["CategoryId"]
		assert.False(t, hasCategoryId, "should NOT have CategoryId (with lowercase Id)")
	})

	t.Run("without model type hook uses default capitalization", func(t *testing.T) {
		filter := &testdatav1.ProductFilter{
			CategoryId: &testdatav1.ProductFilter_CategoryIDFilter{
				Eq: lo.ToPtr("cat-123"),
			},
		}

		// Without ModelTypeTransformKeyHook
		result, err := protofilter.ToMap(filter)
		require.NoError(t, err)

		expected := `{
  "CategoryId": {
    "Eq": "cat-123"
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))

		// Verify it's CategoryId (default capitalization), not CategoryID
		_, hasCategoryId := result["CategoryId"]
		assert.True(t, hasCategoryId, "should have CategoryId (default capitalization)")
	})

	t.Run("nested filters also use model type mapping", func(t *testing.T) {
		filter := &testdatav1.ProductFilter{
			Category: &testdatav1.CategoryFilter{
				Name: &testdatav1.CategoryFilter_NameFilter{
					Eq: lo.ToPtr("Electronics"),
				},
			},
		}

		result, err := protofilter.ToMap(
			filter,
			protofilter.WithTransformKeyHook(protofilter.AlignWith(Product{})),
		)
		require.NoError(t, err)

		expected := `{
  "Category": {
    "Name": {
      "Eq": "Electronics",
      "Fold": false
    }
  }
}`
		assert.JSONEq(t, expected, toJSON(t, result))
	})

	t.Run("panics when model is nil", func(t *testing.T) {
		assert.Panics(t, func() {
			protofilter.AlignWith(nil)
		}, "should panic when model is nil")
	})
}
