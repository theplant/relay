package relay

import "context"

// ApplyCursorsMiddleware is a wrapper for ApplyCursorsFunc (middleware pattern)
type ApplyCursorsMiddleware[T any] func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T]

// PaginationMiddleware is a wrapper for Pagination (middleware pattern)
type PaginationMiddleware[T any] func(next Pagination[T]) Pagination[T]

type ctxApplyCursorsMiddlewares struct{}

func ApplyCursorsMiddlewaresFromContext[T any](ctx context.Context) []ApplyCursorsMiddleware[T] {
	middlewares, _ := ctx.Value(ctxApplyCursorsMiddlewares{}).([]ApplyCursorsMiddleware[T])
	return middlewares
}

func AppendApplyCursorsMiddleware[T any](ws ...ApplyCursorsMiddleware[T]) PaginationMiddleware[T] {
	return func(next Pagination[T]) Pagination[T] {
		return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
			middlewares := ApplyCursorsMiddlewaresFromContext[T](ctx)
			middlewares = append(middlewares, ws...)
			ctx = context.WithValue(ctx, ctxApplyCursorsMiddlewares{}, middlewares)
			return next.Paginate(ctx, req)
		})
	}
}

func PrimaryOrderBys[T any](primaryOrderBys ...OrderBy) ApplyCursorsMiddleware[T] {
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
