package relay

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	relayv1 "github.com/theplant/relay/gen/relay/v1"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// ProtoOrder is an interface that all proto order messages must implement.
type ProtoOrder[T protoreflect.Enum] interface {
	GetField() T
	GetDirection() relayv1.OrderDirection
}

// ParseProtoOrderBy parses proto order messages to OrderBy.
func ParseProtoOrderBy[T protoreflect.Enum, O ProtoOrder[T]](orderBy []O, defaultOrderBy []Order) ([]Order, error) {
	if len(orderBy) == 0 {
		return defaultOrderBy, nil
	}

	result := make([]Order, 0, len(orderBy))
	for _, o := range orderBy {
		field, err := ParseProtoOrderField(o.GetField())
		if err != nil {
			return nil, err
		}

		var direction OrderDirection
		switch o.GetDirection() {
		case relayv1.OrderDirection_ORDER_DIRECTION_ASC:
			direction = OrderDirectionAsc
		case relayv1.OrderDirection_ORDER_DIRECTION_DESC:
			direction = OrderDirectionDesc
		default:
			return nil, errors.Errorf("invalid order direction: %s", o.GetDirection())
		}

		result = append(result, Order{
			Field:     field,
			Direction: direction,
		})
	}

	return result, nil
}

// ParseProtoOrderField parses a proto order field to a string.
// e.g., "PRODUCT_ORDER_FIELD_CREATED_AT" -> "CreatedAt"
func ParseProtoOrderField(field protoreflect.Enum) (string, error) {
	// Get enum value without prefix (e.g., "CREATED_AT")
	enumStr, err := ParseProtoEnum(field)
	if err != nil {
		return "", errors.Wrap(err, "invalid order field")
	}

	// Convert SCREAMING_SNAKE_CASE to PascalCase
	// e.g., "CREATED_AT" -> "CreatedAt"
	return lo.PascalCase(enumStr), nil
}

const unspecifiedEnumValue = "UNSPECIFIED"

// ParseProtoEnum parses a proto enum to a string which is the value of the enum without the prefix.
// e.g., "PRODUCT_STATUS_DRAFT" -> "DRAFT"
func ParseProtoEnum(v protoreflect.Enum) (string, error) {
	// Calculate prefix from enum type name
	// e.g., "ProductStatus" -> "PRODUCT_STATUS_"
	descriptor := v.Descriptor()
	prefix := toScreamingSnakeCase(string(descriptor.Name())) + "_"

	// Use String() method to get full enum name
	// e.g., "PRODUCT_STATUS_DRAFT"
	stringer, ok := v.(interface{ String() string })
	if !ok {
		return "", errors.New("enum does not implement String() method")
	}
	fullName := stringer.String()

	// Remove prefix to get the actual value
	// e.g., "PRODUCT_STATUS_DRAFT" -> "DRAFT"
	if !strings.HasPrefix(fullName, prefix) {
		// Invalid enum values will return numeric strings like "99999"
		return "", errors.Errorf("invalid enum value %s (expected prefix %s)", fullName, prefix)
	}

	result := strings.TrimPrefix(fullName, prefix)
	if result == unspecifiedEnumValue {
		return "", errors.New("unspecified enum value")
	}

	return result, nil
}

var fixDigitalRegex = regexp.MustCompile(`_(\d+)`)

func toScreamingSnakeCase(s string) string {
	s = fixDigitalRegex.ReplaceAllString(lo.SnakeCase(s), "${1}")
	return strings.ToUpper(s)
}
