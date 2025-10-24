# relay

`relay` is a library designed to simplify Relay-style pagination in Go applications, supporting both keyset-based and offset-based pagination. It helps developers efficiently implement pagination queries while offering optimization options, such as skipping `TotalCount` queries and encrypting cursors.

**Currently, only the `GORM` adapter is provided by default. For other adapters, refer to `gormrelay` to implement them yourself.**

## Features

- **Supports keyset-based and offset-based pagination**: You can freely choose high-performance keyset pagination based on multiple indexed columns, or use offset pagination.
- **Optional cursor encryption**: Supports encrypting cursors using `GCM(AES)` or `Base64` to ensure the security of pagination information.
- **Flexible query strategies**: Optionally skip the `TotalCount` query to improve performance, especially in large datasets.
- **Non-generic support**: Even without using Go generics, you can paginate using the `any` type for flexible use cases.
- **Computed fields**: Add database-level calculated fields using SQL expressions for sorting and pagination.
- **Powerful filtering**: Type-safe filtering with support for comparison operators, string matching, logical combinations, and relationship filtering.
- **gRPC/Protocol Buffers integration**: Built-in utilities for parsing proto messages, including enums, order fields, filters, and pagination requests.

## Usage

### Basic Usage

```go
p := relay.New(
    cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
        // Offset-based pagination
        // return gormrelay.NewOffsetAdapter[*User](db)(ctx, req)

        // Keyset-based pagination
        return gormrelay.NewKeysetAdapter[*User](db)(ctx, req)
    }),
    // defaultLimit / maxLimit
    relay.EnsureLimits[*User](10, 100),
    // Append primary sorting fields, if any are unspecified
    relay.EnsurePrimaryOrderBy[*User](
        relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
        relay.Order{Field: "Version", Direction: relay.OrderDirectionAsc},
    ),
)

conn, err := p.Paginate(
    context.Background(),
    // relay.WithSkip(context.Background(), relay.Skip{
    //     Edges:      true,
    //     Nodes:      true,
    //     PageInfo:   true,
    //     TotalCount: true,
    // }),

    // Query first 10 records
    &relay.PaginateRequest[*User]{
        First: lo.ToPtr(10),
    }
)
```

### Cursor Encryption

If you need to encrypt cursors, you can use `cursor.Base64` or `cursor.GCM` wrappers:

```go
// Encode cursors with Base64
cursor.Base64(gormrelay.NewOffsetAdapter[*User](db))

// Encrypt cursors with GCM(AES)
gcm, err := cursor.NewGCM(encryptionKey)
require.NoError(t, err)
cursor.GCM(gcm)(gormrelay.NewKeysetAdapter[*User](db))
```

### Non-Generic Usage

If you do not use generics, you can create a paginator with the `any` type and combine it with the `db.Model` method:

```go
p := relay.New(
    func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
        // Since this is a generic function (T: any), we must call db.Model(x)
        return gormrelay.NewKeysetAdapter[any](db.Model(&User{}))(ctx, req)
    },
    relay.EnsureLimits[any](10, 100),
    relay.EnsurePrimaryOrderBy[any](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
)
conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
    First: lo.ToPtr(10), // query first 10 records
})
```

## Computed Fields

`relay` supports computed fields, allowing you to add SQL expressions calculated at the database level and use them for sorting and pagination.

### Basic Usage

```go
import (
    "github.com/theplant/relay/gormrelay"
)

p := relay.New(
    gormrelay.NewKeysetAdapter[*User](
        db,
        gormrelay.WithComputed(&gormrelay.Computed[*User]{
            Columns: gormrelay.NewComputedColumns(map[string]string{
                "Priority": "CASE WHEN status = 'premium' THEN 1 WHEN status = 'vip' THEN 2 ELSE 3 END",
            }),
            Scanner: gormrelay.NewComputedScanner[*User],
        }),
    ),
    relay.EnsureLimits[*User](10, 100),
    relay.EnsurePrimaryOrderBy[*User](
        relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
    ),
)

// Use computed field in ordering
conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
    First: lo.ToPtr(10),
    OrderBy: []relay.Order{
        {Field: "Priority", Direction: relay.OrderDirectionAsc}, // Sort by computed field
        {Field: "ID", Direction: relay.OrderDirectionAsc},
    },
})
```

### Key Components

**NewComputedColumns**

Helper function to create computed column definitions from SQL expressions:

```go
gormrelay.NewComputedColumns(map[string]string{
    "FieldName": "SQL expression",
})
```

**NewComputedScanner**

Standard scanner function that handles result scanning and wrapping. This is the recommended implementation for most use cases:

