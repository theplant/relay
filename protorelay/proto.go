package protorelay

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/theplant/relay"
	relayv1 "github.com/theplant/relay/protorelay/gen/relay/v1"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Integer is a type constraint for integer types.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// PtrAs converts a pointer to a different integer type.
func PtrAs[From, To Integer](v *From) *To {
	if v == nil {
		return nil
	}
	return lo.ToPtr(To(*v))
}

// ParsePagination parses a proto pagination to a paginate request.
func ParsePagination[T any](p *relayv1.Pagination, orderBy ...relay.Order) *relay.PaginateRequest[T] {
	return &relay.PaginateRequest[T]{
		OrderBy: orderBy,
		After:   p.After,
		Before:  p.Before,
		First:   PtrAs[int32, int](p.First),
		Last:    PtrAs[int32, int](p.Last),
	}
}

// Order is an interface that all proto order messages must implement.
type Order[T protoreflect.Enum] interface {
	GetField() T
	GetDirection() relayv1.OrderDirection
}

// ParseOrderBy parses proto order messages to OrderBy.
func ParseOrderBy[T protoreflect.Enum, O Order[T]](orderBy []O, defaultOrderBy []relay.Order) ([]relay.Order, error) {
	if len(orderBy) == 0 {
		return defaultOrderBy, nil
	}

	result := make([]relay.Order, 0, len(orderBy))
	for _, o := range orderBy {
		field, err := ParseOrderField(o.GetField())
		if err != nil {
			return nil, err
		}

		var direction relay.OrderDirection
		switch o.GetDirection() {
		case relayv1.OrderDirection_ORDER_DIRECTION_ASC:
			direction = relay.OrderDirectionAsc
		case relayv1.OrderDirection_ORDER_DIRECTION_DESC:
			direction = relay.OrderDirectionDesc
		default:
			return nil, errors.Errorf("invalid order direction: %s", o.GetDirection())
		}

		result = append(result, relay.Order{
			Field:     field,
			Direction: direction,
		})
	}

	return result, nil
}

// ParseOrderField parses a proto order field to a string.
// e.g., "PRODUCT_ORDER_FIELD_CREATED_AT" -> "CreatedAt"
func ParseOrderField(field protoreflect.Enum) (string, error) {
	// Get enum value without prefix (e.g., "CREATED_AT")
	enumStr, err := ParseEnum(field)
	if err != nil {
		return "", errors.Wrap(err, "invalid order field")
	}

	// Convert SCREAMING_SNAKE_CASE to PascalCase
	// e.g., "CREATED_AT" -> "CreatedAt"
	return lo.PascalCase(enumStr), nil
}

const unspecifiedEnumValue = "UNSPECIFIED"

// ParseEnum parses a proto enum to a string which is the value of the enum without the prefix.
// e.g., "PRODUCT_STATUS_DRAFT" -> "DRAFT"
func ParseEnum(v protoreflect.Enum) (string, error) {
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
