package cursor

import (
	"context"

	"github.com/theplant/relay"
)

func PrimaryOrderBy[T any](primaryOrderBys ...relay.OrderBy) relay.CursorMiddleware[T] {
	return func(next relay.ApplyCursorsFunc[T]) relay.ApplyCursorsFunc[T] {
		return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[T], error) {
			req.OrderBys = relay.AppendPrimaryOrderBy[T](req.OrderBys, primaryOrderBys...)
			return next(ctx, req)
		}
	}
}
