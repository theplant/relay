package cursor

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theplant/relay"
)

type OffsetFinder[T any] interface {
	Find(ctx context.Context, orderBys []relay.OrderBy, skip, limit int) ([]T, error)
	Count(ctx context.Context) (int, error)
}

type OffsetFinderFunc[T any] func(ctx context.Context, orderBys []relay.OrderBy, skip, limit int) ([]T, error)

func (f OffsetFinderFunc[T]) Find(ctx context.Context, orderBys []relay.OrderBy, skip, limit int) ([]T, error) {
	return f(ctx, orderBys, skip, limit)
}

// NewOffsetAdapter creates a relay.ApplyCursorsFunc from an OffsetFinder.
// If you want to use `last!=nil&&before==nil`, you can't skip totalCount.
func NewOffsetAdapter[T any](finder OffsetFinder[T]) relay.ApplyCursorsFunc[T] {
	return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[T], error) {
		after, before, err := decodeOffsetCursors(req.After, req.Before)
		if err != nil {
			return nil, err
		}

		var totalCount *int
		if !relay.ShouldSkipTotalCount(ctx) {
			count, err := finder.Count(ctx)
			if err != nil {
				return nil, err
			}
			totalCount = &count
		}

		if req.FromLast && before == nil {
			if totalCount == nil {
				return nil, errors.New("totalCount is required for fromLast and nil before")
			}
			before = totalCount
		}

		limit, skip := req.Limit, 0
		if after != nil {
			skip = *after + 1
		} else if before != nil {
			skip = *before - limit
		}
		if skip < 0 {
			skip = 0
		}
		if before != nil {
			rangeLen := *before - skip
			if rangeLen <= 0 {
				rangeLen = 0
			}
			if limit > rangeLen {
				limit = rangeLen
			}
			if req.FromLast && limit < rangeLen {
				skip = *before - limit
			}
		}

		var edges []relay.LazyEdge[T]
		if limit <= 0 || (totalCount != nil && (skip >= *totalCount || *totalCount <= 0)) {
			edges = make([]relay.LazyEdge[T], 0)
		} else {
			nodes, err := finder.Find(ctx, req.OrderBys, skip, limit)
			if err != nil {
				return nil, err
			}
			edges = make([]relay.LazyEdge[T], len(nodes))
			for i, node := range nodes {
				i := i
				edges[i] = relay.LazyEdge[T]{
					Node: node,
					Cursor: func(_ context.Context, _ T) (string, error) {
						return EncodeOffsetCursor(skip + i), nil
					},
				}
			}
		}

		resp := &relay.ApplyCursorsResponse[T]{
			LazyEdges:  edges,
			TotalCount: totalCount,
		}

		if totalCount != nil {
			resp.HasAfterOrPrevious = after != nil && *after < *totalCount
			resp.HasBeforeOrNext = before != nil && *before < *totalCount
		} else {
			// If we don't have totalCount, it would be very costly to check whether after and before really exist,
			// So it is usually not worth it. Normally, checking that it is not nil is sufficient.
			resp.HasAfterOrPrevious = after != nil
			resp.HasBeforeOrNext = before != nil
		}

		return resp, nil
	}
}

func EncodeOffsetCursor(offset int) string {
	return strconv.Itoa(offset)
}

func DecodeOffsetCursor(cursor string) (int, error) {
	offset, err := strconv.Atoi(cursor)
	if err != nil {
		return 0, errors.Wrapf(err, "decode offset cursor %q", cursor)
	}
	return offset, nil
}

func decodeOffsetCursors(after, before *string) (afterOffset, beforeOffset *int, err error) {
	if after != nil {
		offset, err := DecodeOffsetCursor(*after)
		if err != nil {
			return nil, nil, err
		}
		afterOffset = &offset
	}
	if before != nil {
		offset, err := DecodeOffsetCursor(*before)
		if err != nil {
			return nil, nil, err
		}
		beforeOffset = &offset
	}
	if afterOffset != nil && *afterOffset < 0 {
		return nil, nil, errors.New("after < 0")
	}
	if beforeOffset != nil && *beforeOffset < 0 {
		return nil, nil, errors.New("before < 0")
	}
	if afterOffset != nil && before != nil && *afterOffset >= *beforeOffset {
		return nil, nil, errors.New("after >= before")
	}
	return afterOffset, beforeOffset, nil
}
