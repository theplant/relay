// Package gormfilter provides powerful and type-safe filtering capabilities for GORM queries.
package gormfilter

import (
	"cmp"
	"fmt"
	"reflect"
	"sort"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type options struct {
	disableBelongsTo bool
}

type Option func(*options)

// WithDisableBelongsTo returns an option that disables filtering by belongs_to relationships.
// This can be useful when you want to prevent complex subqueries or restrict filtering to direct fields only.
func WithDisableBelongsTo() Option {
	return func(o *options) {
		o.disableBelongsTo = true
	}
}

// Scope returns a GORM scope function that applies the given filter to the query.
// The filter parameter should be a struct with fields matching the model's schema.
// Fields use filter types from the filter package (filter.String, filter.Int, etc.).
//
// The filter struct can include:
//   - Field filters using types from the filter package
//   - Logical operators: And, Or, Not
//   - Relationship filters (BelongsTo)
//
// Example:
//
//	type UserFilter struct {
//	    Name    *filter.String     `json:"name"`
//	    Age     *filter.Int        `json:"age"`
//	    Company *CompanyFilter     `json:"company"`  // BelongsTo relationship
//	    And     []*UserFilter      `json:"and"`
//	    Or      []*UserFilter      `json:"or"`
//	    Not     *UserFilter        `json:"not"`
//	}
//
//	// Simple filter
//	db.Scopes(gormfilter.Scope(&UserFilter{
//	    Name: &filter.String{Contains: lo.ToPtr("john"), Fold: true},
//	})).Find(&users)
//
//	// Complex filter with relationships
//	db.Scopes(gormfilter.Scope(&UserFilter{
//	    Age: &filter.Int{Gte: lo.ToPtr(18)},
//	    Company: &CompanyFilter{
//	        Name: &filter.String{Eq: lo.ToPtr("Tech Corp")},
//	    },
//	})).Find(&users)
//
//	// Logical combinations
//	db.Scopes(gormfilter.Scope(&UserFilter{
//	    Or: []*UserFilter{
//	        {Age: &filter.Int{Lt: lo.ToPtr(25)}},
//	        {Age: &filter.Int{Gt: lo.ToPtr(60)}},
//	    },
//	})).Find(&users)
//
// Options:
//   - WithDisableBelongsTo: Disables filtering by belongs_to relationships
func Scope(filter any, opts ...Option) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if db == nil {
			return nil
		}

		options := &options{}
		for _, opt := range opts {
			opt(options)
		}

		fdb, err := addFilter(db, filter, options)
		if err != nil {
			db.AddError(err)
			return db
		}
		return fdb
	}
}

const FilterTagKey = "~~~filter~~~"

// use strcut field name as key and force emit empty
var jsoniterForFilter = jsoniter.Config{
	EscapeHTML:             true,
	SortMapKeys:            true,
	ValidateJsonRawMessage: true,
	TagKey:                 FilterTagKey,
}.Froze()

func addFilter(db *gorm.DB, filter any, opts *options) (*gorm.DB, error) {
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

	expr, err := buildFilterExpr(stmt, filterMap, opts)
	if err != nil {
		return nil, err
	}
	if expr != nil {
		db = db.Where(expr)
	}
	return db, nil
}

func buildFilterExpr(stmt *gorm.Statement, filterMap map[string]any, opts *options) (clause.Expression, error) {
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
				expr, err := buildFilterExpr(stmt, filterData, opts)
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
				return nil, errors.Errorf("invalid NOT filter format")
			}
			expr, err := buildFilterExpr(stmt, filterData, opts)
			if err != nil {
				return nil, err
			}
			if expr != nil {
				exprs = append(exprs, ClauseNot(expr))
			}

		default:
			filterData, ok := value.(map[string]any)
			if !ok {
				return nil, errors.Errorf("invalid filter format for field %s", key)
			}
			expr, err := buildFilterFieldExpr(stmt, key, filterData, opts)
			if err != nil {
				return nil, err
			}
			if expr != nil {
				exprs = append(exprs, expr)
			}
		}
	}

	return clause.And(exprs...), nil
}

