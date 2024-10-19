package gormrelay

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
	"gorm.io/gorm"
)

func mustEncodeKeysetCursor[T any](node T, keys []string) string {
	cursor, err := cursor.EncodeKeysetCursor(node, keys)
	if err != nil {
		panic(err)
	}
	return cursor
}

func TestScopeKeyset(t *testing.T) {
	{
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			tx = tx.Model(&User{}).Scopes(scopeKeyset(
				&map[string]interface{}{"Age": 85},
				nil,
				[]relay.OrderBy{
					{Field: "Age", Desc: false},
				},
				10,
				false,
			)).Find(&User{})
			require.NoError(t, tx.Error)
			return tx
		})
		require.Equal(t, `SELECT * FROM "users" WHERE "users"."age" > 85 ORDER BY "users"."age" LIMIT 10`, sql)
	}
	{
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			// with table alias
			tx = tx.Table("company_users AS u").Model(&User{}).Scopes(scopeKeyset(
				&map[string]interface{}{"Age": 85},
				nil,
				[]relay.OrderBy{
					{Field: "Age", Desc: false},
				},
				10,
				false,
			)).Find(&User{})
			require.NoError(t, tx.Error)
			return tx
		})
		require.Equal(t, `SELECT * FROM company_users AS u WHERE "u"."age" > 85 ORDER BY "u"."age" LIMIT 10`, sql)
	}
	{
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			tx = tx.Model(&User{}).Scopes(scopeKeyset(
				&map[string]interface{}{"Age": 85},
				&map[string]interface{}{"Age": 88},
				[]relay.OrderBy{
					{Field: "Age", Desc: false},
				},
				10,
				false,
			)).Find(&User{})
			require.NoError(t, tx.Error)
			return tx
		})
		require.Equal(t, `SELECT * FROM "users" WHERE "users"."age" > 85 AND "users"."age" < 88 ORDER BY "users"."age" LIMIT 10`, sql)
	}
	{
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			tx = tx.Model(&User{}).Scopes(scopeKeyset(
				&map[string]interface{}{"Age": 85, "Name": "name15"},
				&map[string]interface{}{"Age": 88, "Name": "name12"},
				[]relay.OrderBy{
					{Field: "Age", Desc: false},
					{Field: "Name", Desc: true},
				},
				10,
				false,
			)).Find(&User{})
			require.NoError(t, tx.Error)
			return tx
		})
		require.Equal(t, `SELECT * FROM "users" WHERE ("users"."age" > 85 OR ("users"."age" = 85 AND "users"."name" < 'name15')) AND ("users"."age" < 88 OR ("users"."age" = 88 AND "users"."name" > 'name12')) ORDER BY "users"."age","users"."name" DESC LIMIT 10`, sql)
	}
	{
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			tx = tx.Model(&User{}).Scopes(scopeKeyset(
				&map[string]interface{}{"Age": 85, "Name": "name15"},
				&map[string]interface{}{"Age": 88, "Name": "name12"},
				[]relay.OrderBy{
					{Field: "Age", Desc: false},
					{Field: "Name", Desc: true},
				},
				10,
				true, // from last
			)).Find(&User{})
			require.NoError(t, tx.Error)
			return tx
		})
		require.Equal(t, `SELECT * FROM "users" WHERE ("users"."age" > 85 OR ("users"."age" = 85 AND "users"."name" < 'name15')) AND ("users"."age" < 88 OR ("users"."age" = 88 AND "users"."name" > 'name12')) ORDER BY "users"."age" DESC,"users"."name" LIMIT 10`, sql)
	}
	{
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			// with extra where
			tx = tx.Model(&User{}).Where("name LIKE ?", "name%").
				Scopes(scopeKeyset(
					&map[string]interface{}{"Age": 85, "Name": "name15"},
					&map[string]interface{}{"Age": 88, "Name": "name12"},
					[]relay.OrderBy{
						{Field: "Age", Desc: false},
						{Field: "Name", Desc: true},
					},
					10,
					false,
				)).Find(&User{})
			require.NoError(t, tx.Error)
			return tx
		})
		require.Equal(t, `SELECT * FROM "users" WHERE name LIKE 'name%' AND (("users"."age" > 85 OR ("users"."age" = 85 AND "users"."name" < 'name15')) AND ("users"."age" < 88 OR ("users"."age" = 88 AND "users"."name" > 'name12'))) ORDER BY "users"."age","users"."name" DESC LIMIT 10`, sql)
	}
}

