package gormrelay

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
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

func TestUnexpectOrderBys(t *testing.T) {
	resetDB(t)

	p := relay.New(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
		return nil, nil
	})
	conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
		First: lo.ToPtr(10),
		OrderBys: []relay.OrderBy{
			{Field: "ID", Desc: false},
			{Field: "ID", Desc: true},
		},
	})
	require.ErrorContains(t, err, "duplicated order by fields [ID]")
	require.Nil(t, conn)
}

func TestContext(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		{
			p := relay.New(
				f(db),
				relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
				relay.EnsureLimits[*User](10, 10),
			)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "context canceled")
			require.Nil(t, conn)
		}

		{
			p := relay.New(
				func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
					// Set WithContext here
					return f(db.WithContext(ctx))(ctx, req)
				},
				relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
				relay.EnsureLimits[*User](10, 10),
			)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "context canceled")
			require.Nil(t, conn)
		}
	}
	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

func TestSkip(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
			relay.EnsureLimits[*User](10, 10),
		)
		t.Run("SkipEdges", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				Edges: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.NotNil(t, conn.PageInfo.StartCursor)
			require.NotNil(t, conn.PageInfo.EndCursor)
			require.Nil(t, conn.Edges)
			require.Len(t, conn.Nodes, 10)
			require.Equal(t, 1, conn.Nodes[0].ID)
			require.Equal(t, 10, conn.Nodes[len(conn.Nodes)-1].ID)
		})
		t.Run("SkipNodes", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				Nodes: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.NotNil(t, conn.PageInfo.StartCursor)
			require.NotNil(t, conn.PageInfo.EndCursor)
			require.Len(t, conn.Edges, 10)
			require.Nil(t, conn.Nodes)
			require.Equal(t, 1, conn.Edges[0].Node.ID)
			require.Equal(t, 10, conn.Edges[len(conn.Edges)-1].Node.ID)
		})
		t.Run("SkipPageInfo", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				PageInfo: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.Nil(t, conn.PageInfo)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 1, conn.Edges[0].Node.ID)
			require.Equal(t, 10, conn.Edges[len(conn.Edges)-1].Node.ID)
			require.Len(t, conn.Nodes, 10)
			require.Equal(t, 1, conn.Nodes[0].ID)
			require.Equal(t, 10, conn.Nodes[len(conn.Nodes)-1].ID)
		})
		t.Run("SkipTotalCount", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				TotalCount: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Nil(t, conn.TotalCount)
			require.NotNil(t, conn.PageInfo.StartCursor)
			require.NotNil(t, conn.PageInfo.EndCursor)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 1, conn.Edges[0].Node.ID)
			require.Equal(t, 10, conn.Edges[len(conn.Edges)-1].Node.ID)
			require.Len(t, conn.Nodes, 10)
			require.Equal(t, 1, conn.Nodes[0].ID)
			require.Equal(t, 10, conn.Nodes[len(conn.Nodes)-1].ID)
		})
		t.Run("SkipEdgesAndNodes", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				Edges: true,
				Nodes: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.NotNil(t, conn.PageInfo.StartCursor)
			require.NotNil(t, conn.PageInfo.EndCursor)
			require.Len(t, conn.Edges, 0)
			require.Len(t, conn.Nodes, 0)
		})
		t.Run("SkipEdgesAndNodesAndPageInfo", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				Edges:    true,
				Nodes:    true,
				PageInfo: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.Nil(t, conn.PageInfo)
			require.Len(t, conn.Edges, 0)
			require.Len(t, conn.Nodes, 0)
		})
		t.Run("SkipAll", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				Edges:      true,
				Nodes:      true,
				PageInfo:   true,
				TotalCount: true,
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Nil(t, conn.TotalCount)
			require.Nil(t, conn.PageInfo)
			require.Len(t, conn.Edges, 0)
			require.Len(t, conn.Nodes, 0)
		})
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

func TestGenericTypeAny(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[any]) {
		t.Run("Correct", func(t *testing.T) {
			p := relay.New(
				func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
					// This is a generic(T: any) function, so we need to call db.Model(x)
					return f(db.Model(&User{}))(ctx, req)
				},
				relay.EnsurePrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false}),
				relay.EnsureLimits[any](10, 10),
			)
			conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 1, conn.Edges[0].Node.(*User).ID)
			require.Equal(t, 10, conn.Edges[len(conn.Edges)-1].Node.(*User).ID)
			require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
			require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))

			conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				Last: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 91, conn.Edges[0].Node.(*User).ID)
			require.Equal(t, 100, conn.Edges[len(conn.Edges)-1].Node.(*User).ID)
			require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
			require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))
		})
		t.Run("Wrong", func(t *testing.T) {
			p := relay.New(
				func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
					// This is wrong, we need to call db.Model(x) for generic(T: any) function
					return f(db)(ctx, req)
				},
				relay.EnsurePrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false}),
				relay.EnsureLimits[any](10, 10),
			)
			conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[any]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "db.Statement.Model is nil and T is not a struct or struct pointer")
			require.Nil(t, conn)
		})
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })

	anotherTestCase := func(t *testing.T, applyCursorsFunc relay.ApplyCursorsFunc[any]) {
		t.Run("Wrong(SkipTotalCount)", func(t *testing.T) {
			p := relay.New(
				applyCursorsFunc,
				relay.EnsurePrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false}),
				relay.EnsureLimits[any](10, 10),
			)
			conn, err := p.Paginate(
				relay.WithSkip(context.Background(), relay.Skip{
					TotalCount: true,
				}),
				&relay.PaginateRequest[any]{
					First: lo.ToPtr(10),
				},
			)
			require.ErrorContains(t, err, "db.Statement.Model is nil and T is not a struct or struct pointer")
			require.Nil(t, conn)
		})
	}

	// This is wrong, we need to call db.Model(x) for generic(T: any) function
	t.Run("keyset", func(t *testing.T) { anotherTestCase(t, NewKeysetAdapter[any](db)) })
	t.Run("offset", func(t *testing.T) { anotherTestCase(t, NewOffsetAdapter[any](db)) })
}

