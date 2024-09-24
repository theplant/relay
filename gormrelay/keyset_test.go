package gormrelay

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	relay "github.com/theplant/gorelay"
	"github.com/theplant/gorelay/cursor"
	"github.com/theplant/testenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

func TestMain(m *testing.M) {
	env, err := testenv.New().DBEnable(true).SetUp()
	if err != nil {
		panic(err)
	}
	defer env.TearDown()

	db = env.DB
	db.Logger = db.Logger.LogMode(logger.Info)

	m.Run()
}

func resetDB(t *testing.T) {
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS users").Error)
	require.NoError(t, db.AutoMigrate(&User{}))

	vs := []*User{}
	for i := 0; i < 100; i++ {
		vs = append(vs, &User{
			Name: fmt.Sprintf("name%d", i),
			Age:  100 - i,
		})
	}
	err := db.Session(&gorm.Session{Logger: logger.Discard}).Create(vs).Error
	require.NoError(t, err)
}

type User struct {
	ID   int    `gorm:"primarykey;not null;" json:"id"`
	Name string `gorm:"not null;" json:"name"`
	Age  int    `gorm:"index;not null;" json:"age"`
}

func MustJsonString(v any) string {
	s, err := jsoniter.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(s)
}

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

	defaultOrderBys := []relay.OrderBy{
		{Field: "ID", Desc: false},
		{Field: "Age", Desc: true},
	}
	defaultOrderByKeys := []string{"ID", "Age"}
	applyCursorsFunc := func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
		return NewKeysetAdapter[*User](db)(ctx, req)
	}

	testCases := []struct {
		name             string
		limitIfNotSet    int
		maxLimit         int
		applyCursorsFunc relay.ApplyCursorsFunc[*User]
		paginateRequest  *relay.PaginateRequest[*User]
		expectedEdgesLen int
		expectedPageInfo relay.PageInfo
		expectedError    string
		expectedPanic    string
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
					&User{ID: 9 + 1, Name: "name9", Age: 91}, defaultOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 9 + 1, Name: "name9", Age: 91}, defaultOrderByKeys,
				)),
			},
			expectedError: "after == before",
		},
		{
			name:             "Limit if not set",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedEdgesLen: 10,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 9 + 1, Name: "name9", Age: 91}, defaultOrderByKeys,
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
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				First: lo.ToPtr(2),
			},
			expectedEdgesLen: 2,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 2 + 1, Name: "name2", Age: 98}, defaultOrderByKeys,
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
			expectedEdgesLen: 2,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, defaultOrderByKeys,
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
					&User{ID: 18 + 1, Name: "name18", Age: 82}, defaultOrderByKeys,
				)),
				Last: lo.ToPtr(2),
			},
			expectedEdgesLen: 2,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 16 + 1, Name: "name16", Age: 84}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 17 + 1, Name: "name17", Age: 83}, defaultOrderByKeys,
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
			expectedEdgesLen: 10,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 90 + 1, Name: "name90", Age: 10}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, defaultOrderByKeys,
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
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 8 + 1, Name: "name8", Age: 92}, defaultOrderByKeys,
				)),
				First: lo.ToPtr(5),
			},
			expectedEdgesLen: 5,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 5 + 1, Name: "name5", Age: 95}, defaultOrderByKeys,
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
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 4 + 1, Name: "name4", Age: 96}, defaultOrderByKeys,
				)),
				First: lo.ToPtr(8),
			},
			expectedEdgesLen: 3,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, defaultOrderByKeys,
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
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 8 + 1, Name: "name8", Age: 92}, defaultOrderByKeys,
				)),
				Last: lo.ToPtr(5),
			},
			expectedEdgesLen: 5,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 7 + 1, Name: "name7", Age: 93}, defaultOrderByKeys,
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
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				Before: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 4 + 1, Name: "name4", Age: 96}, defaultOrderByKeys,
				)),
				Last: lo.ToPtr(8),
			},
			expectedEdgesLen: 3,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 1 + 1, Name: "name1", Age: 99}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, defaultOrderByKeys,
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
					&User{ID: 99 + 1, Name: "name99", Age: 1}, defaultOrderByKeys,
				)),
			},
			expectedEdgesLen: 0,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
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
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
			},
			expectedEdgesLen: 0,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
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
			expectedEdgesLen: 100,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, defaultOrderByKeys,
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
			expectedEdgesLen: 100,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, defaultOrderByKeys,
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
			expectedEdgesLen: 0,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
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
			expectedEdgesLen: 0,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
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
					&User{ID: 95 + 1, Name: "name95", Age: 5}, defaultOrderByKeys,
				)),
				First: lo.ToPtr(10),
			},
			expectedEdgesLen: 4,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 96 + 1, Name: "name96", Age: 4}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 99 + 1, Name: "name99", Age: 1}, defaultOrderByKeys,
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
					&User{ID: 4 + 1, Name: "name4", Age: 96}, defaultOrderByKeys,
				)),
				Last: lo.ToPtr(10),
			},
			expectedEdgesLen: 4,
			expectedPageInfo: relay.PageInfo{
				TotalCount:      100,
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 0 + 1, Name: "name0", Age: 100}, defaultOrderByKeys,
				)),
				EndCursor: lo.ToPtr(mustEncodeKeysetCursor(
					&User{ID: 3 + 1, Name: "name3", Age: 97}, defaultOrderByKeys,
				)),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectedPanic != "" {
				require.PanicsWithValue(t, tc.expectedPanic, func() {
					relay.New(false, tc.maxLimit, tc.limitIfNotSet, defaultOrderBys, tc.applyCursorsFunc)
				})
				return
			}

			p := relay.New(false, tc.maxLimit, tc.limitIfNotSet, defaultOrderBys, tc.applyCursorsFunc)
			resp, err := p.Paginate(context.Background(), tc.paginateRequest)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Len(t, resp.Edges, tc.expectedEdgesLen)
			require.Equal(t, tc.expectedPageInfo, resp.PageInfo)
		})
	}
}

