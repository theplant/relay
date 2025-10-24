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
	"github.com/theplant/testenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
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

func TestUnexpectOrderBy(t *testing.T) {
	resetDB(t)

	p := relay.New(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
		return nil, nil
	})
	conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
		First: lo.ToPtr(10),
		OrderBy: []relay.Order{
			{Field: "ID", Direction: relay.OrderDirectionAsc},
			{Field: "ID", Direction: relay.OrderDirectionDesc},
		},
	})
	require.ErrorContains(t, err, "duplicated order by fields [ID]")
	require.Nil(t, conn)
}

func TestContext(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		{
			p := relay.New(
				f(db),
				relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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
				relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
			relay.EnsureLimits[*User](10, 10),
		)
		t.Run("SkipEdges", func(t *testing.T) {
			ctx := relay.WithSkip(context.Background(), relay.Skip{
				Edges: true,
			})
			t.Run("First 10", func(t *testing.T) {
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
			t.Run("First 1", func(t *testing.T) {
				conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
					First: lo.ToPtr(1),
				})
				require.NoError(t, err)
				require.Equal(t, lo.ToPtr(100), conn.TotalCount)
				require.NotNil(t, conn.PageInfo.StartCursor)
				require.NotNil(t, conn.PageInfo.EndCursor)
				require.Nil(t, conn.Edges)
				require.Len(t, conn.Nodes, 1)
				require.Equal(t, 1, conn.Nodes[0].ID)
				require.Equal(t, 1, conn.Nodes[len(conn.Nodes)-1].ID)
			})
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

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[any]) relay.ApplyCursorsFunc[any]) {
		t.Run("Correct", func(t *testing.T) {
			p := relay.New(
				func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
					// This is a generic(T: any) function, so we need to call db.Model(x)
					return f(db.Model(&User{}))(ctx, req)
				},
				relay.EnsurePrimaryOrderBy[any](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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
				relay.EnsurePrimaryOrderBy[any](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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
				relay.EnsurePrimaryOrderBy[any](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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

func TestCursorHook(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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
			testCase(t, func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User] {
				return cursor.Base64[*User](NewKeysetAdapter[*User](db))
			})
		})
		t.Run("keyset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User] {
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
			testCase(t, func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User] {
				return cursor.GCM[*User](gcm)(NewKeysetAdapter[*User](db))
			})
		})

		t.Run("offset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User] {
				return cursor.GCM[*User](gcm)(NewOffsetAdapter[*User](db))
			})
		})
	})

	t.Run("MockError", func(t *testing.T) {
		testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
			p := relay.New(
				func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User] {
					return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
						rsp, err := next(ctx, req)
						if err != nil {
							return nil, err
						}

						for i := range rsp.LazyEdges {
							edge := rsp.LazyEdges[i]
							edge.Cursor = func(ctx context.Context) (string, error) {
								return "", errors.New("mock error")
							}
						}

						return rsp, nil
					}
				}(f(db)),
				relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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

func TestPrependCursorHook(t *testing.T) {
	resetDB(t)

	encryptionKey, err := generateGCMKey(32)
	require.NoError(t, err)

	gcm, err := cursor.NewGCM(encryptionKey)
	require.NoError(t, err)

	gcmHook := cursor.GCM[*User](gcm)

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.PrependCursorHook(gcmHook),
			relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
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

func TestWithNodeProcessor(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
			relay.EnsureLimits[*User](10, 10),
		)
		t.Run("AddSuffixForName", func(t *testing.T) {
			ctx := relay.WithNodeProcessor(context.Background(), func(ctx context.Context, node *User) (*User, error) {
				node.Name = node.Name + "_suffix"
				return node, nil
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 0+1, conn.Edges[0].Node.ID)
			require.Equal(t, 9+1, conn.Edges[len(conn.Edges)-1].Node.ID)
			require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
			require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))
			require.Equal(t, "name0_suffix", conn.Edges[0].Node.Name)
			require.Equal(t, "name9_suffix", conn.Edges[len(conn.Edges)-1].Node.Name)
		})
		t.Run("MockError", func(t *testing.T) {
			ctx := relay.WithNodeProcessor(context.Background(), func(ctx context.Context, node *User) (*User, error) {
				return nil, errors.New("mock error")
			})
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "mock error")
			require.Nil(t, conn)
		})
		t.Run("OtherType", func(t *testing.T) {
			ctx := relay.WithNodeProcessor(context.Background(), func(ctx context.Context, node struct{}) (struct{}, error) {
				return struct{}{}, errors.New("mock error")
			}) // will be ignored
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 0+1, conn.Edges[0].Node.ID)
			require.Equal(t, 9+1, conn.Edges[len(conn.Edges)-1].Node.ID)
			require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
			require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))
		})
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

