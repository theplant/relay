# relay

`relay` is a library designed to simplify Relay-style pagination in Go applications, supporting both keyset-based and offset-based pagination. It helps developers efficiently implement pagination queries while offering optimization options, such as skipping `TotalCount` queries and encrypting cursors.

**Currently, only the `GORM` adapter is provided by default. For other adapters, refer to `gormrelay` to implement them yourself.**

## Features

- **Supports keyset-based and offset-based pagination**: You can freely choose high-performance keyset pagination based on multiple indexed columns, or use offset pagination.
- **Optional cursor encryption**: Supports encrypting cursors using `GCM(AES)` or `Base64` to ensure the security of pagination information.
- **Flexible query strategies**: Optionally skip the `TotalCount` query to improve performance, especially in large datasets.
- **Non-generic support**: Even without using Go generics, you can paginate using the `any` type for flexible use cases.

## Usage

### Basic Usage

```go
p := relay.New(
    true, // nodesOnly, true means only nodes are returned, otherwise only edges are returned
    10, 10, // maxLimit / limitIfNotSet
    func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
        // Offset-based pagination
        // return gormrelay.NewOffsetAdapter[*User](db)(ctx, req)
        // Keyset-based pagination
        return gormrelay.NewKeysetAdapter[*User](db)(ctx, req)
    },
)
resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
    First: lo.ToPtr(10), // query first 10 records
})
```

### Middleware

If you need to encrypt cursors, you can use `cursor.Base64` or `cursor.GCM` middlewares:

```go
// Encrypt cursors with Base64
cursor.Base64(gormrelay.NewOffsetAdapter[*User](db))

// Encrypt cursors with GCM(AES)
gcm, err := cursor.NewGCM(encryptionKey)
require.NoError(t, err)
cursor.GCM(gcm)(gormrelay.NewKeysetAdapter[*User](db))
```

If you need to append `PrimaryOrderBys` to `PaginateRequest.OrderBys`

```go
// without middleware
req.OrderBys = relay.AppendPrimaryOrderBy[*User](req.OrderBys, 
    relay.OrderBy{Field: "ID", Desc: false},
    relay.OrderBy{Field: "Version", Desc: false},
)

// use cursor middleware
cursor.PrimaryOrderBy[*User](
    relay.OrderBy{Field: "ID", Desc: true},
    relay.OrderBy{Field: "Version", Desc: false},
)(
    gormrelay.NewKeysetAdapter[*User](db),
)

// use pagination middleware
relay.PrimaryOrderBy[*User](
    relay.OrderBy{Field: "ID", Desc: false},
    relay.OrderBy{Field: "Version", Desc: false},
)(p)
```

### Skipping `TotalCount` Query for Optimization

To improve performance, you can skip querying `TotalCount`, especially useful for large datasets:

```go
// Keyset-based pagination without querying TotalCount
// Note: The final PageInfo.TotalCount will be relay.InvalidTotalCount(-1) // TODO:
cursor.NewKeysetAdapter(gormrelay.NewKeysetFinder[any](db))

// Offset-based pagination without querying TotalCount
// Note: The final PageInfo.TotalCount will be relay.InvalidTotalCount(-1)
// Note: Using `Last != nil && Before == nil` is not supported for this case.
cursor.NewOffsetAdapter(gormrelay.NewOffsetFinder[any](db))

// Compared to the version that queries TotalCount

cursor.NewKeysetAdapter(gormrelay.NewKeysetCounter[any](db))
// equals
gormrelay.NewKeysetAdapter[any](db)

cursor.NewOffsetAdapter(gormrelay.NewOffsetCounter[any](db))
// equals
gormrelay.NewOffsetAdapter[any](db)
```

### Non-Generic Usage

If you do not use generics, you can create a paginator with the `any` type and combine it with the `db.Model` method:

```go
p := relay.New(
    false, // nodesOnly
    10, 10,
    func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
        // Since this is a generic function (T: any), we must call db.Model(x)
        return gormrelay.NewKeysetAdapter[any](db.Model(&User{}))(ctx, req)
    },
)
resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
    First: lo.ToPtr(10), // query first 10 records
})
```

## Reference

For more information about Relay-style pagination, refer to [GraphQL Connections](https://relay.dev/graphql/connections.htm).
