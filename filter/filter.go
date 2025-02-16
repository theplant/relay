package filter

import "time"

type String struct {
	Equals     *string  `json:"equals"`
	Not        *string  `json:"not"`
	In         []string `json:"in"`
	NotIn      []string `json:"notIn"`
	Lt         *string  `json:"lt"`
	Lte        *string  `json:"lte"`
	Gt         *string  `json:"gt"`
	Gte        *string  `json:"gte"`
	Contains   *string  `json:"contains"`
	StartsWith *string  `json:"startsWith"`
	EndsWith   *string  `json:"endsWith"`
	IsNull     *bool    `json:"isNull"`
	Fold       bool     `json:"fold"`
}

type ID String

type Float struct {
	Equals *float64  `json:"equals"`
	Not    *float64  `json:"not"`
	In     []float64 `json:"in"`
	NotIn  []float64 `json:"notIn"`
	Lt     *float64  `json:"lt"`
	Lte    *float64  `json:"lte"`
	Gt     *float64  `json:"gt"`
	Gte    *float64  `json:"gte"`
	IsNull *bool     `json:"isNull"`
}

type Int struct {
	Equals *int  `json:"equals"`
	Not    *int  `json:"not"`
	In     []int `json:"in"`
	NotIn  []int `json:"notIn"`
	Lt     *int  `json:"lt"`
	Lte    *int  `json:"lte"`
	Gt     *int  `json:"gt"`
	Gte    *int  `json:"gte"`
	IsNull *bool `json:"isNull"`
}

type Boolean struct {
	Equals *bool `json:"equals"`
	Not    *bool `json:"not"`
	IsNull *bool `json:"isNull"`
}

type Time struct {
	Equals *time.Time   `json:"equals"`
	Not    *time.Time   `json:"not"`
	In     []*time.Time `json:"in"`
	NotIn  []*time.Time `json:"notIn"`
	Lt     *time.Time   `json:"lt"`
	Lte    *time.Time   `json:"lte"`
	Gt     *time.Time   `json:"gt"`
	Gte    *time.Time   `json:"gte"`
	IsNull *bool        `json:"isNull"`
}
