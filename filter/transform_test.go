package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