func TestKeysetCursor(t *testing.T) {
	resetDB(t)

	applyCursorsFunc := NewKeysetAdapter[*User](db)

	primaryOrderByKeys := []string{"ID", "Age"}
	otherMiddlewares := []relay.PaginationMiddleware[*User]{
		relay.EnsurePrimaryOrderBy[*User](
			relay.OrderBy{Field: "ID", Desc: false},
			relay.OrderBy{Field: "Age", Desc: true},
		),
	}

	testCases := []struct {
		name               string
		limitIfNotSet      int
		maxLimit           int
		applyCursorsFunc   relay.ApplyCursorsFunc[*User]
		paginateRequest    *relay.PaginateRequest[*User]
		expectedEdgesLen   int
		expectedTotalCount *int
		expectedPageInfo   *relay.PageInfo
		expectedError      string
		expectedPanic      string
	}{
		{
			name:             "Invalid: Both First and Last",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(5),
				Last:  lo.ToPtr(5),
			},
			expectedError: "first and last cannot be used together",
		},
		{
			name:             "Invalid: Negative First",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(-5),
			},
			expectedError: "first must be a non-negative integer",
		},
		{
			name:             "Invalid: Negative Last",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(-5),
			},
			expectedError: "last must be a non-negative integer",
		},
		{
			name:             "Invalid: No limitIfNotSet",
			limitIfNotSet:    0, // Assuming 0 indicates not set
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedPanic:    "limitIfNotSet must be greater than 0",
		},
		{
			name:             "Invalid: maxLimit < limitIfNotSet",
			limitIfNotSet:    10,
			maxLimit:         8,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedPanic:    "maxLimit must be greater than or equal to limitIfNotSet",
		},
		{
			name:             "Invalid: No applyCursorsFunc",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: nil, // No ApplyCursorsFunc provided
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedPanic:    "applyCursorsFunc must be set",
		},
		{
			name:             "Invalid: first > maxLimit",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(21),
			},
			expectedError: "first must be less than or equal to max limit",
		},
		{
			name:             "Invalid: last > maxLimit",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(21),
			},
			expectedError: "last must be less than or equal to max limit",
		},
		{
			name:             "Invalid: after == before",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 9 + 1, Name: "name9", Age: 91}, primaryOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 9 + 1, Name: "name9", Age: 91}, primaryOrderByKeys,
				)),
			},
			expectedError: "after == before",
		},
		{
			name:               "Limit if not set",
			limitIfNotSet:      10,
			maxLimit:           20,
			applyCursorsFunc:   applyCursorsFunc,
			paginateRequest:    &relay.PaginateRequest[*User]{},
			expectedEdgesLen:   10,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 9 + 1, Name: "name9", Age: 91}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "First 2 after cursor 0",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				First: lo.ToPtr(2),
			},
			expectedEdgesLen:   2,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 2 + 1, Name: "name2", Age: 98}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "First 2 without after cursor",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(2),
			},
			expectedEdgesLen:   2,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "Last 2 before cursor 18",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 18 + 1, Name: "name18", Age: 82}, primaryOrderByKeys,
				)),
				Last: lo.ToPtr(2),
			},
			expectedEdgesLen:   2,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 16 + 1, Name: "name16", Age: 84}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 17 + 1, Name: "name17", Age: 83}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "Last 10 without before cursor",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(10),
			},
			expectedEdgesLen:   10,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 90 + 1, Name: "name90", Age: 10}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 8, First 5",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 8 + 1, Name: "name8", Age: 92}, primaryOrderByKeys,
				)),
				First: lo.ToPtr(5),
			},
			expectedEdgesLen:   5,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 5 + 1, Name: "name5", Age: 95}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 4, First 8",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 4 + 1, Name: "name4", Age: 96}, primaryOrderByKeys,
				)),
				First: lo.ToPtr(8),
			},
			expectedEdgesLen:   3,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 8, Last 5",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 8 + 1, Name: "name8", Age: 92}, primaryOrderByKeys,
				)),
				Last: lo.ToPtr(5),
			},
			expectedEdgesLen:   5,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 7 + 1, Name: "name7", Age: 93}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 4, Last 8",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 4 + 1, Name: "name4", Age: 96}, primaryOrderByKeys,
				)),
				Last: lo.ToPtr(8),
			},
			expectedEdgesLen:   3,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "After cursor 99",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, primaryOrderByKeys,
				)),
			},
			expectedEdgesLen:   0,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor:     nil,
				EndCursor:       nil,
			},
		},
		{
			name:             "Before cursor 0",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
			},
			expectedEdgesLen:   0,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     nil,
				EndCursor:       nil,
			},
		},
		{
			name:             "First 200",
			limitIfNotSet:    10,
			maxLimit:         300,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(200),
			},
			expectedEdgesLen:   100,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "Last 200",
			limitIfNotSet:    10,
			maxLimit:         300,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(200),
			},
			expectedEdgesLen:   100,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "First 0",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(0),
			},
			expectedEdgesLen:   0,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     nil,
				EndCursor:       nil,
			},
		},
		{
			name:             "Last 0",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(0),
			},
			expectedEdgesLen:   0,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor:     nil,
				EndCursor:       nil,
			},
		},
		{
			name:             "After cursor 95, First 10",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 95 + 1, Name: "name95", Age: 5}, primaryOrderByKeys,
				)),
				First: lo.ToPtr(10),
			},
			expectedEdgesLen:   4,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 96 + 1, Name: "name96", Age: 4}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, primaryOrderByKeys,
				)),
			},
		},
		{
			name:             "Before cursor 4, Last 10",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 4 + 1, Name: "name4", Age: 96}, primaryOrderByKeys,
				)),
				Last: lo.ToPtr(10),
			},
			expectedEdgesLen:   4,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, primaryOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, primaryOrderByKeys,
				)),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectedPanic != "" {
				require.PanicsWithValue(t, tc.expectedPanic, func() {
					relay.New(
						tc.applyCursorsFunc,
						append(otherMiddlewares, relay.EnsureLimits[*User](tc.maxLimit, tc.limitIfNotSet))...,
					)
				})
				return
			}

			p := relay.New(
				tc.applyCursorsFunc,
				append(otherMiddlewares, relay.EnsureLimits[*User](tc.maxLimit, tc.limitIfNotSet))...,
			)
			conn, err := p.Paginate(context.Background(), tc.paginateRequest)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Len(t, conn.Edges, tc.expectedEdgesLen)
			require.Equal(t, tc.expectedTotalCount, conn.TotalCount)
			require.Equal(t, tc.expectedPageInfo, conn.PageInfo)
		})
	}
}

