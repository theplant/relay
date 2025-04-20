package gormrelay

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"gorm.io/gorm/clause"

	"github.com/theplant/relay/cursor"
)

type Option[T any] func(*Options[T])

type Options[T any] struct {
	Computed *Computed[T]
}

type Computed[T any] struct {
	Columns map[string]clause.Column
	ForScan func(ctx context.Context) (dest any, toCursorNodes func() []cursor.Node[T], err error)
}

func ComputedColumns(columns map[string]string) map[string]clause.Column {
	return lo.MapEntries(columns, func(key string, value string) (string, clause.Column) {
		value = strings.Trim(value, " ()")
		return key, clause.Column{Name: fmt.Sprintf("(%s)", value), Raw: true}
	})
}

func WithComputed[T any](computed *Computed[T]) Option[T] {
	return func(opts *Options[T]) {
		opts.Computed = computed
	}
}
