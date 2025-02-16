package gormfilter

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func Scope(filter any) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if db == nil {
			return nil
		}
		fdb, err := addFilter(db, filter)
		if err != nil {
			db.AddError(err)
			return db
		}
		return fdb
	}
}

const FilterTagKey = "relay"

// use strcut field name as key and force emit empty
var jsoniterForFilter = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	TagKey:                 FilterTagKey,
}.Froze()

func addFilter(db *gorm.DB, filter any) (*gorm.DB, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	if filter == nil {
		return db, nil
	}

	model := cmp.Or(db.Statement.Model, db.Statement.Dest)
	if model == nil {
		return nil, errors.New("model is nil")
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return nil, errors.Wrap(err, "parse schema with db")
	}

	data, err := jsoniterForFilter.Marshal(filter)
	if err != nil {
		return nil, errors.Wrap(err, "marshal filter")
	}

	var filterMap map[string]any
	if err := jsoniterForFilter.Unmarshal(data, &filterMap); err != nil {
		return nil, errors.Wrap(err, "unmarshal filter")
	}

	expr, err := buildFilterExpr(stmt, filterMap)
	if err != nil {
		return nil, err
	}
	if expr != nil {
		db = db.Where(expr)
	}
	return db, nil
}

func buildFilterExpr(stmt *gorm.Statement, filterMap map[string]any) (clause.Expression, error) {
	var exprs []clause.Expression

	keys := lo.Keys(filterMap)
	sort.Strings(keys)

	for _, key := range keys {
		value := filterMap[key]
		if value == nil {
			continue
		}

		switch key {
		case "And", "Or":
			filters, ok := value.([]any)
			if !ok {
				return nil, errors.Errorf("invalid %s filter format", strings.ToUpper(key))
			}
			var subExprs []clause.Expression
			for _, f := range filters {
				filterData, ok := f.(map[string]any)
				if !ok {
					return nil, errors.Errorf("invalid filter in %s array", strings.ToUpper(key))
				}
				expr, err := buildFilterExpr(stmt, filterData)
				if err != nil {
					return nil, err
				}
				if expr != nil {
					subExprs = append(subExprs, expr)
				}
			}
			if len(subExprs) > 0 {
				if key == "And" {
					exprs = append(exprs, clause.And(subExprs...))
				} else {
					exprs = append(exprs, clause.Or(subExprs...))
				}
			}

		case "Not":
			filterData, ok := value.(map[string]any)
			if !ok {
				return nil, errors.New("invalid NOT filter format")
			}
			expr, err := buildFilterExpr(stmt, filterData)
			if err != nil {
				return nil, err
			}
			if expr != nil {
				exprs = append(exprs, clause.Not(expr))
			}

		default:
			filterData, ok := value.(map[string]any)
			if !ok {
				return nil, errors.Errorf("invalid filter format for field %s", key)
			}
			expr, err := buildFilterFieldExpr(stmt, key, filterData)
			if err != nil {
				return nil, err
			}
			if expr != nil {
				exprs = append(exprs, expr)
			}
		}
	}

	return combineExprs(exprs...)
}

func buildFilterFieldExpr(stmt *gorm.Statement, fieldName string, filter map[string]any) (clause.Expression, error) {
	field, ok := stmt.Schema.FieldsByName[fieldName]
	if !ok {
		return nil, errors.Errorf("missing field %q in schema", fieldName)
	}

	var exprs []clause.Expression

	fold := false
	if v, ok := filter["Fold"].(bool); ok && v {
		fold = true
	}

	var column any
	column = clause.Column{Table: stmt.Table, Name: field.DBName}
	if fold {
		column = clause.Expr{SQL: fmt.Sprintf(`LOWER(%s)`, stmt.Quote(column))}
	}

	ops := lo.Keys(filter)
	sort.Strings(ops)

	for _, op := range ops {
		value := filter[op]
		if value == nil || op == "Fold" {
			continue
		}

		var expr clause.Expression

		switch op {
		case "Eq", "Neq":
			value = foldValue(value, fold)
			if op == "Eq" {
				expr = clause.Eq{Column: column, Value: value}
			} else {
				expr = clause.Neq{Column: column, Value: value}
			}

		case "In", "NotIn":
			arr, ok := value.([]any)
			if !ok {
				return nil, errors.Errorf("invalid %s values for field %q", strings.ToUpper(op), fieldName)
			}
			if fold {
				arr = foldArray(arr)
			}
			expr = clause.IN{Column: column, Values: arr}
			if op != "In" {
				expr = clause.Not(expr)
			}

		case "Lt", "Lte", "Gt", "Gte":
			value = foldValue(value, fold)
			switch op {
			case "Lt":
				expr = clause.Lt{Column: column, Value: value}
			case "Lte":
				expr = clause.Lte{Column: column, Value: value}
			case "Gt":
				expr = clause.Gt{Column: column, Value: value}
			case "Gte":
				expr = clause.Gte{Column: column, Value: value}
			}

		case "Contains", "StartsWith", "EndsWith":
			str, ok := value.(string)
			if !ok {
				return nil, errors.Errorf("invalid %s value for field %q", strings.ToUpper(op), fieldName)
			}
			if fold {
				str = strings.ToLower(str)
			}
			pattern := str
			switch op {
			case "Contains":
				pattern = "%" + str + "%"
			case "StartsWith":
				pattern = str + "%"
			case "EndsWith":
				pattern = "%" + str
			}
			expr = clause.Like{Column: column, Value: pattern}

		case "IsNull":
			isNull, ok := value.(bool)
			if !ok {
				return nil, errors.Errorf("invalid IS NULL value for field %q", fieldName)
			}
			if isNull {
				expr = clause.Eq{Column: column, Value: nil}
			} else {
				expr = clause.Neq{Column: column, Value: nil}
			}

		default:
			return nil, errors.Errorf("unknown operator %s for field %q", op, fieldName)
		}

		if expr != nil {
			exprs = append(exprs, expr)
		}
	}

	return combineExprs(exprs...)
}

// foldValue handles case folding for a single value, converting strings to lowercase if case-insensitive comparison is needed
func foldValue(value any, fold bool) any {
	if str, ok := value.(string); ok && fold {
		return strings.ToLower(str)
	}
	return value
}

// foldArray handles case folding for array values, converting all strings to lowercase
func foldArray(arr []any) []any {
	result := make([]any, len(arr))
	for i, v := range arr {
		if str, ok := v.(string); ok {
			result[i] = strings.ToLower(str)
		} else {
			result[i] = v
		}
	}
	return result
}

// combineExprs combines multiple expressions into a single expression
func combineExprs(exprs ...clause.Expression) (clause.Expression, error) {
	switch len(exprs) {
	case 0:
		return nil, nil
	case 1:
		return exprs[0], nil
	default:
		return clause.And(exprs...), nil
	}
}
