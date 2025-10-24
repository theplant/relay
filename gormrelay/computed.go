package gormrelay

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/tidwall/sjson"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/theplant/relay/cursor"
)

// WithComputed adds computed fields to a query.
func WithComputed[T any](computed *Computed[T]) Option[T] {
	return func(opts *options[T]) {
		opts.Computed = computed
	}
}

// Computed defines SQL expressions calculated at database level and attached to query results.
type Computed[T any] struct {
	// Maps field names to SQL expressions
	Columns map[string]clause.Column

	// Scanner prepares a scanner for database operations with computed fields
	Scanner func(db *gorm.DB) (*ComputedScanner[T], error)
}

// Validate ensures the Computed configuration is valid.
func (c *Computed[T]) Validate() error {
	if len(c.Columns) == 0 {
		return errors.New("Columns must not be empty")
	}

	for field, col := range c.Columns {
		if col.Alias != "" {
			return errors.Errorf("Column %q should have empty Alias, got %q", field, col.Alias)
		}
	}

	if c.Scanner == nil {
		return errors.New("Scanner function must not be nil")
	}

	aliasMap := make(map[string][]string)
	for fieldName := range c.Columns {
		alias := ComputedFieldToColumnAlias(fieldName)
		aliasMap[alias] = append(aliasMap[alias], fieldName)
	}

	var duplicates []string
	for alias, fields := range aliasMap {
		if len(fields) > 1 {
			duplicates = append(duplicates, fmt.Sprintf("alias %q is used by multiple fields: %v", alias, fields))
		}
	}

	if len(duplicates) > 0 {
		return errors.Errorf("duplicate computed field aliases detected:\n%s", strings.Join(duplicates, "\n"))
	}

	return nil
}

// NewComputedColumns creates a map of Column objects from field-to-SQL expression mappings.
func NewComputedColumns(columns map[string]string) map[string]clause.Column {
	return lo.MapEntries(columns, func(field string, value string) (string, clause.Column) {
		value = strings.Trim(value, " ()")
		return field, clause.Column{Name: fmt.Sprintf("(%s)", value), Raw: true}
	})
}

// ComputedFieldToColumnAlias generates a standardized SQL alias for computed fields.
var ComputedFieldToColumnAlias = func(field string) string {
	return "_relay_computed_" + lo.SnakeCase(field)
}

// withComputedResult combines an object with computed field values for JSON serialization.
type withComputedResult struct {
	Object          any
	ComputedResults map[string]any
}

// MarshalJSON implements json.Marshaler interface.
func (v *withComputedResult) MarshalJSON() ([]byte, error) {
	b, err := cursor.JSONMarshal(v.Object)
	if err != nil {
		return nil, err
	}

	for path, value := range v.ComputedResults {
		b, err = sjson.SetBytes(b, path, value)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

// WithComputedResult creates a wrapper that merges an object with computed field values.
func WithComputedResult(object any, computedResults map[string]any) *withComputedResult {
	return &withComputedResult{
		Object:          object,
		ComputedResults: computedResults,
	}
}

// NewComputedNode creates a cursor node with computed results for pagination.
// This is a convenience function for use in ComputedScanner.Transform.
func NewComputedNode[T any](node T, computedResults map[string]any) cursor.Node[T] {
	return &cursor.NodeWrapper[T]{
		Object: WithComputedResult(node, computedResults),
		Unwrap: func() T { return node },
	}
}

// ComputedScanner holds the configuration for scanning database results with computed fields.
type ComputedScanner[T any] struct {
	// Destination is the target for GORM to scan results into (e.g., &[]User{})
	Destination any

	// Transform converts scanned results with their computed values into cursor nodes
	Transform func(computedResults []map[string]any) []cursor.Node[T]
}

// NewComputedScanner creates a standard scanner for computed fields.
// This is the recommended implementation for most use cases.
// It wraps objects with their computed values and makes them accessible in the resulting cursor.Node objects.
func NewComputedScanner[T any](db *gorm.DB) (*ComputedScanner[T], error) {
	if db.Statement.Model == nil {
		return nil, errors.New("db.Statement.Model cannot be nil")
	}

	modelType := reflect.TypeOf(db.Statement.Model)
	genericType := reflect.TypeOf((*T)(nil)).Elem()

	// Only use []T when Model type exactly matches T
	if modelType == genericType {
		nodes := []T{}
		return &ComputedScanner[T]{
			Destination: &nodes,
			Transform: func(computedResults []map[string]any) []cursor.Node[T] {
				return lo.Map(nodes, func(node T, i int) cursor.Node[T] {
					return NewComputedNode(node, computedResults[i])
				})
			},
		}, nil
	}

	sliceType := reflect.SliceOf(modelType)
	nodesVal := reflect.New(sliceType).Elem()

	return &ComputedScanner[T]{
		Destination: nodesVal.Addr().Interface(),
		Transform: func(computedResults []map[string]any) []cursor.Node[T] {
			result := make([]cursor.Node[T], nodesVal.Len())
			for i := 0; i < nodesVal.Len(); i++ {
				node := nodesVal.Index(i).Interface().(T)
				result[i] = NewComputedNode(node, computedResults[i])
			}
			return result
		},
	}, nil
}

func computedSplitScan(tx *gorm.DB, dest any, computedColumns map[string]clause.Column) ([]map[string]any, error) {
	aliasToField := make(map[string]string)
	computedSplitColumns := make(map[string]func(columnType *sql.ColumnType) any)
	for field := range computedColumns {
		alias := ComputedFieldToColumnAlias(field)
		aliasToField[alias] = field
		computedSplitColumns[alias] = func(columnType *sql.ColumnType) any {
			return columnType.ScanType()
		}
	}

	computedResults := make([]map[string]any, 0)
	if err := Scan(tx, dest, WithSplitter(computedSplitColumns, &computedResults)).Error; err != nil {
		return nil, errors.Wrap(err, "failed to scan records with computed columns")
	}

	for i, result := range computedResults {
		computedResults[i] = lo.MapEntries(result, func(alias string, value any) (string, any) {
			return aliasToField[alias], value
		})
	}

	return computedResults, nil
}
