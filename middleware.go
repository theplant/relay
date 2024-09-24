package relay

import "context"

// Middleware is a wrapper for ApplyCursorsFunc (middleware pattern)
type Middleware[T any] func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T]

// PaginationMiddleware is a wrapper for Pagination (middleware pattern)
type PaginationMiddleware[T any] func(next Pagination[T]) Pagination[T]

type ctxMiddlewares struct{}

func MiddlewaresFromContext[T any](ctx context.Context) []Middleware[T] {
	middlewares, _ := ctx.Value(ctxMiddlewares{}).([]Middleware[T])
	return middlewares
}

func AppendMiddleware[T any](ws ...Middleware[T]) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
			middlewares := MiddlewaresFromContext[T](ctx)
			middlewares = append(middlewares, ws...)
			ctx = context.WithValue(ctx, ctxMiddlewares{}, middlewares)
			return next.Paginate(ctx, req)
		})
	}
}

func PrimaryOrderBys[T any](primaryOrderBys ...OrderBy) Middleware[T] {
	return func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T] {
		return func(ctx context.Context, req *ApplyCursorsRequest) (*ApplyCursorsResponse[T], error) {
			// If there are fields in primaryOrderBys that are not in orderBys, add them to orderBys
			for _, primaryOrderBy := range primaryOrderBys {
				found := false
				for _, orderBy := range req.OrderBys {
					if orderBy.Field == primaryOrderBy.Field {
						found = true
						break
					}
				}
				if !found {
					req.OrderBys = append(req.OrderBys, primaryOrderBy)
				}
			}
			return next(ctx, req)
		}
	}
}