func TestOrderBy(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, cursorTest bool, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(db),
			relay.EnsurePrimaryOrderBy[*User](relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc}),
			relay.EnsureLimits[*User](10, 10),
		)
		ctx := context.Background()
		t.Run("Normal", func(t *testing.T) {
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
				OrderBy: []relay.Order{
					{Field: "ID", Direction: relay.OrderDirectionDesc},
				},
			})
			require.NoError(t, err)
			require.Equal(t, lo.ToPtr(100), conn.TotalCount)
			require.Len(t, conn.Edges, 10)
			require.Equal(t, 99+1, conn.Edges[0].Node.ID)
			require.Equal(t, 90+1, conn.Edges[len(conn.Edges)-1].Node.ID)
			require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
			require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))
		})
		t.Run("UnexpectField", func(t *testing.T) {
			conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
				OrderBy: []relay.Order{
					{Field: "Unexpect", Direction: relay.OrderDirectionDesc},
				},
			})
			require.ErrorContains(t, err, `missing field "Unexpect" in schema`)
			require.Nil(t, conn)
		})
		if cursorTest {
			t.Run("after id3 by id desc", func(t *testing.T) {
				conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
					After: lo.ToPtr(mustEncodeKeysetCursor(
						&User{ID: 2 + 1, Name: "name2", Age: 98}, []string{"ID"},
					)),
					First: lo.ToPtr(10),
					OrderBy: []relay.Order{
						{Field: "ID", Direction: relay.OrderDirectionDesc},
					},
				})
				require.NoError(t, err)
				require.Equal(t, lo.ToPtr(100), conn.TotalCount)
				require.Len(t, conn.Edges, 2)
				require.Equal(t, 1+1, conn.Edges[0].Node.ID)
				require.Equal(t, 0+1, conn.Edges[len(conn.Edges)-1].Node.ID)
				require.Equal(t, conn.Edges[0].Cursor, *(conn.PageInfo.StartCursor))
				require.Equal(t, conn.Edges[len(conn.Edges)-1].Cursor, *(conn.PageInfo.EndCursor))
			})
			t.Run("missing keys in cursor", func(t *testing.T) {
				conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
					After: lo.ToPtr(mustEncodeKeysetCursor(
						&User{ID: 2 + 1, Name: "name2", Age: 98}, []string{"ID"},
					)),
					First: lo.ToPtr(10),
					OrderBy: []relay.Order{
						{Field: "ID", Direction: relay.OrderDirectionDesc},
						{Field: "Name", Direction: relay.OrderDirectionDesc},
					},
				})
				require.ErrorContains(t, err, "invalid cursor: has 1 keys, but 2 keys are expected")
				require.Nil(t, conn)
			})
			t.Run("more keys in cursor", func(t *testing.T) {
				conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
					After: lo.ToPtr(mustEncodeKeysetCursor(
						&User{ID: 2 + 1, Name: "name2", Age: 98}, []string{"ID", "Name", "Age"},
					)),
					First: lo.ToPtr(10),
					OrderBy: []relay.Order{
						{Field: "ID", Direction: relay.OrderDirectionDesc},
						{Field: "Name", Direction: relay.OrderDirectionDesc},
					},
				})
				require.ErrorContains(t, err, "invalid cursor: has 3 keys, but 2 keys are expected")
				require.Nil(t, conn)
			})
			t.Run("wrong keys in cursor", func(t *testing.T) {
				conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
					After: lo.ToPtr(mustEncodeKeysetCursor(
						&User{ID: 2 + 1, Name: "name2", Age: 98}, []string{"ID", "Age"},
					)),
					First: lo.ToPtr(10),
					OrderBy: []relay.Order{
						{Field: "ID", Direction: relay.OrderDirectionDesc},
						{Field: "Name", Direction: relay.OrderDirectionDesc},
					},
				})
				require.ErrorContains(t, err, `required key "Name" not found in cursor`)
				require.Nil(t, conn)
			})
			t.Run("wrong cursor keys and wrong fields", func(t *testing.T) {
				conn, err := p.Paginate(ctx, &relay.PaginateRequest[*User]{
					After: lo.ToPtr(mustEncodeKeysetCursor(
						struct {
							*User
							NameX string
						}{
							User:  &User{ID: 2 + 1, Name: "name2", Age: 98},
							NameX: "namex2",
						}, []string{"ID", "NameX"},
					)),
					First: lo.ToPtr(10),
					OrderBy: []relay.Order{
						{Field: "ID", Direction: relay.OrderDirectionDesc},
						{Field: "NameX", Direction: relay.OrderDirectionDesc},
					},
				})
				require.ErrorContains(t, err, `failed to find records with keyset pagination: missing field "NameX" in schema`)
				require.Nil(t, conn)
			})
		}
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, true, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, false, NewOffsetAdapter) })
}

