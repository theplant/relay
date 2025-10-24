package gormrelay

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"sync"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// RowsSplitter wraps a gorm.Rows object and splits column data into different destinations
// based on column name splitColumns
type RowsSplitter struct {
	gorm.Rows
	splitColumns map[string]func(columnType *sql.ColumnType) any
	columnTypes  []*sql.ColumnType

	mu           sync.RWMutex
	splitResults []map[string]any
}

// NewRowsSplitter creates a new RowsSplitter instance
func NewRowsSplitter(rows gorm.Rows, splitColumns map[string]func(columnType *sql.ColumnType) any) (*RowsSplitter, error) {
	if rows == nil {
		return nil, errors.New("rows cannot be nil")
	}

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get column types from rows")
	}

	return &RowsSplitter{
		Rows:         rows,
		splitColumns: splitColumns,
		columnTypes:  columnTypes,
		splitResults: []map[string]any{},
	}, nil
}

// Scan intercepts the scan operation to split specific columns
// into mapped destinations while preserving original scan behavior for others
func (w *RowsSplitter) Scan(dest ...any) error {
	// Create values to scan into, splitColumns specific columns to custom destinations
	scanDest := make([]any, len(w.columnTypes))
	splitValues := make([]any, 0, len(w.splitColumns))
	splitColumns := make([]string, 0, len(w.splitColumns))

	// Track original dest index to scan position
	destIndex := 0

	// Assign scan destinations
	for i, columnType := range w.columnTypes {
		colName := columnType.Name()
		if factory, exists := w.splitColumns[colName]; exists {
			v := factory(columnType)
			if rt, ok := v.(reflect.Type); ok {
				v = reflect.New(reflect.PointerTo(rt)).Interface()
			}
			scanDest[i] = v
			splitValues = append(splitValues, v)
			splitColumns = append(splitColumns, colName)
		} else if destIndex < len(dest) {
			scanDest[i] = dest[destIndex]
			destIndex++
		} else {
			scanDest[i] = new(any)
		}
	}

	// Perform the actual scan
	if err := w.Rows.Scan(scanDest...); err != nil {
		return errors.Wrap(err, "failed to scan rows with mapped columns")
	}

	// Collect the custom destinations into a map[string]any
	result := make(map[string]any)

	// Use scanIntoMap to handle possible NULL values
	scanIntoMap(result, splitValues, splitColumns)

	w.mu.Lock()
	w.splitResults = append(w.splitResults, result)
	w.mu.Unlock()

	return nil
}

// from gorm
func scanIntoMap(mapValue map[string]any, values []any, columns []string) {
	for idx, column := range columns {
		if reflectValue := reflect.Indirect(reflect.Indirect(reflect.ValueOf(values[idx]))); reflectValue.IsValid() {
			mapValue[column] = reflectValue.Interface()
			if valuer, ok := mapValue[column].(driver.Valuer); ok {
				mapValue[column], _ = valuer.Value()
			} else if b, ok := mapValue[column].(sql.RawBytes); ok {
				mapValue[column] = string(b)
			}
		} else {
			mapValue[column] = nil
		}
	}
}

// Columns returns filtered column names based on splitColumns
func (w *RowsSplitter) Columns() ([]string, error) {
	result := make([]string, 0, len(w.columnTypes))

	for _, columnType := range w.columnTypes {
		colName := columnType.Name()
		if _, exists := w.splitColumns[colName]; !exists {
			result = append(result, colName)
		}
	}

	return result, nil
}

// ColumnTypes returns filtered column types based on splitColumns
func (w *RowsSplitter) ColumnTypes() ([]*sql.ColumnType, error) {
	if len(w.splitColumns) == 0 {
		return w.columnTypes, nil
	}

	result := make([]*sql.ColumnType, 0, len(w.columnTypes))

	for _, columnType := range w.columnTypes {
		colName := columnType.Name()
		if _, exists := w.splitColumns[colName]; !exists {
			result = append(result, columnType)
		}
	}

	return result, nil
}

// SplitResults returns the collected custom destinations after each scan
func (w *RowsSplitter) SplitResults() []map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.splitResults
}

// ScanOption configures the Scan operation.
type ScanOption func(*scanConfig)

type scanConfig struct {
	splitColumns map[string]func(columnType *sql.ColumnType) any
	splitDest    *[]map[string]any
}

// WithSplitter configures column splitting during scan.
// Columns matched by splitColumns will be intercepted and scanned into splitDest instead of dest.
//
// IMPORTANT: Columns intercepted by the splitColumns will NOT be available in dest.
// This is by design - the splitColumns "splits" those columns away from the main destination.
//
// Example:
//
//	SQL: SELECT id, name, priority FROM shops
//	splitColumns: {"priority": ...}
//	Result:
//	  - dest receives: id, name (priority is excluded)
//	  - splitDest receives: [{"priority": 1}, {"priority": 2}, ...]
func WithSplitter(
	splitColumns map[string]func(columnType *sql.ColumnType) any,
	splitDest *[]map[string]any,
) ScanOption {
	return func(c *scanConfig) {
		c.splitColumns = splitColumns
		c.splitDest = splitDest
	}
}

// Scan executes a query and scans the result into dest.
// When WithSplitter is provided, specific columns will be split into a separate destination.
// Inspired by https://github.com/go-gorm/gorm/blob/489a56329318ee9316bc8a73f035320df6855e53/finisher_api.go#L526-L549
func Scan(db *gorm.DB, dest any, opts ...ScanOption) (tx *gorm.DB) {
	cfg := &scanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	splitColumns := cfg.splitColumns
	splitDest := cfg.splitDest

	config := *db.Config
	currentLogger, newLogger := config.Logger, logger.Recorder.New()
	config.Logger = newLogger

	tx = db.Session(&gorm.Session{Initialized: true})
	tx.Config = &config

	sqlRows, err := tx.Rows()
	if err == nil {
		rows, err := NewRowsSplitter(sqlRows, splitColumns)
		if err != nil {
			rows.Close()
			_ = tx.AddError(err)
		} else {
			if rows.Next() {
				_ = splitScanRows(tx, rows, dest)
				if tx.Error == nil && splitDest != nil {
					*splitDest = rows.SplitResults()
				}
			} else {
				tx.RowsAffected = 0
				_ = tx.AddError(rows.Err())
			}
			_ = tx.AddError(rows.Close())
		}
	}

	currentLogger.Trace(tx.Statement.Context, newLogger.BeginAt, func() (string, int64) {
		return newLogger.SQL, tx.RowsAffected
	}, tx.Error)
	tx.Logger = currentLogger
	return
}

// From https://github.com/go-gorm/gorm/blob/489a56329318ee9316bc8a73f035320df6855e53/finisher_api.go#L576-L593
// but support gorm.Rows
func splitScanRows(tx *gorm.DB, rows gorm.Rows, dest any) error {
	if err := tx.Statement.Parse(dest); !errors.Is(err, schema.ErrUnsupportedDataType) {
		_ = tx.AddError(err)
	}
	tx.Statement.Dest = dest
	tx.Statement.ReflectValue = reflect.ValueOf(dest)
	for tx.Statement.ReflectValue.Kind() == reflect.Ptr {
		elem := tx.Statement.ReflectValue.Elem()
		if !elem.IsValid() {
			elem = reflect.New(tx.Statement.ReflectValue.Type().Elem())
			tx.Statement.ReflectValue.Set(elem)
		}
		tx.Statement.ReflectValue = elem
	}
	gorm.Scan(rows, tx, gorm.ScanInitialized)
	return tx.Error
}