```go
gormrelay.NewComputedScanner[*User]
```

**Custom Scanner**

For custom types or complex scenarios, implement your own Scanner function:

```go
type Shop struct {
    ID       int
    Name     string
    Priority int `gorm:"-"` // Computed field, not stored in DB
}

gormrelay.WithComputed(&gormrelay.Computed[*Shop]{
    Columns: gormrelay.NewComputedColumns(map[string]string{
        "Priority": "CASE WHEN name = 'premium' THEN 1 ELSE 2 END",
    }),
    Scanner: func(db *gorm.DB) (*gormrelay.ComputedScanner[*Shop], error) {
        shops := []*Shop{}
        return &gormrelay.ComputedScanner[*Shop]{
            Destination: &shops,
            Transform: func(computedResults []map[string]any) []cursor.Node[*Shop] {
                return lo.Map(shops, func(s *Shop, i int) cursor.Node[*Shop] {
                    // Populate computed field
                    s.Priority = int(computedResults[i]["Priority"].(int32))
                    return gormrelay.NewComputedNode(s, computedResults[i])
                })
            },
        }, nil
    },
})
```

### Complex Example

```go
p := relay.New(
    gormrelay.NewKeysetAdapter[*User](
        db,
        gormrelay.WithComputed(&gormrelay.Computed[*User]{
            Columns: gormrelay.NewComputedColumns(map[string]string{
                "Score": "(points * 10 + bonus)",
                "Rank":  "CASE WHEN score > 100 THEN 'A' WHEN score > 50 THEN 'B' ELSE 'C' END",
            }),
            Scanner: gormrelay.NewComputedScanner[*User],
        }),
    ),
    relay.EnsureLimits[*User](10, 100),
    relay.EnsurePrimaryOrderBy[*User](
        relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
    ),
)

// Multi-level sorting with computed fields
conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
    First: lo.ToPtr(10),
    OrderBy: []relay.Order{
        {Field: "Rank", Direction: relay.OrderDirectionAsc},
        {Field: "Score", Direction: relay.OrderDirectionDesc},
        {Field: "ID", Direction: relay.OrderDirectionAsc},
    },
})
```

### Notes

- Computed fields are calculated by the database, ensuring consistency and performance
- The computed values are automatically included in cursor serialization for pagination
- Field names in `NewComputedColumns` are converted to SQL aliases using `ComputedFieldToColumnAlias`
- Both keyset and offset pagination support computed fields

For more details on computed fields design and common questions, see [FAQ: Computed Fields](FAQ_COMPUTED.md).

## Filter Support

`relay` provides powerful type-safe filtering capabilities through the `filter` and `gormfilter` packages.

### Basic Filtering

```go
import (
    "github.com/theplant/relay/filter"
    "github.com/theplant/relay/filter/gormfilter"
)

type UserFilter struct {
    Name *filter.String
    Age  *filter.Int
}

db.Scopes(
    gormfilter.Scope(&UserFilter{
        Name: &filter.String{
            Contains: lo.ToPtr("john"),
            Fold:     true, // case-insensitive
        },
        Age: &filter.Int{
            Gte: lo.ToPtr(18),
        },
    }),
).Find(&users)
```

### Supported Operators

The filter package provides the following types and operators:

**String** (`filter.String` / `filter.ID`)

- `Eq`, `Neq`: Equal / Not equal
- `Lt`, `Lte`, `Gt`, `Gte`: Less than, Less than or equal, Greater than, Greater than or equal
- `In`, `NotIn`: In / Not in array
- `Contains`, `StartsWith`, `EndsWith`: String pattern matching
- `Fold`: Case-insensitive comparison (works with all string operators)
- `IsNull`: Null check

**Numeric** (`filter.Int` / `filter.Float`)

- `Eq`, `Neq`, `Lt`, `Lte`, `Gt`, `Gte`: Comparison operators
- `In`, `NotIn`: In / Not in array
- `IsNull`: Null check

**Boolean** (`filter.Boolean`)

- `Eq`, `Neq`: Equal / Not equal
- `IsNull`: Null check

**Time** (`filter.Time`)

- `Eq`, `Neq`, `Lt`, `Lte`, `Gt`, `Gte`: Time comparison
- `In`, `NotIn`: In / Not in array
- `IsNull`: Null check

### Logical Combinations

Filters support `And`, `Or`, and `Not` logical operators:

