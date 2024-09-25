package relay

import (
	"context"

	"github.com/samber/lo"
)

func PrimaryOrderBy[T any](primaryOrderBys ...OrderBy) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
			req.OrderBys = AppendPrimaryOrderBy[T](req.OrderBys, primaryOrderBys...)
			return next.Paginate(ctx, req)
		})
	}
}

func AppendPrimaryOrderBy[T any](orderBys []OrderBy, primaryOrderBys ...OrderBy) []OrderBy {
	if len(primaryOrderBys) == 0 {
		return orderBys
	}
	orderByFields := lo.SliceToMap(orderBys, func(orderBy OrderBy) (string, bool) {
		return orderBy.Field, true
	})
	// If there are fields in primaryOrderBys that are not in orderBys, add them to orderBys
	for _, primaryOrderBy := range primaryOrderBys {
		if _, ok := orderByFields[primaryOrderBy.Field]; !ok {
			orderBys = append(orderBys, primaryOrderBy)
		}
	}
	return orderBys
}
