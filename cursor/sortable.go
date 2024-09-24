package cursor

import (
	"context"

	"github.com/samber/lo"
	relay "github.com/theplant/gorelay"
)

// KeysetEncodeBySortableFields is a wrapper for ApplyCursorsFunc that encodes the cursor using all sortable fields.
func KeysetEncodeBySortableFields[T any](sortableFields []string) relay.ApplyCursorsFuncWrapper[T] {
	sortableFields = lo.Uniq(sortableFields)
	return func(next relay.ApplyCursorsFunc[T]) relay.ApplyCursorsFunc[T] {
		return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[T], error) {
			resp, err := next(ctx, req)
			if err != nil {
				return nil, err
			}

			for i := range resp.Edges {
				edge := &resp.Edges[i]
				edge.Cursor = func(ctx context.Context, node T) (string, error) {
					return EncodeKeysetCursor(node, sortableFields)
				}
			}

			return resp, nil
		}
	}
}
