package cursor

import (
	"context"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	relay "github.com/theplant/gorelay"
)

type KeysetFinder[T any] interface {
	Find(ctx context.Context, after, before *map[string]any, orderBys []relay.OrderBy, limit int, fromLast bool) ([]T, error)
}

type KeysetFinderFunc[T any] func(ctx context.Context, after, before *map[string]any, orderBys []relay.OrderBy, limit int, fromLast bool) ([]T, error)

func (f KeysetFinderFunc[T]) Find(ctx context.Context, after, before *map[string]any, orderBys []relay.OrderBy, limit int, fromLast bool) ([]T, error) {
	return f(ctx, after, before, orderBys, limit, fromLast)
}

func NewKeysetAdapter[T any](finder KeysetFinder[T]) relay.ApplyCursorsFunc[T] {
	return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[T], error) {
		keys := lo.Map(req.OrderBys, func(item relay.OrderBy, _ int) string {
			return item.Field
		})

		after, before, err := decodeKeysetCursors[T](req.After, req.Before, keys)
		if err != nil {
			return nil, err
		}

		var totalCount int
		counter, ok := finder.(Counter)
		if ok {
			var err error
			totalCount, err = counter.Count(ctx)
			if err != nil {
				return nil, err
			}
		}

		cursorEncoder := func(_ context.Context, node T) (string, error) {
			return EncodeKeysetCursor(node, keys)
		}

		var edges []relay.LazyEdge[T]
		if req.Limit <= 0 || (counter != nil && totalCount <= 0) {
			edges = make([]relay.LazyEdge[T], 0)
		} else {
			nodes, err := finder.Find(ctx, after, before, req.OrderBys, req.Limit, req.FromLast)
			if err != nil {
				return nil, err
			}
			edges = make([]relay.LazyEdge[T], len(nodes))
			for i, node := range nodes {
				edges[i] = relay.LazyEdge[T]{
					Node:   node,
					Cursor: cursorEncoder,
				}
			}
		}

		resp := &relay.ApplyCursorsResponse[T]{
			Edges:      edges,
			TotalCount: totalCount,
			// If we don't have a counter, it would be very costly to check whether after and before really exist,
			// So it is usually not worth it. Normally, checking that it is not nil is sufficient.
			HasAfterOrPrevious: after != nil,
			HasBeforeOrNext:    before != nil,
		}
		return resp, nil
	}
}

const KeysetTagKey = "relay"

// use strcut field name as key and force emit empty
var jsoniterForKeyset = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	TagKey:                 KeysetTagKey,
}.Froze()

func EncodeKeysetCursor[T any](node T, keys []string) (string, error) {
	b, err := jsoniterForKeyset.Marshal(node)
	if err != nil {
		return "", errors.Wrap(err, "marshal cursor")
	}

	m := make(map[string]any)
	if err := jsoniterForKeyset.Unmarshal(b, &m); err != nil {
		return "", errors.Wrap(err, "unmarshal cursor")
	}

	keysMap := lo.SliceToMap(keys, func(key string) (string, bool) {
		return key, true
	})
	for k := range keysMap {
		if _, ok := m[k]; !ok {
			return "", errors.Errorf("key %q not found in node", k)
		}
	}
	for k := range m {
		if _, ok := keysMap[k]; !ok {
			delete(m, k)
		}
	}

	b, err = jsoniterForKeyset.Marshal(m)
	if err != nil {
		return "", errors.Wrap(err, "marshal filtered cursor")
	}
	return string(b), nil
}

func DecodeKeysetCursor[T any](cursor string, keys []string) (map[string]any, error) {
	var m map[string]any
	if err := jsoniterForKeyset.Unmarshal([]byte(cursor), &m); err != nil {
		return nil, errors.Wrap(err, "unmarshal cursor")
	}

	keysMap := lo.SliceToMap(keys, func(key string) (string, bool) {
		return key, true
	})
	for k := range keysMap {
		if _, ok := m[k]; !ok {
			return nil, errors.Errorf("key %q not found in cursor", k)
		}
	}
	for k := range m {
		if _, ok := keysMap[k]; !ok {
			delete(m, k)
		}
	}

	return m, nil
}

func decodeKeysetCursors[T any](after, before *string, keys []string) (afterKeyset, beforeKeyset *map[string]any, err error) {
	if after != nil && before != nil && *after == *before {
		return nil, nil, errors.New("after == before")
	}
	if after != nil {
		m, err := DecodeKeysetCursor[T](*after, keys)
		if err != nil {
			return nil, nil, err
		}
		afterKeyset = &m
	}
	if before != nil {
		m, err := DecodeKeysetCursor[T](*before, keys)
		if err != nil {
			return nil, nil, err
		}
		beforeKeyset = &m
	}
	return afterKeyset, beforeKeyset, nil
}
