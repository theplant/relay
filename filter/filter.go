package filter

import "time"

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

type ID String

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

type Boolean struct {
	Eq     *bool `json:"eq"`
	Neq    *bool `json:"neq"`
	IsNull *bool `json:"isNull"`
}

type Time struct {
	Eq     *time.Time   `json:"eq"`
	Neq    *time.Time   `json:"neq"`
	Lt     *time.Time   `json:"lt"`
	Lte    *time.Time   `json:"lte"`
	Gt     *time.Time   `json:"gt"`
	Gte    *time.Time   `json:"gte"`
	In     []*time.Time `json:"in"`
	NotIn  []*time.Time `json:"notIn"`
	IsNull *bool        `json:"isNull"`
}