func TestAppendPrimaryOrderBy(t *testing.T) {
	primaryOrderBy := []relay.Order{
		{Field: "ID", Direction: relay.OrderDirectionDesc},
		{Field: "CreatedAt", Direction: relay.OrderDirectionDesc},
	}

	require.Equal(t,
		[]relay.Order{
			{Field: "Age", Direction: relay.OrderDirectionAsc},
			{Field: "ID", Direction: relay.OrderDirectionDesc},
			{Field: "CreatedAt", Direction: relay.OrderDirectionDesc},
		},
		relay.AppendPrimaryOrderBy([]relay.Order{
			{Field: "Age", Direction: relay.OrderDirectionAsc},
		}, primaryOrderBy...),
	)

	require.Equal(t,
		[]relay.Order{
			{Field: "ID", Direction: relay.OrderDirectionAsc},
			{Field: "CreatedAt", Direction: relay.OrderDirectionDesc},
		},
		relay.AppendPrimaryOrderBy([]relay.Order{
			{Field: "ID", Direction: relay.OrderDirectionAsc},
		}, primaryOrderBy...),
	)

	require.Equal(t,
		[]relay.Order{
			{Field: "CreatedAt", Direction: relay.OrderDirectionAsc},
			{Field: "ID", Direction: relay.OrderDirectionDesc},
		},
		relay.AppendPrimaryOrderBy([]relay.Order{
			{Field: "CreatedAt", Direction: relay.OrderDirectionAsc},
		}, primaryOrderBy...),
	)

	require.Equal(t,
		[]relay.Order{
			{Field: "CreatedAt", Direction: relay.OrderDirectionAsc},
		},
		relay.AppendPrimaryOrderBy([]relay.Order{
			{Field: "CreatedAt", Direction: relay.OrderDirectionAsc},
		}),
	)
}

