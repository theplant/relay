package gormrelay

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
)

func TestOffsetCursor(t *testing.T) {
	resetDB(t)

	applyCursorsFunc := NewOffsetAdapter[*User](db)

	const withoutEnsureLimits = -404

	testCases := []struct {
		name               string
		defaultLimit       int // -404 indicates without EnsureLimits
		maxLimit           int // -404 indicates without EnsureLimits
		applyCursorsFunc   relay.ApplyCursorsFunc[*User]
		paginateRequest    *relay.PaginateRequest[*User]
		expectedEdgesLen   int
		expectedFirstKey   int
		expectedLastKey    int
		expectedTotalCount *int
		expectedPageInfo   *relay.PageInfo
		expectedError      string
		expectedPanic      string
	}{
		{
			name:             "Invalid: Both First and Last",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(5),
				Last:  lo.ToPtr(5),
			},
			expectedError: "first and last cannot be used together",
		},
		{
			name:             "Negative First",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(-5),
			},
			expectedEdgesLen:   10,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    9 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(9)),
			},
		},
		{
			name:             "Invalid: Negative First without EnsureLimits",
			defaultLimit:     withoutEnsureLimits,
			maxLimit:         withoutEnsureLimits,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(-5),
			},
			expectedError: "first must be a non-negative integer",
		},
		{
			name:             "Negative Last",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(-5),
			},
			expectedEdgesLen:   10,
			expectedFirstKey:   90 + 1,
			expectedLastKey:    99 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(90)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(99)),
			},
		},
		{
			name:             "Invalid: Negative Last without EnsureLimits",
			defaultLimit:     withoutEnsureLimits,
			maxLimit:         withoutEnsureLimits,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(-5),
			},
			expectedError: "last must be a non-negative integer",
		},
		{
			name:             "Invalid defaultLimit",
			defaultLimit:     -1,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedPanic:    "defaultLimit cannot be negative",
		},
		{
			name:             "Invalid: maxLimit < defaultLimit",
			defaultLimit:     10,
			maxLimit:         8,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedPanic:    "maxLimit must be greater than or equal to defaultLimit",
		},
		{
			name:             "Invalid: No applyCursorsFunc",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: nil, // No ApplyCursorsFunc provided
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedPanic:    "applyCursorsFunc must be set",
		},
		{
			name:             "first > maxLimit",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(21),
			},
			expectedEdgesLen:   20,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    19 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(19)),
			},
		},
		{
			name:             "last > maxLimit",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(21),
			},
			expectedEdgesLen:   20,
			expectedFirstKey:   80 + 1,
			expectedLastKey:    99 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(80)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(99)),
			},
		},
		{
			name:             "Invalid: after < 0",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(cursor.EncodeOffsetCursor(-1)),
			},
			expectedError: "invalid pagination: after cursor must be non-negative",
		},
		{
			name:             "Invalid: before < 0",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(-1)),
			},
			expectedError: "invalid pagination: before cursor must be non-negative",
		},
		{
			name:             "Invalid: after <= before",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After:  lo.ToPtr(cursor.EncodeOffsetCursor(1)),
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(1)),
			},
			expectedError: "invalid pagination: after cursor must be less than before cursor",
		},
		{
			name:             "Invalid: invalid after",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr("invalid"),
			},
			expectedError: `invalid offset cursor "invalid"`,
		},
		{
			name:             "Invalid: invalid before",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr("invalid"),
			},
			expectedError: `invalid offset cursor "invalid"`,
		},
		{
			name:               "Limit if not set",
			defaultLimit:       10,
			maxLimit:           20,
			applyCursorsFunc:   applyCursorsFunc,
			paginateRequest:    &relay.PaginateRequest[*User]{},
			expectedEdgesLen:   10,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    9 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(9)),
			},
		},
		{
			name:             "Invalid: Limit if not set without EnsureLimits",
			defaultLimit:     withoutEnsureLimits,
			maxLimit:         withoutEnsureLimits,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest:  &relay.PaginateRequest[*User]{},
			expectedError:    "first or last must be set",
		},
		{
			name:             "First 2 after cursor 0",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				First: lo.ToPtr(2),
			},
			expectedEdgesLen:   2,
			expectedFirstKey:   1 + 1,
			expectedLastKey:    2 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(1)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(2)),
			},
		},
		{
			name:             "First 2 without after cursor",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(2),
			},
			expectedEdgesLen:   2,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    1 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(1)),
			},
		},
		{
			name:             "Last 2 before cursor 18",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(18)),
				Last:   lo.ToPtr(2),
			},
			expectedEdgesLen:   2,
			expectedFirstKey:   16 + 1,
			expectedLastKey:    17 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(16)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(17)),
			},
		},
		{
			name:             "Last 10 without before cursor",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(10),
			},
			expectedEdgesLen:   10,
			expectedFirstKey:   90 + 1,
			expectedLastKey:    99 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(90)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(99)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 8, First 5",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After:  lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(8)),
				First:  lo.ToPtr(5),
			},
			expectedEdgesLen:   5,
			expectedFirstKey:   1 + 1,
			expectedLastKey:    5 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(1)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(5)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 4, First 8",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After:  lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(4)),
				First:  lo.ToPtr(8),
			},
			expectedEdgesLen:   3,
			expectedFirstKey:   1 + 1,
			expectedLastKey:    3 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(1)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(3)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 8, Last 5",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After:  lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(8)),
				Last:   lo.ToPtr(5),
			},
			expectedEdgesLen:   5,
			expectedFirstKey:   3 + 1,
			expectedLastKey:    7 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(3)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(7)),
			},
		},
		{
			name:             "After cursor 0, Before cursor 4, Last 8",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After:  lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(4)),
				Last:   lo.ToPtr(8),
			},
			expectedEdgesLen:   3,
			expectedFirstKey:   1 + 1,
			expectedLastKey:    3 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(1)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(3)),
			},
		},
		{
			name:             "After cursor 99",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(cursor.EncodeOffsetCursor(99)),
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
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(0)),
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
			defaultLimit:     10,
			maxLimit:         300,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				First: lo.ToPtr(200),
			},
			expectedEdgesLen:   100,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    99 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(99)),
			},
		},
		{
			name:             "Last 200",
			defaultLimit:     10,
			maxLimit:         300,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Last: lo.ToPtr(200),
			},
			expectedEdgesLen:   100,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    99 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(99)),
			},
		},
		{
			name:             "First 0",
			defaultLimit:     10,
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
			name:             "First 0 without EnsureLimits",
			defaultLimit:     withoutEnsureLimits,
			maxLimit:         withoutEnsureLimits,
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
			defaultLimit:     10,
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
			name:             "Last 0 without EnsureLimits",
			defaultLimit:     withoutEnsureLimits,
			maxLimit:         withoutEnsureLimits,
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
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(cursor.EncodeOffsetCursor(95)),
				First: lo.ToPtr(10),
			},
			expectedEdgesLen:   4,
			expectedFirstKey:   96 + 1,
			expectedLastKey:    99 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: true,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(96)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(99)),
			},
		},
		{
			name:             "Before cursor 4, Last 10",
			defaultLimit:     10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(4)),
				Last:   lo.ToPtr(10),
			},
			expectedEdgesLen:   4,
			expectedFirstKey:   0 + 1,
			expectedLastKey:    3 + 1,
			expectedTotalCount: lo.ToPtr(100),
			expectedPageInfo: &relay.PageInfo{
				HasNextPage:     true,
				HasPreviousPage: false,
				StartCursor:     lo.ToPtr(cursor.EncodeOffsetCursor(0)),
				EndCursor:       lo.ToPtr(cursor.EncodeOffsetCursor(3)),
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			newPaginator := func() relay.Paginator[*User] {
				mws := []relay.PaginatorMiddleware[*User]{
					relay.EnsurePrimaryOrderBy[*User](
						relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
						relay.Order{Field: "Age", Direction: relay.OrderDirectionDesc},
					),
				}
				if tc.defaultLimit != withoutEnsureLimits {
					mws = append(mws, relay.EnsureLimits[*User](tc.defaultLimit, tc.maxLimit))
				}
				return relay.New(tc.applyCursorsFunc, mws...)
			}
			if tc.expectedPanic != "" {
				require.PanicsWithValue(t, tc.expectedPanic, func() {
					_ = newPaginator()
				})
				return
			}

			p := newPaginator()
			conn, err := p.Paginate(context.Background(), tc.paginateRequest)

			if tc.expectedError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedError)
				return
			}

			require.NoError(t, err)
			require.Len(t, conn.Edges, tc.expectedEdgesLen)

			if tc.expectedEdgesLen > 0 {
				require.Equal(t, tc.expectedFirstKey, conn.Edges[0].Node.ID)
				require.Equal(t, tc.expectedLastKey, conn.Edges[len(conn.Edges)-1].Node.ID)
			}

			require.Equal(t, tc.expectedTotalCount, conn.TotalCount)
			require.Equal(t, tc.expectedPageInfo, conn.PageInfo)
		})
	}
}

func TestOffsetWithLastAndNilBeforeIfSkipTotalCount(t *testing.T) {
	resetDB(t)

	p := relay.New(
		cursor.NewOffsetAdapter(NewOffsetFinder[*User](db)),
		relay.EnsurePrimaryOrderBy[*User](
			relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
		),
		relay.EnsureLimits[*User](10, 10),
	)
	conn, err := p.Paginate(
		relay.WithSkip(context.Background(), relay.Skip{
			TotalCount: true,
		}),
		&relay.PaginateRequest[*User]{
			Last: lo.ToPtr(10),
		},
	)
	require.ErrorContains(t, err, "totalCount is required for pagination from end when before cursor is not provided")
	require.Nil(t, conn)
}
