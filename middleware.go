package relay

import (
	"context"

	"github.com/pkg/errors"
)

// CursorMiddleware is a wrapper for ApplyCursorsFunc (middleware pattern)
type CursorMiddleware[T any] func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T]

// PaginationMiddleware is a wrapper for Pagination (middleware pattern)
type PaginationMiddleware[T any] func(next Pagination[T]) Pagination[T]

type ctxCursorMiddlewares struct{}

func CursorMiddlewaresFromContext[T any](ctx context.Context) []CursorMiddleware[T] {
	middlewares, _ := ctx.Value(ctxCursorMiddlewares{}).([]CursorMiddleware[T])
	return middlewares
}

func AppendCursorMiddleware[T any](ws ...CursorMiddleware[T]) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
			middlewares := append(CursorMiddlewaresFromContext[T](ctx), ws...)
			ctx = context.WithValue(ctx, ctxCursorMiddlewares{}, middlewares)
			return next.Paginate(ctx, req)
		})
	}
}

func EnsureLimits[T any](maxLimit int, limitIfNotSet int) PaginationMiddleware[T] {
	if limitIfNotSet <= 0 {
		panic("limitIfNotSet must be greater than 0")
	}
	if maxLimit < limitIfNotSet {
		panic("maxLimit must be greater than or equal to limitIfNotSet")
	}
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
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
