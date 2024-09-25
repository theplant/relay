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

	p := relay.New(false, 10, 10, func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
		return nil, nil
	})
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
				cursor.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(
					f(db),
				),
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
				cursor.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(
					func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
						// Set WithContext here
						return f(db.WithContext(ctx))(ctx, req)
					},
				),
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

func TestNodesOnly(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			true,
			10, 10,
			cursor.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(
				f(db),
			),
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

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}

func TestGenericTypeAny(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[any]) {
		t.Run("Correct", func(t *testing.T) {
			p := relay.New(
				false,
				10, 10,
				cursor.PrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false})(
					func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
						// This is a generic(T: any) function, so we need to call db.Model(x)
						return f(db.Model(&User{}))(ctx, req)
					},
				),
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
				cursor.PrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false})(
					func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[any], error) {
						// This is wrong, we need to call db.Model(x) for generic(T: any) function
						return f(db)(ctx, req)
					},
				),
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
				cursor.PrimaryOrderBy[any](relay.OrderBy{Field: "ID", Desc: false})(
					applyCursorsFunc,
				),
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

func TestTotalCountZero(t *testing.T) {
	resetDB(t)
	require.NoError(t, db.Exec("DELETE FROM users").Error)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			false,
			10, 10,
			cursor.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(
				f(db),
			),
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

func generateAESKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func TestMiddleware(t *testing.T) {
	resetDB(t)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			false,
			10, 10,
			cursor.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(
				f(db),
			),
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
			After: lo.ToPtr("invalid%20x"),
		})
		require.ErrorContains(t, err, "invalid after cursor")
		require.Nil(t, resp)
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

	t.Run("AES", func(t *testing.T) {
		encryptionKey, err := generateAESKey(32)
		require.NoError(t, err)

		t.Run("keyset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB) relay.ApplyCursorsFunc[*User] {
				return cursor.AES[*User](encryptionKey)(NewKeysetAdapter[*User](db))
			})
		})

		t.Run("offset", func(t *testing.T) {
			testCase(t, func(db *gorm.DB) relay.ApplyCursorsFunc[*User] {
				return cursor.AES[*User](encryptionKey)(NewOffsetAdapter[*User](db))
			})
		})
	})

	t.Run("MockError", func(t *testing.T) {
		testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
			p := relay.New(
				false,
				10, 10,
				cursor.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(
					func(next relay.ApplyCursorsFunc[*User]) relay.ApplyCursorsFunc[*User] {
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
					}(f(db)),
				),
			)
			resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
				First: lo.ToPtr(10),
			})
			require.ErrorContains(t, err, "mock error")
			require.Nil(t, resp)
		}
		t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
		t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
	})
}

func TestAppendCursorMiddleware(t *testing.T) {
	resetDB(t)

	encryptionKey, err := generateAESKey(32)
	require.NoError(t, err)

	aesMiddleware := cursor.AES[*User](encryptionKey)

	testCase := func(t *testing.T, f func(db *gorm.DB) relay.ApplyCursorsFunc[*User]) {
		p := relay.New(
			false,
			10, 10,
			f(db),
		)
		p = relay.AppendCursorMiddleware(aesMiddleware)(p)                          // test add single middleware
		p = relay.AppendCursorMiddleware(cursor.Base64[*User], aesMiddleware)(p)    // test add multiple middlewares
		p = relay.PrimaryOrderBy[*User](relay.OrderBy{Field: "ID", Desc: false})(p) // test a pagination middleware

		resp, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, 100, resp.PageInfo.TotalCount)
		require.Len(t, resp.Edges, 10)
		require.Equal(t, 0+1, resp.Edges[0].Node.ID)
		require.Equal(t, 9+1, resp.Edges[len(resp.Edges)-1].Node.ID)
		require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
		require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))

		// next page
		resp, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			After: resp.PageInfo.EndCursor,
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, 100, resp.PageInfo.TotalCount)
		require.Len(t, resp.Edges, 10)
		require.Equal(t, 10+1, resp.Edges[0].Node.ID)
		require.Equal(t, 19+1, resp.Edges[len(resp.Edges)-1].Node.ID)
		require.Equal(t, resp.Edges[0].Cursor, *(resp.PageInfo.StartCursor))
		require.Equal(t, resp.Edges[len(resp.Edges)-1].Cursor, *(resp.PageInfo.EndCursor))
	}

	t.Run("keyset", func(t *testing.T) { testCase(t, NewKeysetAdapter) })
	t.Run("offset", func(t *testing.T) { testCase(t, NewOffsetAdapter) })
}