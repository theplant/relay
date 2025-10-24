package relay

import (
	"context"

	"github.com/samber/lo"

	"github.com/theplant/relay/internal/hook"
)

// EnsureLimits ensures that the limit is within the range 0 -> maxLimit and uses defaultLimit if limit is not set or is negative
// This method introduced a breaking change in version 0.4.0, intentionally swapping the order of parameters to strongly indicate the breaking change.
// https://github.com/theplant/relay/compare/genx?expand=1#diff-02f50901140d6057da6310a106670552aa766a093efbc2200fb34c099b762131R14
func EnsureLimits[T any](defaultLimit, maxLimit int) func(next Paginator[T]) Paginator[T] {
	if defaultLimit < 0 {
		panic("defaultLimit cannot be negative")
	}
	if maxLimit < defaultLimit {
		panic("maxLimit must be greater than or equal to defaultLimit")
	}
	return func(next Paginator[T]) Paginator[T] {
		return PaginatorFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*Connection[T], error) {
			if req.First != nil {
				if *req.First > maxLimit {
					req.First = &maxLimit
				}
				if *req.First < 0 {
					req.First = &defaultLimit
				}
			}
			if req.Last != nil {
				if *req.Last > maxLimit {
					req.Last = &maxLimit
				}
				if *req.Last < 0 {
					req.Last = &defaultLimit
				}
			}
			if req.First == nil && req.Last == nil {
				if req.After == nil && req.Before != nil {
					req.Last = &defaultLimit
				} else {
					req.First = &defaultLimit
				}
			}
			return next.Paginate(ctx, req)
		})
	}
}

func EnsurePrimaryOrderBy[T any](primaryOrderBy ...Order) func(next Paginator[T]) Paginator[T] {
	return func(next Paginator[T]) Paginator[T] {
		return PaginatorFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*Connection[T], error) {
			req.OrderBy = AppendPrimaryOrderBy(req.OrderBy, primaryOrderBy...)
			return next.Paginate(ctx, req)
		})
	}
}

func AppendPrimaryOrderBy(orderBy []Order, primaryOrderBy ...Order) []Order {
	if len(primaryOrderBy) == 0 {
		return orderBy
	}
	orderByFields := lo.SliceToMap(orderBy, func(orderBy Order) (string, bool) {
		return orderBy.Field, true
	})
	// If there are fields in primaryOrderBy that are not in orderBy, add them to orderBy
	for _, primaryOrderBy := range primaryOrderBy {
		if _, ok := orderByFields[primaryOrderBy.Field]; !ok {
			orderBy = append(orderBy, primaryOrderBy)
		}
	}
	return orderBy
}

type ctxCursorHook struct{}

func CursorHookFromContext[T any](ctx context.Context) func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T] {
	hook, _ := ctx.Value(ctxCursorHook{}).(func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T])
	return hook
}

func PrependCursorHook[T any](hooks ...func(next ApplyCursorsFunc[T]) ApplyCursorsFunc[T]) func(next Paginator[T]) Paginator[T] {
	return func(next Paginator[T]) Paginator[T] {
		return PaginatorFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*Connection[T], error) {
			if len(hooks) > 0 {
				cursorHook := CursorHookFromContext[T](ctx)
				cursorHook = hook.Prepend(cursorHook, hooks...)
				ctx = context.WithValue(ctx, ctxCursorHook{}, cursorHook)
			}
			return next.Paginate(ctx, req)
		})
	}
}