func TestKeysetWithoutCounter(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, applyCursorFunc relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			false,
			10, 10,
			[]relay.OrderBy{
				{Field: "ID", Desc: false},
			},
			applyCursorFunc,
		)
		resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Len(t, resp.Edges, 10)
		require.Equal(t, 1, resp.Edges[0].Node.ID)
		require.Equal(t, 10, resp.Edges[len(resp.Edges)-1].Node.ID)
		require.Zero(t, resp.PageInfo.TotalCount)
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, cursor.NewKeysetAdapter(NewKeysetFinder[*User](db))) })
	t.Run("offset", func(t *testing.T) { testCase(t, cursor.NewOffsetAdapter(NewOffsetFinder[*User](db))) })
}

func TestUnexpectOrderBys(t *testing.T) {
	require.PanicsWithValue(t, "orderBysIfNotSet must be set", func() {
		relay.New(false, 10, 10, nil, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
			return nil, nil
		})
	})
	require.PanicsWithValue(t, "orderBysIfNotSet must be set", func() {
		relay.New(false, 10, 10, []relay.OrderBy{}, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
			return nil, nil
		})
	})

	p := relay.New(false, 10, 10,
		[]relay.OrderBy{
			{Field: "ID", Desc: false},
		}, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
			return nil, nil
		},
	)
	resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
		First: lo.ToPtr(10),
		OrderBys: []relay.OrderBy{
			{Field: "ID", Desc: false},
			{Field: "ID", Desc: true},
		},
	})
	require.ErrorContains(t, err, "duplicated order by fields [ID]")
	require.Nil(t, resp)
}

