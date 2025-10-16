// Package filter provides type-safe filter types for building query conditions.
// These types are used with gormfilter to create powerful and flexible database queries.
//
// Each filter type supports various operators appropriate to its data type:
//   - String/ID: Eq, Neq, Lt, Lte, Gt, Gte, In, NotIn, Contains, StartsWith, EndsWith, IsNull, Fold
//   - Int/Float: Eq, Neq, Lt, Lte, Gt, Gte, In, NotIn, IsNull
//   - Boolean: Eq, Neq, IsNull
//   - Time: Eq, Neq, Lt, Lte, Gt, Gte, In, NotIn, IsNull
//
// The Fold field in String filters enables case-insensitive comparison.
package filter

import "time"

// String provides filtering operations for string fields.
// All comparison operations can be case-insensitive when Fold is set to true.
type String struct {
	Eq         *string  `json:"eq"`
	Neq        *string  `json:"neq"`
	Lt         *string  `json:"lt"`
	Lte        *string  `json:"lte"`
	Gt         *string  `json:"gt"`
	Gte        *string  `json:"gte"`
	In         []string `json:"in"`
	NotIn      []string `json:"notIn"`
	IsNull     *bool    `json:"isNull"`
	Contains   *string  `json:"contains"`
	StartsWith *string  `json:"startsWith"`
	EndsWith   *string  `json:"endsWith"`
	Fold       bool     `json:"fold"`
}

// ID is an alias for String, used for identifier fields.
type ID String

// Float provides filtering operations for float64 fields.
type Float struct {
	Eq     *float64  `json:"eq"`
	Neq    *float64  `json:"neq"`
	Lt     *float64  `json:"lt"`
	Lte    *float64  `json:"lte"`
	Gt     *float64  `json:"gt"`
	Gte    *float64  `json:"gte"`
	In     []float64 `json:"in"`
	NotIn  []float64 `json:"notIn"`
	IsNull *bool     `json:"isNull"`
}

// Int provides filtering operations for integer fields.
type Int struct {
	Eq     *int  `json:"eq"`
	Neq    *int  `json:"neq"`
	Lt     *int  `json:"lt"`
	Lte    *int  `json:"lte"`
	Gt     *int  `json:"gt"`
	Gte    *int  `json:"gte"`
	In     []int `json:"in"`
	NotIn  []int `json:"notIn"`
	IsNull *bool `json:"isNull"`
}

// Boolean provides filtering operations for boolean fields.
type Boolean struct {
	Eq     *bool `json:"eq"`
	Neq    *bool `json:"neq"`
	IsNull *bool `json:"isNull"`
}

// Time provides filtering operations for time.Time fields.
type Time struct {
	Eq     *time.Time  `json:"eq"`
	Neq    *time.Time  `json:"neq"`
	Lt     *time.Time  `json:"lt"`
	Lte    *time.Time  `json:"lte"`
	Gt     *time.Time  `json:"gt"`
	Gte    *time.Time  `json:"gte"`
	In     []time.Time `json:"in"`
	NotIn  []time.Time `json:"notIn"`
	IsNull *bool       `json:"isNull"`
}
