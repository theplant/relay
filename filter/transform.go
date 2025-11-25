package filter

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// KeyType represents the type of a filter key
type KeyType string

const (
	// KeyTypeLogical represents a logical operator key (And, Or, Not)
	KeyTypeLogical KeyType = "LOGICAL"

	// KeyTypeField represents a model field key (Name, Code, CategoryID, etc.)
	KeyTypeField KeyType = "FIELD"

	// KeyTypeOperator represents a filter operator key (Eq, Contains, In, Gt, etc.)
	KeyTypeOperator KeyType = "OPERATOR"

	// KeyTypeModifier represents a modifier key that changes operator behavior (Fold, etc.)
	KeyTypeModifier KeyType = "MODIFIER"
)

// TransformInput provides input information for transformation
type TransformInput struct {
	KeyPath   []string
	KeyType   KeyType
	Value     any
	RootMap   map[string]any
	ParentMap map[string]any
	TargetMap map[string]any
}

// TransformOutput represents the result of transformation
type TransformOutput struct {
	Key   string
	Value any
}

// TransformFunc is a function that transforms keys and values.
type TransformFunc func(input *TransformInput) (*TransformOutput, error)

// Transform applies transformations to a filter map.
func Transform(sourceMap map[string]any, transform TransformFunc) (map[string]any, error) {
	if sourceMap == nil {
		return nil, nil
	}
	rootMap := make(map[string]any)
	if err := transformMap(sourceMap, rootMap, nil, nil, rootMap, transform); err != nil {
		return nil, err
	}
	return rootMap, nil
}

func transformMap(
	sourceMap map[string]any,
	targetMap map[string]any,
	parentPath []string,
	parentMap map[string]any,
	rootMap map[string]any,
	transform TransformFunc,
) error {
	for key, value := range sourceMap {
		if value == nil {
			continue
		}

		currentPath := appendPath(parentPath, key)

		lowerKey := strings.ToLower(key)
		isLogical := lowerKey == "and" || lowerKey == "or" || lowerKey == "not"

		if isLogical {
			if err := handleLogical(lowerKey, value, currentPath, targetMap, parentMap, rootMap, transform); err != nil {
				return err
			}
			continue
		}

		valueMap, ok := value.(map[string]any)
		if !ok {
			return errors.Errorf("field %s value should be map[string]any, got %T", key, value)
		}

		input := &TransformInput{
			KeyPath:   currentPath,
			KeyType:   KeyTypeField,
			Value:     value,
			RootMap:   rootMap,
			ParentMap: parentMap,
			TargetMap: targetMap,
		}
		output, err := transform(input)
		if err != nil {
			return errors.Wrapf(err, "transform key %s", strings.Join(currentPath, "."))
		}

		if output.Key == "" {
			continue
		}

		if isRelationshipFilterMap(valueMap) {
			nestedResult := make(map[string]any)
			if err := transformMap(valueMap, nestedResult, currentPath, targetMap, rootMap, transform); err != nil {
				return errors.Wrapf(err, "field %s", key)
			}
			targetMap[output.Key] = nestedResult
		} else {
			fieldResult := make(map[string]any)
			for opKey, opValue := range valueMap {
				opPath := appendPath(currentPath, opKey)
				keyType := KeyTypeOperator
				if strings.ToLower(opKey) == "fold" {
					keyType = KeyTypeModifier
				}
				opInput := &TransformInput{
					KeyPath:   opPath,
					KeyType:   keyType,
					Value:     opValue,
					RootMap:   rootMap,
					ParentMap: targetMap,
					TargetMap: fieldResult,
				}
				opOutput, err := transform(opInput)
				if err != nil {
					return errors.Wrapf(err, "transform key %s", strings.Join(opPath, "."))
				}
				if opOutput.Key != "" {
					fieldResult[opOutput.Key] = opOutput.Value
				}
			}
			targetMap[output.Key] = fieldResult
		}
	}
	return nil
}

