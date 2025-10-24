package relay

import "github.com/samber/lo"

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