func TestContext(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		{
			p := relay.New(
				false,
				10, 10,
				[]relay.OrderBy{
					{Field: "ID", Desc: false},
				},
				func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
					return f(db)(ctx, req)
				},
			)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			resp, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "context canceled")
			require.Nil(t, resp)
		}

		{
			p := relay.New(
				false,
				10, 10,
				[]relay.OrderBy{
					{Field: "ID", Desc: false},
				}, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
					// Set WithContext here
					return f(db.WithContext(ctx))(ctx, req)
				},
			)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			resp, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "context canceled")
			require.Nil(t, resp)
		}
	}
	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

func generateAESKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func TestWrapEncrypt(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, w func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			false,
			10, 10,
			[]relay.OrderBy{
				{Field: "ID", Desc: false},
			},
			func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return w(NewKeysetAdapter[*User](db))(ctx, req)
			},
		)
		resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Len(t, resp.Edges, 10)
		require.Equal(t, 1, resp.Edges[0].Node.ID)
		require.Equal(t, 10, resp.Edges[len(resp.Edges)-1].Node.ID)
		require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
		require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))

		// next page
		resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(5),
			After: resp.PageInfo.EndCursor,
		})
		require.NoError(t, err)
		require.Len(t, resp.Edges, 5)
		require.Equal(t, 11, resp.Edges[0].Node.ID)
		require.Equal(t, 15, resp.Edges[len(resp.Edges)-1].Node.ID)
		require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
		require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))

		// prev page
		resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			Last:   lo.ToPtr(6),
			Before: resp.PageInfo.StartCursor,
		})
		require.NoError(t, err)
		require.Len(t, resp.Edges, 6)
		require.Equal(t, 5, resp.Edges[0].Node.ID)
		require.Equal(t, 10, resp.Edges[len(resp.Edges)-1].Node.ID)
		require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
		require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))

		// invalid after cursor
		resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(5),
			After: lo.ToPtr("invalid"),
		})
		require.ErrorContains(t, err, "invalid after cursor")
		require.Nil(t, resp)
	}

	t.Run("WrapBase64", func(t *testing.T) {
		testCase(t, func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User] {
			return cursor.WrapBase64(next)
		})
	})
	t.Run("WrapAES", func(t *testing.T) {
		encryptionKey, err := generateAESKey(32)
		require.NoError(t, err)
		testCase(t, func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User] {
			return cursor.WrapAES(next, encryptionKey)
		})
	})

	t.Run("WrapMockError", func(t *testing.T) {
		p := relay.New(
			false,
			10, 10,
			[]relay.OrderBy{
				{Field: "ID", Desc: false},
			},
			func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User] {
					return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
						resp, err := next(ctx, req)
						if err != nil {
							return nil, err
						}

						for i := range resp.Edges {
							edge := &resp.Edges[i]
							edge.Cursor = func(ctx context.Context, node *User) (string, error) {
								return "", errors.New("mock error")
							}
						}

						return resp, nil
					}
				}(NewKeysetAdapter[*User](db))(ctx, req)
			},
		)
		resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.ErrorContains(t, err, "mock error")
		require.Nil(t, resp)
	})
}

func TestNodesOnly(t *testing.T) {
	resetDB(t)

	p := relay.New(
		true,
		10, 10,
		[]relay.OrderBy{
			{Field: "ID", Desc: false},
		},
		func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
			return NewKeysetAdapter[*User](db)(ctx, req)
		},
	)
	resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
		First: lo.ToPtr(10),
	})
	require.NoError(t, err)
	require.Equal(t, 100, resp.PageInfo.TotalCount)
	require.NotNil(t, resp.PageInfo.StartCursor)
	require.NotNil(t, resp.PageInfo.EndCursor)
	require.Len(t, resp.Edges, 0)
	require.Len(t, resp.Nodes, 10)
	require.Equal(t, 1, resp.Nodes[0].ID)
	require.Equal(t, 10, resp.Nodes[len(resp.Nodes)-1].ID)
}