func handleLogical(
	lowerKey string,
	value any,
	currentPath []string,
	targetMap map[string]any,
	parentMap map[string]any,
	rootMap map[string]any,
	transform TransformFunc,
) error {
	input := &TransformInput{
		KeyPath:   currentPath,
		KeyType:   KeyTypeLogical,
		Value:     value,
		RootMap:   rootMap,
		ParentMap: parentMap,
		TargetMap: targetMap,
	}
	output, err := transform(input)
	if err != nil {
		return err
	}

	if output.Key == "" {
		return nil
	}

	pathStr := strings.Join(currentPath, ".")
	switch lowerKey {
	case "and", "or":
		filterList, ok := value.([]any)
		if !ok {
			return errors.Errorf("logical filter %s should be []any, got %T", pathStr, value)
		}

		transformedList := make([]any, 0, len(filterList))
		for i, item := range filterList {
			subMap, ok := item.(map[string]any)
			if !ok {
				return errors.Errorf("logical filter %s item at index %d should be map[string]any, got %T", pathStr, i, item)
			}

			result := make(map[string]any)
			itemPath := appendPath(currentPath, fmt.Sprintf("[%d]", i))
			if err := transformMap(subMap, result, itemPath, targetMap, rootMap, transform); err != nil {
				return errors.Wrapf(err, "index %d", i)
			}
			transformedList = append(transformedList, result)
		}
		targetMap[output.Key] = transformedList

	case "not":
		subMap, ok := value.(map[string]any)
		if !ok {
			return errors.Errorf("logical filter %s should be map[string]any, got %T", pathStr, value)
		}

		result := make(map[string]any)
		if err := transformMap(subMap, result, currentPath, targetMap, rootMap, transform); err != nil {
			return err
		}
		targetMap[output.Key] = result
	}

	return nil
}

func appendPath(parent []string, key string) []string {
	result := make([]string, len(parent), len(parent)+1)
	copy(result, parent)
	return append(result, key)
}

func isRelationshipFilterMap(m map[string]any) bool {
	if len(m) == 0 {
		return false
	}

	knownKeys := map[string]bool{
		"eq":          true,
		"in":          true,
		"not_in":      true,
		"contains":    true,
		"starts_with": true,
		"ends_with":   true,
		"gt":          true,
		"gte":         true,
		"lt":          true,
		"lte":         true,
		"between":     true,
		"is_null":     true,
		"fold":        true,
	}

	for key := range m {
		snakeKey := lo.SnakeCase(key)
		if !knownKeys[snakeKey] {
			return true
		}
	}

	return false
}

// Capitalize simply capitalizes the first letter without acronym handling
func Capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// SmartPascalCase converts camelCase to PascalCase with proper handling of common acronyms
func SmartPascalCase(s string) string {
	if s == "" {
		return s
	}

	acronyms := map[string]bool{
		"id":    true,
		"url":   true,
		"uri":   true,
		"api":   true,
		"http":  true,
		"https": true,
		"html":  true,
		"xml":   true,
		"json":  true,
		"sql":   true,
		"uuid":  true,
		"uid":   true,
		"ip":    true,
		"tcp":   true,
		"udp":   true,
		"rpc":   true,
		"grpc":  true,
		"oauth": true,
		"jwt":   true,
		"ssh":   true,
		"tls":   true,
		"ssl":   true,
		"ui":    true,
		"ux":    true,
		"seo":   true,
		"cms":   true,
		"db":    true,
		"os":    true,
		"io":    true,
		"pdf":   true,
		"csv":   true,
		"svg":   true,
		"png":   true,
		"jpg":   true,
		"gif":   true,
		"ftp":   true,
		"smtp":  true,
		"pop":   true,
		"imap":  true,
		"dns":   true,
		"cdn":   true,
		"cpu":   true,
		"gpu":   true,
		"ram":   true,
		"rom":   true,
		"ssd":   true,
		"hdd":   true,
		"usb":   true,
		"lan":   true,
		"wan":   true,
		"vpn":   true,
		"nat":   true,
	}

	var words []string
	var currentWord strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if currentWord.Len() > 0 {
				words = append(words, strings.ToLower(currentWord.String()))
				currentWord.Reset()
			}
		}
		currentWord.WriteRune(r)
	}
	if currentWord.Len() > 0 {
		words = append(words, strings.ToLower(currentWord.String()))
	}

	var result strings.Builder
	for _, word := range words {
		if acronyms[word] {
			result.WriteString(strings.ToUpper(word))
		} else {
			if word != "" {
				result.WriteString(strings.ToUpper(word[:1]) + word[1:])
			}
		}
	}

	return result.String()
}

// WithSmartPascalCase creates a transform hook that uses SmartPascalCase for all keys
func WithSmartPascalCase() func(next TransformFunc) TransformFunc {
	return func(next TransformFunc) TransformFunc {
		return func(input *TransformInput) (*TransformOutput, error) {
			output, err := next(input)
			if err != nil {
				return nil, err
			}
			if len(input.KeyPath) > 0 {
				lastKey := input.KeyPath[len(input.KeyPath)-1]
				output.Key = SmartPascalCase(lastKey)
			}
			return output, nil
		}
	}
}
