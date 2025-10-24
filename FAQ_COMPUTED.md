# Frequently Asked Questions (FAQ)

## Computed Fields

### Why does relay use SplitScan for computed fields?

Computed fields are database-calculated values (e.g., `(CASE WHEN status = 'premium' THEN 1 ELSE 3 END)`) that are often used for sorting but don't necessarily belong in your domain model.

#### The Core Challenge

**The fundamental requirement:** When using keyset pagination with computed fields in `OrderBy`, those computed values **must** be included in the cursor for pagination to work correctly.

Consider this scenario:

```go
type Shop struct {
    ID   int
    Name string
    // Note: No Priority field in the domain model
}

conn, _ := paginator.Paginate(&relay.PaginateRequest[*Shop]{
    OrderBy: []relay.Order{
        {Field: "Priority", Direction: relay.OrderDirectionAsc},  // Computed field!
        {Field: "ID", Direction: relay.OrderDirectionAsc},
    },
})
```

For keyset pagination to work, the cursor must contain both `Priority` and `ID` values. However, `Priority` is a query-time computation, not a persistent attribute of `Shop`.

#### Design Goals

1. **Keep domain models pure**: `Shop` should only contain business data, not query-specific computed values
2. **Enable cursor encoding**: Computed fields must be available during cursor serialization
3. **Maintain type consistency**: Generic type should remain `relay.New[*Shop]`, not change based on query requirements
4. **Runtime flexibility**: Different queries can compute different fields dynamically

#### The Solution: SplitScan + Dynamic JSON Injection

**Architecture:**

```
┌─────────────────────────────────────────────────────┐
│ SQL Query                                           │
│ SELECT id, name, (CASE...) AS _relay_computed_priority│
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ SplitScan                                           │
│  • Regular columns → Shop struct                    │
│  • _relay_computed_* → computedResults map          │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ NodeWrapper[*Shop]                                  │
│  • RelayNode() returns *Shop (type consistency ✓)  │
│  • MarshalJSON() injects computedResults (cursor ✓)│
└─────────────────────────────────────────────────────┘
```

**Implementation:**

```go
gormrelay.WithComputed(&Computed[*Shop]{
    Columns: NewComputedColumns(map[string]string{
        "Priority": "(CASE WHEN name = 'premium' THEN 1 ELSE 3 END)",
    }),
    Scanner: NewComputedScanner[*Shop],
})
```

**How It Works:**

**1. SQL Generation**

```sql
SELECT shops.id, shops.name,
       (CASE WHEN name = 'premium' THEN 1 ELSE 3 END) AS _relay_computed_priority
FROM shops
ORDER BY _relay_computed_priority, shops.id
```

**2. SplitScan Separation**

```go
// Regular columns scanned into Shop struct
shop := &Shop{ID: 1, Name: "premium"}

// Computed columns extracted separately
computedResults := map[string]any{"Priority": 1}
```

**3. Dynamic JSON Injection**

```go
type withComputedResult struct {
    Object          any            // *Shop
    ComputedResults map[string]any // {"Priority": 1}
}

func (v *withComputedResult) MarshalJSON() ([]byte, error) {
    b, _ := cursor.JSONMarshal(v.Object)  // {"ID": 1, "Name": "premium"}

    // Inject computed fields
    for field, value := range v.ComputedResults {
        b, _ = sjson.SetBytes(b, field, value)
    }
    // Result: {"ID": 1, "Name": "premium", "Priority": 1}
    return b, nil
}
```

**4. Cursor Encoding**

```go
// Create a computed node (this wraps shop with computed results)
node := gormrelay.NewComputedNode(shop, computedResults)

// Internally, this creates:
// &cursor.NodeWrapper[*Shop]{
//     Object: WithComputedResult(shop, computedResults),
//     Unwrap: func() *Shop { return shop },
// }

// node.RelayNode() → returns *Shop (type: *Shop ✓)
// node.MarshalJSON() → includes Priority (cursor: {"ID": 1, "Priority": 1} ✓)
```

#### Why This Design is Necessary

**The Causal Chain:**

