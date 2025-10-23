package relay

import "github.com/samber/lo"

// Deprecated: Kept for backward compatibility only. Do not use in new code. Use Order instead.
type OrderBy struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc"`
}

// Deprecated: Kept for backward compatibility only. Do not use in new code. Use Order instead.
func OrderByFromOrderBys(orderBys []OrderBy) []Order {
	return lo.Map(orderBys, func(orderBy OrderBy, _ int) Order {
		direction := OrderDirectionAsc
		if orderBy.Desc {
			direction = OrderDirectionDesc
		}
		return Order{
			Field:     orderBy.Field,
			Direction: direction,
		}
	})
}
