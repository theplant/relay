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
	TotalCount      int     `json:"totalCount,omitempty"`
	HasNextPage     bool    `json:"hasNextPage"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	StartCursor     *string `json:"startCursor"`
	EndCursor       *string `json:"endCursor"`
}

type PaginateResponse[T any] struct {
	Edges []Edge[T] `json:"edges,omitempty"`
	// Sometimes we need nodes only
	Nodes    []T      `json:"nodes,omitempty"`
	PageInfo PageInfo `json:"pageInfo"`
}

type Pagination[T any] interface {
	Paginate(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error)
}

type PaginationFunc[T any] func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error)

func (f PaginationFunc[T]) Paginate(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
	return f(ctx, req)
}

func New[T any](nodesOnly bool, maxLimit int, limitIfNotSet int, orderBysIfNotSet []OrderBy, applyCursorsFunc ApplyCursorsFunc[T]) Pagination[T] {
	if limitIfNotSet <= 0 {
		panic("limitIfNotSet must be greater than 0")
	}
	if maxLimit < limitIfNotSet {
		panic("maxLimit must be greater than or equal to limitIfNotSet")
	}
	if applyCursorsFunc == nil {
		panic("applyCursorsFunc must be set")
	}
	if len(orderBysIfNotSet) == 0 {
		panic("orderBysIfNotSet must be set")
	}
	return PaginationFunc[T](func(ctx context.Context, req *PaginateRequest[T]) (*PaginateResponse[T], error) {
		first, last := req.First, req.Last
		if first == nil && last == nil {
			if req.After == nil && req.Before != nil {
				last = &limitIfNotSet
			} else {
				first = &limitIfNotSet
			}
		}
		if first != nil && *first > maxLimit {
			return nil, errors.New("first must be less than or equal to max limit")
		}
		if last != nil && *last > maxLimit {
			return nil, errors.New("last must be less than or equal to max limit")
		}

		orderBys := req.OrderBys
		if len(orderBys) == 0 {
			orderBys = orderBysIfNotSet
		}

		dups := lo.FindDuplicatesBy(orderBys, func(item OrderBy) string {
			return item.Field
		})
		if (len(dups)) > 0 {
			return nil, errors.Errorf("duplicated order by fields %v", lo.Map(dups, func(item OrderBy, _ int) string {
				return item.Field
			}))
		}

		edges, nodes, pageInfo, err := EdgesToReturn(ctx, req.Before, req.After, first, last, orderBys, nodesOnly, applyCursorsFunc)
		if err != nil {
			return nil, err
		}
		return &PaginateResponse[T]{Edges: edges, Nodes: nodes, PageInfo: *pageInfo}, nil
	})
}

type ApplyCursorsRequest struct {
	Before   *string
	After    *string
	OrderBys []OrderBy
	Limit    int
	FromLast bool
}

type LazyEdge[T any] struct {
	Node   T
	Cursor func(ctx context.Context, node T) (string, error)
}

type ApplyCursorsResponse[T any] struct {
	Edges              []LazyEdge[T]
	TotalCount         int
	HasBeforeOrNext    bool // `before` exists or it's next exists
	HasAfterOrPrevious bool // `after` exists or it's previous exists
}

// https://relay.dev/graphql/connections.htm#ApplyCursorsToEdges()
type ApplyCursorsFunc[T any] func(ctx context.Context, req *ApplyCursorsRequest) (*ApplyCursorsResponse[T], error)

// https://relay.dev/graphql/connections.htm#sec-Pagination-algorithm
// https://relay.dev/graphql/connections.htm#sec-undefined.PageInfo.Fields
func EdgesToReturn[T any](
	ctx context.Context,
	before, after *string, first, last *int,
	orderBys []OrderBy,
	nodesOnly bool,
	applyCursorsFunc ApplyCursorsFunc[T],
) (edges []Edge[T], nodes []T, pageInfo *PageInfo, err error) {
	if first != nil && last != nil {
		return nil, nil, nil, errors.New("first and last cannot be used together")
	}
	if first != nil && *first < 0 {
		return nil, nil, nil, errors.New("first must be a non-negative integer")
	}
	if last != nil && *last < 0 {
		return nil, nil, nil, errors.New("last must be a non-negative integer")
	}

	var limit int
	if first != nil {
		limit = *first + 1
	} else {
		limit = *last + 1
	}

	result, err := applyCursorsFunc(ctx, &ApplyCursorsRequest{
		Before:   before,
		After:    after,
		OrderBys: orderBys,
		Limit:    limit,
		FromLast: last != nil,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	lazyEdges := result.Edges

	var hasPreviousPage, hasNextPage bool

	if first != nil && len(lazyEdges) > *first {
		lazyEdges = lazyEdges[:*first]
		hasNextPage = true
	}
	if before != nil && result.HasBeforeOrNext {
		hasNextPage = true
	}

	if last != nil && len(lazyEdges) > *last {
		lazyEdges = lazyEdges[len(lazyEdges)-*last:]
		hasPreviousPage = true
	}
	if after != nil && result.HasAfterOrPrevious {
		hasPreviousPage = true
	}

	edges = make([]Edge[T], len(lazyEdges))
	for i, lazyEdge := range lazyEdges {
		if !nodesOnly || i == 0 || i == len(lazyEdges)-1 {
			cursor, err := lazyEdge.Cursor(ctx, lazyEdge.Node)
			if err != nil {
				return nil, nil, nil, err
			}
			edges[i] = Edge[T]{Node: lazyEdge.Node, Cursor: cursor}
		} else {
			edges[i] = Edge[T]{Node: lazyEdge.Node, Cursor: ""}
		}
	}

	pageInfo = &PageInfo{
		TotalCount:      result.TotalCount,
		HasNextPage:     hasNextPage,
		HasPreviousPage: hasPreviousPage,
	}
	if len(edges) > 0 {
		startCursor := edges[0].Cursor
		pageInfo.StartCursor = &startCursor
		endCursor := edges[len(edges)-1].Cursor
		pageInfo.EndCursor = &endCursor
	}

	if nodesOnly {
		nodes = make([]T, len(lazyEdges))
		for i, lazyEdge := range lazyEdges {
			nodes[i] = lazyEdge.Node
		}
		return nil, nodes, pageInfo, nil
	}

	return edges, nil, pageInfo, nil
}
