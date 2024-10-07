package gormrelay

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OffsetFinder[T any] struct {
	db *gorm.DB
}

func NewOffsetFinder[T any](db *gorm.DB) *OffsetFinder[T] {
	return &OffsetFinder[T]{
		db: db,
	}
}

func (a *OffsetFinder[T]) Find(ctx context.Context, orderBys []relay.OrderBy, skip, limit int) ([]T, error) {
	var nodes []T

	if limit == 0 {
		return nodes, nil
	}

	db := a.db
	if db.Statement.Context != ctx {
		db = db.WithContext(ctx)
	}

	if skip > 0 {
		db = db.Offset(skip)
	}

	db = db.Limit(limit)

	basedOnModel, err := shouldBasedOnModel[T](db)
	if err != nil {
		return nil, err
	}

	if !basedOnModel && db.Statement.Model == nil {
		var t T
		db = db.Model(t)
	}

	if len(orderBys) > 0 {
		s, err := parseSchema(db, db.Statement.Model)
		if err != nil {
			return nil, err
		}

		orderByColumns := make([]clause.OrderByColumn, 0, len(orderBys))
		for _, orderBy := range orderBys {
			field, ok := s.FieldsByName[orderBy.Field]
			if !ok {
				return nil, errors.Errorf("missing field %q in schema", orderBy.Field)
			}

			orderByColumns = append(orderByColumns, clause.OrderByColumn{
				Column: clause.Column{Table: clause.CurrentTable, Name: field.DBName},
				Desc:   orderBy.Desc,
			})
		}
		db = db.Order(clause.OrderBy{Columns: orderByColumns})
	}

	if basedOnModel {
		modelType := reflect.TypeOf(db.Statement.Model)
		sliceType := reflect.SliceOf(modelType)
		nodesVal := reflect.New(sliceType).Elem()

		err := db.Find(nodesVal.Addr().Interface()).Error
		if err != nil {
			return nil, errors.Wrap(err, "find")
		}

		nodes := make([]T, nodesVal.Len())
		for i := 0; i < nodesVal.Len(); i++ {
			nodes[i] = nodesVal.Index(i).Interface().(T)
		}

		return nodes, nil
	}

	if err := db.Find(&nodes).Error; err != nil {
		return nil, errors.Wrap(err, "find")
	}
	return nodes, nil
}

func (a *OffsetFinder[T]) Count(ctx context.Context) (int, error) {
	db := a.db

	basedOnModel, err := shouldBasedOnModel[T](db)
	if err != nil {
		return 0, err
	}

	if !basedOnModel && db.Statement.Context != ctx {
		db = db.WithContext(ctx)
	}

	if db.Statement.Model == nil {
		var t T
		db = db.Model(t)
	}

	var totalCount int64
	if err := db.Count(&totalCount).Error; err != nil {
		return 0, errors.Wrap(err, "count")
	}
	return int(totalCount), nil
}

func NewOffsetAdapter[T any](db *gorm.DB) relay.ApplyCursorsFunc[T] {
	return cursor.NewOffsetAdapter(NewOffsetFinder[T](db))
}