```
Business Requirement
    ↓
Order by computed field
    ↓
Keyset pagination requirement
    ↓
Cursor must contain computed field values
    ↓
Design Constraint
    ↓
Computed fields needed for cursor, not for domain model
    ↓
Solution
    ↓
Extract computed values separately
    ↓
Inject only during JSON serialization
    ↓
SplitScan is the mechanism to extract → Dynamic injection is the mechanism to provide
```

**Benefits:**

- ✅ **Type consistency**: Always `relay.New[*Shop]`, never changes based on query
- ✅ **Clean domain model**: `Shop` contains only business data
- ✅ **Cursor encoding works**: Computed fields dynamically injected into JSON
- ✅ **Runtime flexibility**: Can change computed columns based on permissions/context
- ✅ **Logical cohesion**: All computed field logic centralized in `Computed` config

### Why do computed columns need the `_relay_computed_` prefix?

The unique prefix serves multiple critical purposes:

#### 1. Marking Columns for SplitScan Interception

The prefix acts as a marker to distinguish which columns should be intercepted by SplitScan:

```go
// gormrelay/scan.go
for i, columnType := range w.columnTypes {
    colName := columnType.Name()
    if factory, exists := w.splitter[colName]; exists {
        // This column is intercepted and routed to splitResults
    } else {
        // Regular column, scanned into destination struct
    }
}
```

**Example:**

```sql
SELECT id, name,
       priority,                           -- Regular column → scanned into struct
       (CASE...) as _relay_computed_score  -- Marked column → intercepted by SplitScan
FROM shops
```

Without the prefix, SplitScan wouldn't know which columns to intercept.

#### 2. Avoiding Column Name Conflicts

**Problem: Ambiguous Column Names**

Go's `sql.ColumnType` only provides column names, not positions:

```go
type ColumnType struct {
    name string  // Only the name, no position index
}
```

If a computed column has the same name as a table column:

```sql
-- Without unique prefix
SELECT id, name, priority, (CASE...) as priority FROM shops
--                  ↑                            ↑
--            Table column              Computed column
```

Both columns would have `columnType.Name() == "priority"`, making it impossible to distinguish which should be intercepted.

#### 3. GORM Column Mapping Isolation

GORM maps SQL columns to struct fields based on names and tags:

```go
type Shop struct {
    ID       int    `gorm:"column:id"`
    Name     string `gorm:"column:name"`
    Priority int    `gorm:"column:priority"`
}
```

If a computed column is named `priority`:

- GORM tries to map it to `Shop.Priority`
- But SplitScan also wants to intercept it
- Conflict occurs, behavior becomes unpredictable

With `_relay_computed_priority`:

- GORM sees no matching struct field → ignores it
- SplitScan recognizes the prefix → intercepts it
- Clean separation ✓

#### 4. Database SQL Standard Compliance

Most databases don't allow duplicate column names:

```sql
-- PostgreSQL error
SELECT id, priority, priority as priority FROM shops;
-- Error: column "priority" specified more than once
```

The unique prefix ensures SQL compliance across all databases.

**Summary:**

The `_relay_computed_` prefix creates a clear separation where:

- Computed columns are uniquely identifiable
- SplitScan can intercept them without ambiguity
- GORM won't interfere with them
- SQL remains valid and unambiguous

### Can I add non-computed columns to my query?

Yes! You can use `AppendSelect` to add regular columns alongside computed fields:

```go
type ShopWithExtra struct {
    *Shop
    ExtraInfo string `gorm:"column:extra_info"`
}

gormrelay.NewKeysetAdapter[*Shop](
    db.Scopes(
        AppendSelect(clause.Column{
            Name:  "'additional data'",
            Alias: "extra_info",
            Raw:   true,
        }),
    ),
    gormrelay.WithComputed(&Computed[*Shop]{
        Columns: NewComputedColumns(map[string]string{
            "Priority": "(CASE...)",
        }),
        Scanner: func(_ *gorm.DB) (*ComputedScanner[*Shop], error) {
            nodes := []*ShopWithExtra{}
            return &ComputedScanner[*Shop]{
                Destination: &nodes,
                Transform: func(computedResults []map[string]any) []cursor.Node[*Shop] {
                    return lo.Map(nodes, func(s *ShopWithExtra, i int) cursor.Node[*Shop] {
                        // ExtraInfo is populated by GORM directly
                        // Priority is in computedResults
                        return NewComputedNode(s.Shop, computedResults[i])
                    })
                },
            }, nil
        },
    }),
)
```