func TestKeysetGenericTypeAny(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[any]) {
		t.Run("Right", func(t *testing.T) {
			p := relay.New(
				false,
				10, 10,
				[]relay.OrderBy{
					{Field: "ID", Desc: false},
				}, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
					// This is a generic(T: any) function, so we need to call db.Model(x)
					return f(db.Model(&User{}))(ctx, req)
				},
			)
			resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Len(t, resp.Edges, 10)
			require.Equal(t, 1, resp.Edges[0].Node.(*User).ID)
			require.Equal(t, 10, resp.Edges[len(resp.Edges)-1].Node.(*User).ID)
			require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
			require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))

			resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				Last: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Len(t, resp.Edges, 10)
			require.Equal(t, 91, resp.Edges[0].Node.(*User).ID)
			require.Equal(t, 100, resp.Edges[len(resp.Edges)-1].Node.(*User).ID)
			require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
			require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))
		})
		t.Run("Wrong", func(t *testing.T) {
			p := relay.New(
				false,
				10, 10,
				[]relay.OrderBy{
					{Field: "ID", Desc: false},
				}, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
					// This is wrong, we need to call db.Model(x) for generic(T: any) function
					return f(db)(ctx, req)
				},
			)
			resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "db.Statement.Model is nil and T is not a struct or struct pointer")
			require.Nil(t, resp)
		})
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })

	anotherTestCase := func(t *testing.T, applyCursorsFunc relay.ApplyCursorsFunc[any]) {
		t.Run("Wrong(WithoutCounter)", func(t *testing.T) {
			p := relay.New(
				false,
				10, 10,
				[]relay.OrderBy{
					{Field: "ID", Desc: false},
				},
				applyCursorsFunc,
			)
			resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "db.Statement.Model is nil and T is not a struct or struct pointer")
			require.Nil(t, resp)
		})
	}

	// This is wrong, we need to call db.Model(x) for generic(T: any) function
	t.Run("keyset", func(t *testing.T) { anotherTestCase(t, cursor.NewKeysetAdapter(NewKeysetFinder[any](db))) })
	t.Run("offset", func(t *testing.T) { anotherTestCase(t, cursor.NewOffsetAdapter(NewOffsetFinder[any](db))) })
}

func TestKeysetInvalidCursor(t *testing.T) {
	resetDB(t)

	p := relay.New(
		false,
		10, 10,
		[]relay.OrderBy{
			{Field: "ID", Desc: false},
		}, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
			// This is a generic(T: any) function, so we need to cast the model to the correct type
			return NewKeysetAdapter[any](db.Model(&User{}))(ctx, req)
		},
	)
	resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		After: lo.ToPtr(`{"ID":1}`),
		First: lo.ToPtr(10),
	})
	require.NoError(t, err)
	require.Len(t, resp.Edges, 10)
	require.Equal(t, 1+1, resp.Edges[0].Node.(*User).ID)
	require.Equal(t, 10+1, resp.Edges[len(resp.Edges)-1].Node.(*User).ID)
	require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
	require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))

	resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		After: lo.ToPtr(`{"FieldNotExists":1}`),
		First: lo.ToPtr(10),
	})
	require.ErrorContains(t, err, `key "ID" not found in cursor`)
	require.Nil(t, resp)

	resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		After: lo.ToPtr(`{"ID":1,"Name":"name0"}`),
		First: lo.ToPtr(10),
	})
	require.ErrorContains(t, err, `cursor length != keys length`)
	require.Nil(t, resp)

	resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
		Before: lo.ToPtr(`invalid`),
		First:  lo.ToPtr(10),
	})
	require.ErrorContains(t, err, `unmarshal cursor`)
	require.Nil(t, resp)
}

func TestTotalCountZero(t *testing.T) {
	resetDB(t)
	require.NoError(t, db.Exec("DELETE FROM users").Error)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			false,
			10, 10,
			[]relay.OrderBy{
				{Field: "ID", Desc: false},
			},
			func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return f(db)(ctx, req)
			},
		)
		resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, 0, resp.PageInfo.TotalCount)
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}
