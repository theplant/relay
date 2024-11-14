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

	testCases := []struct {
		name               string
		limitIfNotSet      int
		maxLimit           int
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
			name:             "Invalid: after < 0",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr(cursor.EncodeOffsetCursor(-1)),
			},
			expectedError: "after < 0",
		},
		{
			name:             "Invalid: before < 0",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(-1)),
			},
			expectedError: "before < 0",
		},
		{
			name:             "Invalid: after <= before",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After:  lo.ToPtr(cursor.EncodeOffsetCursor(1)),
				Before: lo.ToPtr(cursor.EncodeOffsetCursor(1)),
			},
			expectedError: "after >= before",
		},
		{
			name:             "Invalid: invalid after",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				After: lo.ToPtr("invalid"),
			},
			expectedError: `decode offset cursor "invalid"`,
		},
		{
			name:             "Invalid: invalid before",
			limitIfNotSet:    10,
			maxLimit:         20,
			applyCursorsFunc: applyCursorsFunc,
			paginateRequest: &relay.PaginateRequest[*User]{
				Before: lo.ToPtr("invalid"),
			},
			expectedError: `decode offset cursor "invalid"`,
		},
		{
			name:               "Limit if not set",
			limitIfNotSet:      10,
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
			name:             "First 2 after cursor 0",
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			limitIfNotSet:    10,
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
			if tc.expectedPanic != "" {
				require.PanicsWithValue(t, tc.expectedPanic, func() {
					relay.New(
						tc.applyCursorsFunc,
						relay.EnsurePrimaryOrderBy[*User](
							relay.OrderBy{Field: "ID", Desc: false},
							relay.OrderBy{Field: "Age", Desc: true},
						),
						relay.EnsureLimits[*User](tc.maxLimit, tc.limitIfNotSet),
					)
				})
				return
			}

			p := relay.New(
				tc.applyCursorsFunc,
				relay.EnsurePrimaryOrderBy[*User](
					relay.OrderBy{Field: "ID", Desc: false},
					relay.OrderBy{Field: "Age", Desc: true},
				),
				relay.EnsureLimits[*User](tc.maxLimit, tc.limitIfNotSet),
			)
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
			relay.OrderBy{Field: "ID", Desc: false},
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
	require.ErrorContains(t, err, "totalCount is required for fromEnd and nil before")
	require.Nil(t, conn)
}