**How this works:**

```sql
SELECT shops.*,
       'additional data' as extra_info,           -- Regular column → GORM scans to ExtraInfo
       (CASE...) as _relay_computed_priority      -- Computed field → SplitScan intercepts
FROM shops
```

- `extra_info` (no prefix) → GORM scans into `ShopWithExtra.ExtraInfo`
- `_relay_computed_priority` (with prefix) → SplitScan intercepts into `computedResults`

### Can I use computed fields without ordering by them?

**Yes, you can define and query computed fields without using them in `OrderBy`.** However, they will be filtered out from the cursor encoding and won't appear in the final response by default.

**Why are they filtered out?** The `EncodeKeysetCursor` function only includes fields specified in `OrderBy`:

```go
// cursor/keyset.go
keysMap := lo.SliceToMap(keys, func(key string) (string, bool) {
    return key, true
})
for k := range m {
    if _, ok := keysMap[k]; !ok {
        delete(m, k)  // Remove fields not in OrderBy
    }
}
```

Even though the computed field is queried and extracted into `computedResults`, it gets filtered out during cursor encoding if it's not in the `OrderBy` list.

**How to include them in the response?** If you need computed fields for display purposes (not ordering), extract them from `computedResults` in your `ComputedScanner.Transform` and assign them to your model:

```go
type Product struct {
    ID              int     `json:"id"`
    Name            string  `json:"name"`
    Price           float64 `json:"price"`
    DiscountedPrice float64 `gorm:"-" json:"discountedPrice"`  // Will be populated from computedResults
}

gormrelay.WithComputed(&Computed[*Product]{
    Columns: NewComputedColumns(map[string]string{
        "DiscountedPrice": "(price * (1 - discount_rate))",
        "Priority":        "(CASE...)",  // Used for ordering
    }),
    Scanner: func(_ *gorm.DB) (*ComputedScanner[*Product], error) {
        nodes := []*Product{}
        return &ComputedScanner[*Product]{
            Destination: &nodes,
            Transform: func(computedResults []map[string]any) []cursor.Node[*Product] {
                return lo.Map(nodes, func(p *Product, i int) cursor.Node[*Product] {
                    // Extract DiscountedPrice from computedResults and assign to Product
                    if discounted, ok := computedResults[i]["DiscountedPrice"].(float64); ok {
                        p.DiscountedPrice = discounted
                    }

                    return NewComputedNode(p, computedResults[i])
                })
            },
        }, nil
    },
})

OrderBy: []relay.Order{
    {Field: "Priority", Direction: relay.OrderDirectionAsc},
    {Field: "ID", Direction: relay.OrderDirectionAsc},
}
```

**Result:**

- `Priority` appears in cursor (via `WithComputedResult` and is in OrderBy)
- `DiscountedPrice` appears in response JSON (via direct assignment to `p.DiscountedPrice`, filtered out from cursor)

**Alternative approach:** Use `AppendSelect` with regular column aliases (without `_relay_computed_` prefix) so GORM scans them directly into your struct fields, bypassing SplitScan entirely.

### How do I debug computed field issues?

1. **Check SQL generation**: Enable GORM's logger to see the actual SQL

   ```go
   db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Info)})
   ```

2. **Verify column aliases**: Ensure computed columns use `_relay_computed_` prefix in the SQL output

3. **Test cursor encoding**: Check if `OrderBy` fields exist in the serialized node

   ```go
   cursor, err := cursor.EncodeKeysetCursor(node, []string{"Priority"})
   // Error: "required key Priority not found" → computed field not injected
   ```

4. **Validate Transform function**: Ensure `computedResults` are properly applied
   ```go
   Transform: func(computedResults []map[string]any) []cursor.Node[*Shop] {
       // Add debug logging
       fmt.Printf("computedResults: %+v\n", computedResults)
       // ...
   }
   ```

### What are the limitations of computed fields?

1. **Database-specific**: SQL expressions must be compatible with your database
2. **No ORM validation**: Computed field expressions are passed directly to SQL
3. **Type safety**: Type conversion between SQL and Go must be handled manually
4. **Performance**: Complex computed expressions may impact query performance
