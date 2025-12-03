package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartPascalCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic camelCase (common protobuf JSON format)
		{
			name:     "simple camelCase",
			input:    "categoryId",
			expected: "CategoryID",
		},
		{
			name:     "single word",
			input:    "name",
			expected: "Name",
		},
		{
			name:     "acronym at end",
			input:    "userId",
			expected: "UserID",
		},
		{
			name:     "acronym only",
			input:    "id",
			expected: "ID",
		},
		{
			name:     "multiple words",
			input:    "createdAt",
			expected: "CreatedAt",
		},

		// Consecutive uppercase letters handling
		{
			name:     "consecutive uppercase - HTMLParser",
			input:    "HTMLParser",
			expected: "HTMLParser",
		},
		{
			name:     "consecutive uppercase - XMLHttpRequest",
			input:    "XMLHttpRequest",
			expected: "XMLHTTPRequest",
		},
		{
			name:     "consecutive uppercase - getHTTPResponse",
			input:    "getHTTPResponse",
			expected: "GetHTTPResponse",
		},
		{
			name:     "consecutive uppercase - parseJSON",
			input:    "parseJSON",
			expected: "ParseJSON",
		},
		{
			name:     "consecutive uppercase at end - userID",
			input:    "userID",
			expected: "UserID",
		},
		{
			name:     "all uppercase acronym",
			input:    "URL",
			expected: "URL",
		},
		{
			name:     "consecutive uppercase - APIKey",
			input:    "APIKey",
			expected: "APIKey",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character lowercase",
			input:    "a",
			expected: "A",
		},
		{
			name:     "single character uppercase",
			input:    "A",
			expected: "A",
		},
		{
			name:     "multiple acronyms",
			input:    "httpURLConnection",
			expected: "HTTPURLConnection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SmartPascalCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase word",
			input:    "hello",
			expected: "Hello",
		},
		{
			name:     "already capitalized",
			input:    "Hello",
			expected: "Hello",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			expected: "A",
		},
		{
			name:     "camelCase - only first letter",
			input:    "categoryId",
			expected: "CategoryId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Capitalize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKeyPath_String(t *testing.T) {
	tests := []struct {
		name     string
		keyPath  KeyPath
		expected string
	}{
		{
			name:     "empty path",
			keyPath:  KeyPath{},
			expected: "",
		},
		{
			name:     "single key",
			keyPath:  KeyPath{"Name"},
			expected: "Name",
		},
		{
			name:     "simple nested path",
			keyPath:  KeyPath{"Name", "Eq"},
			expected: "Name.Eq",
		},
		{
			name:     "path with array index",
			keyPath:  KeyPath{"And", "[0]", "Name"},
			expected: "And[0].Name",
		},
		{
			name:     "path with multiple array indices",
			keyPath:  KeyPath{"And", "[0]", "Or", "[1]", "Name", "Eq"},
			expected: "And[0].Or[1].Name.Eq",
		},
		{
			name:     "array index at end",
			keyPath:  KeyPath{"And", "[0]"},
			expected: "And[0]",
		},
		{
			name:     "consecutive array indices",
			keyPath:  KeyPath{"Data", "[0]", "[1]", "Value"},
			expected: "Data[0][1].Value",
		},
		{
			name:     "relationship path",
			keyPath:  KeyPath{"Category", "Name", "Contains"},
			expected: "Category.Name.Contains",
		},
		{
			name:     "complex nested with relationship",
			keyPath:  KeyPath{"And", "[1]", "Category", "Name", "Eq"},
			expected: "And[1].Category.Name.Eq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.keyPath.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKeyPath_Last(t *testing.T) {
	tests := []struct {
		name     string
		keyPath  KeyPath
		expected string
	}{
		{
			name:     "empty path",
			keyPath:  KeyPath{},
			expected: "",
		},
		{
			name:     "single key",
			keyPath:  KeyPath{"Name"},
			expected: "Name",
		},
		{
			name:     "nested path",
			keyPath:  KeyPath{"Name", "Eq"},
			expected: "Eq",
		},
		{
			name:     "path with array index",
			keyPath:  KeyPath{"And", "[0]", "Name"},
			expected: "Name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.keyPath.Last()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformOutput_skip(t *testing.T) {
	tests := []struct {
		name     string
		output   *TransformOutput
		expected bool
	}{
		{
			name:     "nil output",
			output:   nil,
			expected: true,
		},
		{
			name:     "empty key",
			output:   &TransformOutput{Key: ""},
			expected: true,
		},
		{
			name:     "non-empty key",
			output:   &TransformOutput{Key: "Name"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.output.skip())
		})
	}
}

func TestTransform(t *testing.T) {
	t.Run("skip by returning nil", func(t *testing.T) {
		source := map[string]any{
			"Name": map[string]any{"Eq": "test"},
			"Code": map[string]any{"Eq": "P001"},
		}

		result, err := Transform(source, func(input *TransformInput) (*TransformOutput, error) {
			if input.KeyPath.Last() == "Code" {
				return nil, nil
			}
			return &TransformOutput{Key: input.KeyPath.Last(), Value: input.Value}, nil
		})

		require.NoError(t, err)
		assert.Contains(t, result, "Name")
		assert.NotContains(t, result, "Code")
	})

	t.Run("skip by returning empty key", func(t *testing.T) {
		source := map[string]any{
			"Name": map[string]any{"Eq": "test"},
			"Code": map[string]any{"Eq": "P001"},
		}

		result, err := Transform(source, func(input *TransformInput) (*TransformOutput, error) {
			if input.KeyPath.Last() == "Code" {
				return &TransformOutput{Key: ""}, nil
			}
			return &TransformOutput{Key: input.KeyPath.Last(), Value: input.Value}, nil
		})

		require.NoError(t, err)
		assert.Contains(t, result, "Name")
		assert.NotContains(t, result, "Code")
	})

	t.Run("skip operator by returning nil", func(t *testing.T) {
		source := map[string]any{
			"Name": map[string]any{
				"Eq":       "test",
				"Contains": "es",
			},
		}

		result, err := Transform(source, func(input *TransformInput) (*TransformOutput, error) {
			if input.KeyType == KeyTypeOperator && input.KeyPath.Last() == "Contains" {
				return nil, nil
			}
			return &TransformOutput{Key: input.KeyPath.Last(), Value: input.Value}, nil
		})

		require.NoError(t, err)
		nameFilter := result["Name"].(map[string]any)
		assert.Contains(t, nameFilter, "Eq")
		assert.NotContains(t, nameFilter, "Contains")
	})

	t.Run("skip logical by returning nil", func(t *testing.T) {
		source := map[string]any{
			"And": []any{
				map[string]any{"Name": map[string]any{"Eq": "test"}},
			},
			"Name": map[string]any{"Eq": "direct"},
		}

		result, err := Transform(source, func(input *TransformInput) (*TransformOutput, error) {
			if input.KeyType == KeyTypeLogical {
				return nil, nil
			}
			return &TransformOutput{Key: input.KeyPath.Last(), Value: input.Value}, nil
		})

		require.NoError(t, err)
		assert.NotContains(t, result, "And")
		assert.Contains(t, result, "Name")
	})

	t.Run("containers correspond to key path", func(t *testing.T) {
		source := map[string]any{
			"And": []any{
				map[string]any{
					"Name": map[string]any{"Eq": "test"},
				},
			},
		}

		var captured *TransformInput
		_, err := Transform(source, func(input *TransformInput) (*TransformOutput, error) {
			if input.KeyType == KeyTypeOperator && input.KeyPath.Last() == "Eq" {
				captured = input
			}
			return &TransformOutput{Key: input.KeyPath.Last(), Value: input.Value}, nil
		})

		require.NoError(t, err)
		require.NotNil(t, captured)

		// KeyPath: ["And", "[0]", "Name", "Eq"]
		assert.Equal(t, KeyPath{"And", "[0]", "Name", "Eq"}, captured.KeyPath)
		assert.Len(t, captured.Containers, 4)

		// Containers[0] is root map
		_, ok := captured.Containers[0].(map[string]any)
		assert.True(t, ok, "Containers[0] should be root map")

		// Containers[1] is []any (And's list)
		_, ok = captured.Containers[1].([]any)
		assert.True(t, ok, "Containers[1] should be []any")

		// Containers[2] is map[string]any ([0]'s map)
		_, ok = captured.Containers[2].(map[string]any)
		assert.True(t, ok, "Containers[2] should be map")

		// Containers[3] is map[string]any (Name's field result)
		_, ok = captured.Containers[3].(map[string]any)
		assert.True(t, ok, "Containers[3] should be map")
	})
}
