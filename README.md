# gorelay

## Usage

```go
p := relay.New(
    true, // nodesOnly
    10, 10, // maxLimit / limitIfNotSet
    []relay.OrderBy{
        {Field: "ID", Desc: false},
    }, // orderBysIfNotSet
    func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
        // return gormrelay.NewOffsetAdapter[*User](db)(ctx, req) // offset-based
        return gormrelay.NewKeysetAdapter[*User](db)(ctx, req) // cursor-based
    }, // applyCursorsFunc
)
resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
    First: lo.ToPtr(10),
})
```

If you need to encrypt the cursor

```go
cursor.WrapBase64(gormrelay.NewOffsetAdapter[*User](db))
cursor.WrapAES(gormrelay.NewKeysetAdapter[*User](db), encryptionKey)
```

If you do not want to query `TotalCount` to improve query performance

```go
cursor.NewKeysetAdapter(gormrelay.NewKeysetFinder[any](db))

// !!! Note: For offset-based pagination, it is not possible to use `Last != nil && Before == nil` if cant query `TotalCount`
cursor.NewOffsetAdapter(gormrelay.NewOffsetFinder[any](db))
```

If you do not want to use generics, you can create Pagination with the `any` type and use it with the `db.Model` method.

```go
p := relay.New(
    false,
    10, 10,
    []relay.OrderBy{
        {Field: "ID", Desc: false},
    }, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
        // This is a generic(T: any) function, so we must to call db.Model(x)
        return gormrelay.NewKeysetAdapter[*User](db.Model(&User{}))(ctx, req)
    },
)
resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
    First: lo.ToPtr(10),
})
```

## Reference

[GraphQL Connections](https://relay.dev/graphql/connections.htm)