```go
type UserFilter struct {
    And  []*UserFilter
    Or   []*UserFilter
    Not  *UserFilter
    Name *filter.String
    Age  *filter.Int
}

// Complex filter example
db.Scopes(
    gormfilter.Scope(&UserFilter{
        Or: []*UserFilter{
            {
                Name: &filter.String{
                    StartsWith: lo.ToPtr("J"),
                    Fold:       true,
                },
            },
            {
                Age: &filter.Int{
                    Gt: lo.ToPtr(30),
                },
            },
        },
    }),
).Find(&users)
```

### Relationship Filtering

The filter supports filtering by `BelongsTo/HasOne` relationships with multi-level nesting:

```go
type CountryFilter struct {
    Code *filter.String
    Name *filter.String
}

type CompanyFilter struct {
    Name    *filter.String
    Country *CountryFilter  // BelongsTo relationship
}

type UserFilter struct {
    Age     *filter.Int
    Company *CompanyFilter  // BelongsTo relationship
}

// Filter users by company's country
db.Scopes(
    gormfilter.Scope(&UserFilter{
        Age: &filter.Int{
            Gte: lo.ToPtr(21),
        },
        Company: &CompanyFilter{
            Name: &filter.String{
                Contains: lo.ToPtr("Tech"),
            },
            Country: &CountryFilter{
                Code: &filter.String{
                    Eq: lo.ToPtr("US"),
                },
                Name: &filter.String{
                    Eq: lo.ToPtr("United States"),
                },
            },
        },
    }),
).Find(&users)
```

### Combining with Paginator

Filter and paginator can work together seamlessly:

```go
import (
    "github.com/theplant/relay"
    "github.com/theplant/relay/cursor"
    "github.com/theplant/relay/filter"
    "github.com/theplant/relay/filter/gormfilter"
    "github.com/theplant/relay/gormrelay"
)

type UserFilter struct {
    Name    *filter.String
    Age     *filter.Int
    Company *CompanyFilter
}

// Create paginator with filter
p := relay.New(
    cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
        return gormrelay.NewKeysetAdapter[*User](
            db.WithContext(ctx).Scopes(gormfilter.Scope(&UserFilter{
                Age: &filter.Int{
                    Gte: lo.ToPtr(18),
                },
                Company: &CompanyFilter{
                    Name: &filter.String{
                        Contains: lo.ToPtr("Tech"),
                        Fold:     true,
                    },
                },
            })),
        )(ctx, req)
    }),
    relay.EnsureLimits[*User](10, 100),
    relay.EnsurePrimaryOrderBy[*User](
        relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
    ),
)

conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
    First: lo.ToPtr(10),
})
```

### Filter Options

**Disable Relationship Filtering:**

```go
db.Scopes(
    gormfilter.Scope(
        userFilter,
        gormfilter.WithDisableBelongsTo(),
        gormfilter.WithDisableHasOne(),
        // gormfilter.WithDisableRelationships(), // disable all relationships
    ),
).Find(&users)
```

### Performance Considerations

Relationship filters use `IN` subqueries, which are generally efficient for most use cases. Performance depends on:

- Database indexes on foreign keys
- Size of result sets
- Query complexity

For detailed performance analysis comparing `IN` subqueries with `JOIN` approaches, see `filter/gormfilter/perf/perf_test.go`.

## gRPC Integration

`relay` provides seamless integration with gRPC/Protocol Buffers, including utilities for parsing proto enums, order fields, filters, and pagination requests.

### Protocol Buffers Definition

For a complete example of proto definitions with pagination, ordering, and filtering support, see:

- Buf configuration: [`protorelay/testdata/buf.yaml`](protorelay/testdata/buf.yaml)
- Buf generation config: [`protorelay/testdata/buf.gen.yaml`](protorelay/testdata/buf.gen.yaml)
- Proto definitions: [`protorelay/testdata/proto/testdata/v1/product.proto`](protorelay/testdata/proto/testdata/v1/product.proto)
- Relay pagination types: [`protorelay/proto/relay/v1/relay.proto`](protorelay/proto/relay/v1/relay.proto)

### Implementation Example

For a complete implementation of a gRPC service using `relay`, refer to the `ProductService.ListProducts` method:

- Implementation: [`protorelay/proto_test.go` (ProductService.ListProducts)](protorelay/proto_test.go)

This example demonstrates:

- Parsing proto order fields with `protorelay.ParseOrderBy`
- Parsing proto filters with `protofilter.ToMap`
- Creating a paginator with Base64-encoded cursors
- Converting between proto and internal types with `protorelay.ParsePagination`
- Building gRPC responses from pagination results

## Reference

- [FAQ: Computed Fields](FAQ_COMPUTED.md) - Detailed guide on computed fields design and common questions
- [GraphQL Connections](https://relay.dev/graphql/connections.htm) - Relay-style pagination specification