func TestTotalCountZero(t *testing.T) {
	resetDB(t)
	require.NoError(t, db.Exec("DELETE FROM users").Error)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
			relay.EnsureLimits[*User](10, 10),
		)
		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, lo.ToPtr(0), conn.TotalCount)
		require.Nil(t, conn.PageInfo.StartCursor)
		require.Nil(t, conn.PageInfo.EndCursor)
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

func generateGCMKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, errors.Wrap(err, "could not generate key")
	}
	return key, nil
}

func TestCursorMiddleware(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
			relay.EnsureLimits[*User](10, 10),
		)
		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Len(t, conn.Edges, 10)
		require.Equal(t, 1, conn.Edges[0].Node.ID)
		require.Equal(t, 10, conn.Edges[len(conn.Edges)-1].Node.ID)
		require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
		require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))

		// next page
		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(5),
			After: conn.PageInfo.EndCursor,
		})
		require.NoError(t, err)
		require.Len(t, conn.Edges, 5)
		require.Equal(t, 11, conn.Edges[0].Node.ID)
		require.Equal(t, 15, conn.Edges[len(conn.Edges)-1].Node.ID)
		require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
		require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))

		// prev page
		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			Last:   lo.ToPtr(6),
			Before: conn.PageInfo.StartCursor,
		})
		require.NoError(t, err)
		require.Len(t, conn.Edges, 6)
		require.Equal(t, 5, conn.Edges[0].Node.ID)
		require.Equal(t, 10, conn.Edges[len(conn.Edges)-1].Node.ID)
		require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
		require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))

		// invalid after cursor
		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(5),
			After: lo.ToPtr("invalid%20x"),
		})
		require.ErrorContains(t, err, "invalid after cursor")
		require.Nil(t, conn)
	}

	t.Run("Base64", func(t *testing.T) {
		t.Run("keyset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB) relay.ApplyCursorsFunc[*User] {
				return cursor.Base64[*User](NewKeysetAdapter[*User](db))
			})
		})
		t.Run("keyset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB) relay.ApplyCursorsFunc[*User] {
				return cursor.Base64[*User](NewOffsetAdapter[*User](db))
			})
		})
	})

	t.Run("GCM", func(t *testing.T) {
		encryptionKey, err := generateGCMKey(32)
		require.NoError(t, err)

		gcm, err := cursor.NewGCM(encryptionKey)
		require.NoError(t, err)

		t.Run("keyset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB) relay.ApplyCursorsFunc[*User] {
				return cursor.GCM[*User](gcm)(NewKeysetAdapter[*User](db))
			})
		})

		t.Run("offset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB) relay.ApplyCursorsFunc[*User] {
				return cursor.GCM[*User](gcm)(NewOffsetAdapter[*User](db))
			})
		})
	})

	t.Run("MockError", func(t *testing.T) {
		testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
			p := relay.New(
				func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User] {
					return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
						rsp, err := next(ctx, req)
						if err != nil {
							return nil, err
						}

						for i := range rsp.LazyEdges {
							edge := rsp.LazyEdges[i]
							edge.Cursor = func(ctx context.Context, node *User) (string, error) {
								return "", errors.New("mock error")
							}
						}

						return rsp, nil
					}
				}(f(db)),
				relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
				relay.EnsureLimits[*User](10, 10),
			)
			conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "mock error")
			require.Nil(t, conn)
		}
		t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
		t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
	})
}

func TestAppendCursorMiddleware(t *testing.T) {
	resetDB(t)

	encryptionKey, err := generateGCMKey(32)
	require.NoError(t, err)

	gcm, err := cursor.NewGCM(encryptionKey)
	require.NoError(t, err)

	gcmMiddleware := cursor.GCM[*User](gcm)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.AppendCursorMiddleware(gcmMiddleware),
			relay.EnsurePrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false}),
			relay.EnsureLimits[*User](10, 10),
		)

		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, lo.ToPtr(100), conn.TotalCount)
		require.Len(t, conn.Edges, 10)
		require.Equal(t, 0+1, conn.Edges[0].Node.ID)
		require.Equal(t, 9+1, conn.Edges[len(conn.Edges)-1].Node.ID)
		require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
		require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))

		// next page
		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			After: conn.PageInfo.EndCursor,
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, lo.ToPtr(100), conn.TotalCount)
		require.Len(t, conn.Edges, 10)
		require.Equal(t, 10+1, conn.Edges[0].Node.ID)
		require.Equal(t, 19+1, conn.Edges[len(conn.Edges)-1].Node.ID)
		require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
		require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}
