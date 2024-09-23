# gorelay

`gorelay` is a library designed to simplify Relay-style pagination in Go applications, supporting both cursor-based and offset-based pagination. It helps developers efficiently implement GraphQL pagination queries while offering optimization options, such as skipping `TotalCount` queries and encrypting cursors.

**Currently, only the `GORM` adapter is provided by default. For other adapters, refer to `gormrelay` to implement them yourself.**

## Features

- **Supports cursor-based and offset-based pagination**: You can freely choose high-performance cursor pagination based on multiple indexed columns, or use offset pagination.
- **Optional cursor encryption**: Supports encrypting cursors using AES or Base64 to ensure the security of pagination information.
- **Flexible query strategies**: Optionally skip the `TotalCount` query to improve performance, especially in large datasets.
- **Non-generic support**: Even without using Go generics, you can paginate using the `any` type for flexible use cases.

## Usage

### Basic Usage

```go
p := relay.New(
    true, // nodesOnly, default returns nodes and pageInfo
    10, 10, // maxLimit / limitIfNotSet
    []relay.OrderBy{
        {Field: "ID", Desc: false}, // default order if not provided
    },
    func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
        // Offset-based pagination
        // return gormrelay.NewOffsetAdapter[*User](db)(ctx, req)
        // Cursor-based pagination
        return gormrelay.NewKeysetAdapter[*User](db)(ctx, req)
    },
)
resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
    First: lo.ToPtr(10), // query first 10 records
})
```

### Encrypting Cursors

If you need to encrypt cursors, you can use WrapBase64 or WrapAES wrappers:

```go
// Encrypt cursors with Base64
cursor.WrapBase64(gormrelay.NewOffsetAdapter[*User](db))

// Encrypt cursors with AES
cursor.WrapAES(gormrelay.NewKeysetAdapter[*User](db), encryptionKey)
```

### Skipping `TotalCount` Query for Optimization

To improve performance, you can skip querying TotalCount, especially useful for large datasets:

```go
// Cursor-based pagination without querying TotalCount
cursor.NewKeysetAdapter(gormrelay.NewKeysetFinder[any](db))

// Note: For offset-based pagination, if you can't query TotalCount, 
// using `Last != nil && Before == nil` is not possible.
cursor.NewOffsetAdapter(gormrelay.NewOffsetFinder[any](db))
```

### Non-Generic Usage

If you do not use generics, you can create a paginator with the `any` type and combine it with the `db.Model` method:

```go
p := relay.New(
    false, // nodesOnly
    10, 10,
    []relay.OrderBy{
        {Field: "ID", Desc: false},
    },
    func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
        // Since this is a generic function (T: any), we must call db.Model(x)
        return gormrelay.NewKeysetAdapter[*User](db.Model(&User{}))(ctx, req)
    },
)
resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
    First: lo.ToPtr(10), // query first 10 records
})
```

## Reference

For more information about Relay-style pagination, refer to [GraphQL Connections](https://relay.dev/graphql/connections.htm).
