package relay

import (
	"context"
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
