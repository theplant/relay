package filter

import (
	"time"
)

// String provides filtering operations for string fields.
// All comparison operations can be case-insensitive when Fold is set to true.
type String struct {
	Eq         *string
	Neq        *string
	Lt         *string
	Lte        *string
	Gt         *string
	Gte        *string
	In         []string
	NotIn      []string
	IsNull     *bool
	Contains   *string
	StartsWith *string
	EndsWith   *string
	Fold       bool
}

// ID is an alias for String, used for identifier fields.
type ID String

// Float provides filtering operations for float64 fields.
type Float struct {
	Eq     *float64
	Neq    *float64
	Lt     *float64
	Lte    *float64
	Gt     *float64
	Gte    *float64
	In     []float64
	NotIn  []float64
	IsNull *bool
}

// Int provides filtering operations for integer fields.
type Int struct {
	Eq     *int
	Neq    *int
	Lt     *int
	Lte    *int
	Gt     *int
	Gte    *int
	In     []int
	NotIn  []int
	IsNull *bool
}

// Boolean provides filtering operations for boolean fields.
type Boolean struct {
	Eq     *bool
	Neq    *bool
	IsNull *bool
}

// Time provides filtering operations for time.Time fields.
type Time struct {
	Eq     *time.Time
	Neq    *time.Time
	Lt     *time.Time
	Lte    *time.Time
	Gt     *time.Time
	Gte    *time.Time
	In     []time.Time
	NotIn  []time.Time
	IsNull *bool
}