func buildFilterFieldExpr(stmt *gorm.Statement, fieldName string, filter map[string]any, opts *options) (clause.Expression, error) {
	// Check if this is a belongs_to relationship
	if rel := stmt.Schema.Relationships.Relations[fieldName]; rel != nil && rel.Type == schema.BelongsTo {
		if opts != nil && opts.disableBelongsTo {
			return nil, errors.Errorf("belongs_to filter is disabled for field %q", fieldName)
		}
		return buildBelongsToFilterExpr(stmt, rel, filter, opts)
	}

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
		case "Eq", "Neq", "Lt", "Lte", "Gt", "Gte":
			value = foldValue(value, fold)
			switch op {
			case "Eq":
				expr = clause.Eq{Column: column, Value: value}
			case "Neq":
				expr = clause.Neq{Column: column, Value: value}
			case "Lt":
				expr = clause.Lt{Column: column, Value: value}
			case "Lte":
				expr = clause.Lte{Column: column, Value: value}
			case "Gt":
				expr = clause.Gt{Column: column, Value: value}
			case "Gte":
				expr = clause.Gte{Column: column, Value: value}
			}

		case "In", "NotIn":
			arr, ok := value.([]any)
			if !ok {
				return nil, errors.Errorf("invalid %s values for field %q", op, fieldName)
			}
			if len(arr) == 0 {
				return nil, errors.Errorf("empty %s values for field %q", op, fieldName)
			}

			if fold {
				arr = foldArray(arr)
			}
			expr = clause.IN{Column: column, Values: arr}
			if op != "In" {
				expr = clause.Not(expr)
			}

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

		case "Contains", "StartsWith", "EndsWith":
			str, ok := value.(string)
			if !ok {
				return nil, errors.Errorf("invalid %s value for field %q", op, fieldName)
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

		default:
			return nil, errors.Errorf("unknown operator %s for field %q", op, fieldName)
		}

		if expr != nil {
			exprs = append(exprs, expr)
		}
	}

	return clause.And(exprs...), nil
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

func buildBelongsToFilterExpr(stmt *gorm.Statement, rel *schema.Relationship, filter map[string]any, opts *options) (clause.Expression, error) {
	if len(rel.References) == 0 {
		return nil, errors.Errorf("no references found for belongs_to relationship %q", rel.Name)
	}

	foreignKey := rel.References[0].ForeignKey
	if foreignKey == nil {
		return nil, errors.Errorf("foreign key not found for belongs_to relationship %q", rel.Name)
	}

	referencedKey := rel.References[0].PrimaryKey
	if referencedKey == nil {
		return nil, errors.Errorf("referenced key not found for belongs_to relationship %q", rel.Name)
	}

	modelType := rel.FieldSchema.ModelType
	var relatedModelInstance any
	if modelType.Kind() == reflect.Ptr {
		relatedModelInstance = reflect.New(modelType.Elem()).Interface()
	} else {
		relatedModelInstance = reflect.New(modelType).Interface()
	}

	subQuery := stmt.DB.Session(&gorm.Session{NewDB: true}).Model(relatedModelInstance)

	subQuery, err := addFilter(subQuery, filter, opts)
	if err != nil {
		return nil, err
	}

	// Wrap the dialector to use ? placeholders for database-agnostic SQL generation
	originalDialector := subQuery.Statement.DB.Dialector
	subQuery.Statement.DB.Dialector = &questionMarkDialector{Dialector: originalDialector}

	dryRunStmt := subQuery.
		Clauses(clause.Select{Columns: []clause.Column{
			{Table: clause.CurrentTable, Name: referencedKey.DBName},
		}}).
		Session(&gorm.Session{DryRun: true}).
		Find(nil).Statement

	return &BelongsToInExpr{
		Expr: clause.Expr{
			SQL:  dryRunStmt.SQL.String(),
			Vars: dryRunStmt.Vars,
		},
		ForeignKeyColumn: clause.Column{Table: stmt.Table, Name: foreignKey.DBName},
	}, nil
}

// questionMarkDialector wraps any dialector to always use ? placeholders
type questionMarkDialector struct {
	gorm.Dialector
}

func (d *questionMarkDialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v any) {
	writer.WriteByte('?')
}

type BelongsToInExpr struct {
	clause.Expr
	ForeignKeyColumn clause.Column
}

func (in *BelongsToInExpr) Build(builder clause.Builder) {
	builder.WriteQuoted(in.ForeignKeyColumn)
	builder.WriteString(" IN (")
	in.Expr.Build(builder)
	builder.WriteByte(')')
}

func (in *BelongsToInExpr) NegationBuild(builder clause.Builder) {
	builder.WriteQuoted(in.ForeignKeyColumn)
	builder.WriteString(" NOT IN (")
	in.Expr.Build(builder)
	builder.WriteByte(')')
}