func TestKeysetEmptyOrderBys(t *testing.T) {
	conn, err := relay.New(
		NewKeysetAdapter[*User](db),
		relay.EnsureLimits[*User](10, 10),
	).Paginate(context.Background(), &relay.PaginateRequest[*User]{
		First: lo.ToPtr(10),
	})
	require.ErrorContains(t, err, "no keys to encode cursor, orderBys must be set for keyset")
	require.Nil(t, conn)
}

func TestKeysetInvalidCursor(t *testing.T) {
	resetDB(t)

	p := relay.New(
		func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
			// This is a generic(T: any) function, so we need to cast the model to the correct type
			return NewKeysetAdapter[any](db.Model(&User{}))(ctx, req)
		},
		relay.EnsurePrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false}),
		relay.EnsureLimits[any](10, 10),
	)
	conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		After: lo.ToPtr(`{"ID":1}`),
		First: lo.ToPtr(10),
	})
	require.NoError(t, err)
	require.Len(t, conn.Edges, 10)
	require.Equal(t, 1+1, conn.Edges[0].Node.(*User).ID)
	require.Equal(t, 10+1, conn.Edges[len(conn.Edges)-1].Node.(*User).ID)
	require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
	require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))

	conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		After: lo.ToPtr(`{"FieldNotExists":1}`),
		First: lo.ToPtr(10),
	})
	require.ErrorContains(t, err, `key "ID" not found in cursor`)
	require.Nil(t, conn)

	conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		After: lo.ToPtr(`{"ID":1,"Name":"name0"}`),
		First: lo.ToPtr(10),
	})
	require.ErrorContains(t, err, `cursor has 2 keys, but 1 keys are expected`)
	require.Nil(t, conn)

	conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		Before: lo.ToPtr(`invalid`),
		First:  lo.ToPtr(10),
	})
	require.ErrorContains(t, err, `unmarshal cursor`)
	require.Nil(t, conn)
}