func TestWithComputed(t *testing.T) {
	resetDB(t)

	require.NoError(t, db.Exec("UPDATE users SET name = 'molon' WHERE id = 30").Error)
	require.NoError(t, db.Exec("UPDATE users SET name = 'sam' WHERE id = 50").Error)

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*User]) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			f(
				db,
				WithComputed(&Computed[*User]{
					Columns: NewComputedColumns(map[string]string{
						"GlobalPriority": "(CASE WHEN users.name = 'molon' THEN 1 WHEN users.name = 'sam' THEN 2 ELSE 3 END)",
					}),
					Scanner: NewComputedScanner[*User],
				}),
			),
			relay.EnsureLimits[*User](10, 50),
		)

		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(conn.Edges))
		require.Equal(t, "molon", conn.Edges[0].Node.Name)
		require.Equal(t, "sam", conn.Edges[1].Node.Name)
		require.Equal(t, "name0", conn.Edges[2].Node.Name)

		// Test pagination using cursor to fetch the next page
		nextConn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			After: conn.PageInfo.EndCursor,
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(nextConn.Edges))
		// Users with Priority=3 sorted by ID in ascending order
		require.Equal(t, "name1", nextConn.Edges[0].Node.Name)
		require.Equal(t, "name2", nextConn.Edges[1].Node.Name)
		require.Equal(t, "name3", nextConn.Edges[2].Node.Name)

		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionDesc},
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(conn.Edges))
		require.Equal(t, "name0", conn.Edges[0].Node.Name)
		require.Equal(t, "name1", conn.Edges[1].Node.Name)
		require.Equal(t, "name2", conn.Edges[2].Node.Name)

		// Test cursor-based pagination with descending Priority order
		nextConn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			After: conn.PageInfo.EndCursor,
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionDesc},
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(nextConn.Edges))
		require.Equal(t, "name3", nextConn.Edges[0].Node.Name)
		require.Equal(t, "name4", nextConn.Edges[1].Node.Name)
		require.Equal(t, "name5", nextConn.Edges[2].Node.Name)

		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionDesc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(conn.Edges))
		require.Equal(t, "molon", conn.Edges[0].Node.Name)
		require.Equal(t, "sam", conn.Edges[1].Node.Name)
		require.Equal(t, "name99", conn.Edges[2].Node.Name)

		// Test cursor pagination with mixed sort order (Priority ASC, ID DESC)
		nextConn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			After: conn.PageInfo.EndCursor,
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionDesc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(nextConn.Edges))
		require.Equal(t, "name98", nextConn.Edges[0].Node.Name)
		require.Equal(t, "name97", nextConn.Edges[1].Node.Name)
		require.Equal(t, "name96", nextConn.Edges[2].Node.Name)

		// Test pagination starting from middle of first page to verify cursor precision
		middleConn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(3),
			After: &conn.Edges[0].Cursor,
			OrderBy: []relay.Order{
				{Field: "GlobalPriority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionDesc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(middleConn.Edges))
		require.Equal(t, "sam", middleConn.Edges[0].Node.Name)
		require.Equal(t, "name99", middleConn.Edges[1].Node.Name)
		require.Equal(t, "name98", middleConn.Edges[2].Node.Name)
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

// Shop represents a store with an in-memory Priority that doesn't exist in DB
type Shop struct {
	ID       int    `gorm:"primarykey;not null;" json:"id"`
	Name     string `gorm:"not null;" json:"name"`
	Category int    `gorm:"index;not null;" json:"category"`
	Priority int    `gorm:"-" json:"priority"` // Not stored in DB, populated from computed column
}

func TestWithComputedShop(t *testing.T) {
	// Create Shop table and seed data
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS shops").Error)
	require.NoError(t, db.AutoMigrate(&Shop{}))

	// Create test shops
	shops := []*Shop{}
	for i := 0; i < 100; i++ {
		shops = append(shops, &Shop{
			Name:     fmt.Sprintf("shop%d", i),
			Category: i % 3, // Three categories: 0, 1, 2
		})
	}

	// Update some shops to specific categories for testing
	shops[30].Name = "premium"
	shops[50].Name = "featured"

	err := db.Session(&gorm.Session{Logger: logger.Discard}).Create(shops).Error
	require.NoError(t, err)

	testCase := func(t *testing.T, f func(db *gorm.DB, opts ...Option[*Shop]) relay.ApplyCursorsFunc[*Shop]) {
		p := relay.New(
			f(
				db,
				WithComputed(&Computed[*Shop]{
					Columns: NewComputedColumns(map[string]string{
						// Define computed Priority column based on shop name
						"Priority": "(CASE WHEN shops.name = 'premium' THEN 1 WHEN shops.name = 'featured' THEN 2 ELSE 3 END)",
					}),
					Scanner: func(_ *gorm.DB) (*ComputedScanner[*Shop], error) {
						nodes := []*Shop{}
						return &ComputedScanner[*Shop]{
							Destination: &nodes,
							Transform: func(computedResults []map[string]any) []cursor.Node[*Shop] {
								return lo.Map(nodes, func(s *Shop, i int) cursor.Node[*Shop] {
									s.Priority = int(computedResults[i]["Priority"].(int32))
									return NewComputedNode(s, computedResults[i])
								})
							},
						}, nil
					},
				}),
			),
			relay.EnsureLimits[*Shop](10, 50),
		)

		// Test pagination with Priority ascending order
		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*Shop]{
			First: lo.ToPtr(3),
			OrderBy: []relay.Order{
				{Field: "Priority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(conn.Edges))
		require.Equal(t, "premium", conn.Edges[0].Node.Name)
		require.Equal(t, "featured", conn.Edges[1].Node.Name)
		require.Equal(t, "shop0", conn.Edges[2].Node.Name)

		// Verify Priority field is populated in returned shops
		require.Equal(t, 1, conn.Edges[0].Node.Priority)
		require.Equal(t, 2, conn.Edges[1].Node.Priority)
		require.Equal(t, 3, conn.Edges[2].Node.Priority)

		// Test pagination using cursor to fetch the next page
		nextConn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*Shop]{
			First: lo.ToPtr(3),
			After: conn.PageInfo.EndCursor,
			OrderBy: []relay.Order{
				{Field: "Priority", Direction: relay.OrderDirectionAsc},
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(nextConn.Edges))
		// Shops with Priority=3 sorted by ID in ascending order
		require.Equal(t, "shop1", nextConn.Edges[0].Node.Name)
		require.Equal(t, "shop2", nextConn.Edges[1].Node.Name)
		require.Equal(t, "shop3", nextConn.Edges[2].Node.Name)
		require.Equal(t, 3, nextConn.Edges[0].Node.Priority)
		require.Equal(t, 3, nextConn.Edges[1].Node.Priority)
		require.Equal(t, 3, nextConn.Edges[2].Node.Priority)

		// no priority order by
		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*Shop]{
			First: lo.ToPtr(3),
			OrderBy: []relay.Order{
				{Field: "ID", Direction: relay.OrderDirectionAsc},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(conn.Edges))
		require.Equal(t, "shop0", conn.Edges[0].Node.Name)
		require.Equal(t, "shop1", conn.Edges[1].Node.Name)
		require.Equal(t, "shop2", conn.Edges[2].Node.Name)
		require.Equal(t, 3, conn.Edges[0].Node.Priority)
		require.Equal(t, 3, conn.Edges[1].Node.Priority)
		require.Equal(t, 3, conn.Edges[2].Node.Priority)
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}
