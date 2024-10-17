package relay

import (
	"context"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type OrderBy struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc"`
}

type PaginateRequest[T any] struct {
	After    *string   `json:"after"`
	First    *int      `json:"first"`
	Before   *string   `json:"before"`
	Last     *int      `json:"last"`
	OrderBys []OrderBy `json:"orderBys"`
}

type Edge[T any] struct {
	Node   T      `json:"node"`
	Cursor string `json:"cursor"`
}

type PageInfo struct {
	HasNextPage     bool    `json:"hasNextPage"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	StartCursor     *string `json:"startCursor"`
	EndCursor       *string `json:"endCursor"`
}

type PaginateResponse[T any] struct {
	Edges      []*Edge[T] `json:"edges,omitempty"`
	Nodes      []T        `json:"nodes,omitempty"`
	PageInfo   PageInfo   `json:"pageInfo"`
	TotalCount *int       `json:"totalCount,omitempty"`
}

type ApplyCursorsRequest struct {
	Before   *string
	After    *string
	OrderBys []OrderBy
	Limit    int
	FromEnd  bool
}

type LazyEdge[T any] struct {
	Node   T
	Cursor func(ctx context.Context, node T) (string, error)
}

type ApplyCursorsResponse[T any] struct {
	LazyEdges          []*LazyEdge[T]
	TotalCount         *int
	HasBeforeOrNext    bool // `before` exists or it's next exists
	HasAfterOrPrevious bool // `after` exists or it's previous exists
}

// https://relay.dev/graphql/connections.htm#ApplyCursorsToEdges()
type ApplyCursorsFunc[T any] func(ctx context.Context, req *ApplyCursorsRequest) (*ApplyCursorsResponse[T], error)

// https://relay.dev/graphql/connections.htm#sec-Pagination-algorithm
// https://relay.dev/graphql/connections.htm#sec-undefined.PageInfo.Fields
func Paginate[T any](ctx context.Context, req *PaginateRequest[T], applyCursorsFunc ApplyCursorsFunc[T]) (*PaginateResponse[T], error) {
	if applyCursorsFunc == nil {
		panic("applyCursorsFunc must be set")
	}

	if req.First == nil && req.Last == nil {
		return nil, errors.New("first or last must be set")
	}
	if req.First != nil && req.Last != nil {
		return nil, errors.New("first and last cannot be used together")
	}
	if req.First != nil && *req.First < 0 {
		return nil, errors.New("first must be a non-negative integer")
	}
	if req.Last != nil && *req.Last < 0 {
		return nil, errors.New("last must be a non-negative integer")
	}

	orderBys := req.OrderBys
	if len(orderBys) > 0 {
		dups := lo.FindDuplicatesBy(orderBys, func(item OrderBy) string {
			return item.Field
		})
		if (len(dups)) > 0 {
			return nil, errors.Errorf("duplicated order by fields %v", lo.Map(dups, func(item OrderBy, _ int) string {
				return item.Field
			}))
		}
	}

	var limit int
	if req.First != nil {
		limit = *req.First + 1
	} else {
		limit = *req.Last + 1
	}

	result, err := applyCursorsFunc(ctx, &ApplyCursorsRequest{
		Before:   req.Before,
		After:    req.After,
		OrderBys: orderBys,
		Limit:    limit,
		FromEnd:  req.Last != nil,
	})
	if err != nil {
		return nil, err
	}

	lazyEdges := result.LazyEdges

	var hasPreviousPage, hasNextPage bool

	if req.First != nil && len(lazyEdges) > *req.First {
		lazyEdges = lazyEdges[:*req.First]
		hasNextPage = true
	}
	if req.Before != nil && result.HasBeforeOrNext {
		hasNextPage = true
	}

	if req.Last != nil && len(lazyEdges) > *req.Last {
		lazyEdges = lazyEdges[len(lazyEdges)-*req.Last:]
		hasPreviousPage = true
	}
	if req.After != nil && result.HasAfterOrPrevious {
		hasPreviousPage = true
	}

	resp := &PaginateResponse[T]{}

	if !ShouldSkipEdges(ctx) {
		edges := make([]*Edge[T], len(lazyEdges))
		for i, lazyEdge := range lazyEdges {
			cursor, err := lazyEdge.Cursor(ctx, lazyEdge.Node)
			if err != nil {
				return nil, err
			}
			edges[i] = &Edge[T]{Node: lazyEdge.Node, Cursor: cursor}
		}
		resp.Edges = edges
	}

	if !ShouldSkipNodes(ctx) {
		nodes := make([]T, len(lazyEdges))
		for i, lazyEdge := range lazyEdges {
			nodes[i] = lazyEdge.Node
		}
		resp.Nodes = nodes
	}

	if !ShouldSkipTotalCount(ctx) {
		resp.TotalCount = result.TotalCount
	}

	pageInfo := PageInfo{
		HasNextPage:     hasNextPage,
		HasPreviousPage: hasPreviousPage,
	}
	if len(lazyEdges) > 0 {
		var startCursor, endCursor string
		if len(resp.Edges) > 0 {
			startCursor = resp.Edges[0].Cursor
			endCursor = resp.Edges[len(resp.Edges)-1].Cursor
		} else {
			startCursor, err = lazyEdges[0].Cursor(ctx, lazyEdges[0].Node)
			if err != nil {
				return nil, err
			}
			endIndex := len(lazyEdges) - 1
			if endIndex == 0 {
				endCursor = startCursor
			} else {
				endCursor, err = lazyEdges[endIndex].Cursor(ctx, lazyEdges[endIndex].Node)
				if err != nil {
					return nil, err
				}
			}
		}
		pageInfo.StartCursor = &startCursor
		pageInfo.EndCursor = &endCursor
	}
	resp.PageInfo = pageInfo

	return resp, nil
}

type Pagination[T any] interface {
	Paginate(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error)
}

type PaginationFunc[T any] func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error)

func (f PaginationFunc[T]) Paginate(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
	return f(ctx, req)
}

func New[T any](applyCursorsFunc ApplyCursorsFunc[T], middlewares ...PaginationMiddleware[T]) Pagination[T] {
	if applyCursorsFunc == nil {
		panic("applyCursorsFunc must be set")
	}

	var p Pagination[T] = PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
		applyCursorsFunc := applyCursorsFunc
		cursorMiddlewares := CursorMiddlewaresFromContext[T](ctx)
		for _, cursorMiddleware := range cursorMiddlewares {
			applyCursorsFunc = cursorMiddleware(applyCursorsFunc)
		}
		return Paginate(ctx, req, applyCursorsFunc)
	})
	for _, middleware := range middlewares {
		p = middleware(p)
	}
	return p
}
