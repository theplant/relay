package relay

import (
	"context"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// PaginationMiddleware is a wrapper for Pagination (middleware pattern)
type PaginationMiddleware[T any] func(next Pagination[T]) Pagination[T]

func EnsureLimits[T any](maxLimit int, limitIfNotSet int) PaginationMiddleware[T] {
	if limitIfNotSet <= 0 {
		panic("limitIfNotSet must be greater than 0")
	}
	if maxLimit < limitIfNotSet {
		panic("maxLimit must be greater than or equal to limitIfNotSet")
	}
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*Connection[T], error) {
			if req.First == nil && req.Last == nil {
				if req.After == nil && req.Before != nil {
					req.Last = &limitIfNotSet
				} else {
					req.First = &limitIfNotSet
				}
			}
			if req.First != nil && *req.First > maxLimit {
				return nil, errors.New("first must be less than or equal to max limit")
			}
			if req.Last != nil && *req.Last > maxLimit {
				return nil, errors.New("last must be less than or equal to max limit")
			}
			return next.Paginate(ctx, req)
		})
	}
}

func EnsurePrimaryOrderBy[T any](primaryOrderBys ...OrderBy) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*Connection[T], error) {
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

// CursorMiddleware is a wrapper for ApplyCursorsFunc (middleware pattern)
type CursorMiddleware[T any] func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T]

type ctxCursorMiddlewares struct{}

func CursorMiddlewaresFromContext[T any](ctx context.Context) []CursorMiddleware[T] {
	middlewares, _ := ctx.Value(ctxCursorMiddlewares{}).([]CursorMiddleware[T])
	return middlewares
}

func AppendCursorMiddleware[T any](cursorMiddlewares ...CursorMiddleware[T]) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*Connection[T], error) {
			if len(cursorMiddlewares) > 0 {
				cursorMiddlewares := append(CursorMiddlewaresFromContext[T](ctx), cursorMiddlewares...)
				ctx = context.WithValue(ctx, ctxCursorMiddlewares{}, cursorMiddlewares)
			}
			return next.Paginate(ctx, req)
		})
	}
}

func chainCursorMiddlewares[T any](mws []CursorMiddleware[T]) CursorMiddleware[T] {
	return func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T] {
		for i := len(mws); i > 0; i-- {
			next = mws[i-1](next)
		}
		return next
	}
}

func chainPaginationMiddlewares[T any](mws []PaginationMiddleware[T]) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		for i := len(mws); i > 0; i-- {
			next = mws[i-1](next)
		}
		return next
	}
}
