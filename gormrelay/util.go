package gormrelay

import (
	"reflect"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func parseSchema(db *gorm.DB, v any) (*schema.Schema, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(v); err != nil {
		return nil, errors.Wrap(err, "parse schema with db")
	}
	return stmt.Schema, nil
}

// If T is not a struct or struct pointer, we need to use db.Statement.Model to find or count
func shouldBasedOnModel[T any](db *gorm.DB) (bool, error) {
	tType := reflect.TypeOf((*T)(nil)).Elem()
	if tType.Kind() != reflect.Struct && (tType.Kind() != reflect.Ptr || tType.Elem().Kind() != reflect.Struct) {
		if db.Statement.Model == nil {
			return true, errors.New("db.Statement.Model is nil and T is not a struct or struct pointer")
		}
		return true, nil
	}
	return false, nil
}
