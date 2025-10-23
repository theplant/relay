package relay_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theplant/testenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
	"github.com/theplant/relay/filter"
	"github.com/theplant/relay/filter/gormfilter"
	relayv1 "github.com/theplant/relay/gen/relay/v1"
	testdatav1 "github.com/theplant/relay/gen/testdata/v1"
	"github.com/theplant/relay/gormrelay"
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

type Category struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"index;not null" json:"createdAt"`
	UpdatedAt time.Time `gorm:"index;not null" json:"updatedAt"`
	Name      string    `gorm:"not null" json:"name"`
	Code      string    `gorm:"not null" json:"code"`
}

type Product struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time `gorm:"index;not null" json:"createdAt"`
	UpdatedAt  time.Time `gorm:"index;not null" json:"updatedAt"`
	Name       string    `gorm:"not null" json:"name"`
	Code       string    `gorm:"not null" json:"code"`
	Status     string    `gorm:"not null" json:"status"`
	CategoryId string    `gorm:"column:category_id;not null" json:"categoryId"`
	Category   *Category `json:"category"`
}

func parseProductStatus(status string) testdatav1.ProductStatus {
	switch status {
	case "DRAFT":
		return testdatav1.ProductStatus_PRODUCT_STATUS_DRAFT
	case "PENDING_REVIEW":
		return testdatav1.ProductStatus_PRODUCT_STATUS_PENDING_REVIEW
	case "REJECTED":
		return testdatav1.ProductStatus_PRODUCT_STATUS_REJECTED
	case "APPROVED":
		return testdatav1.ProductStatus_PRODUCT_STATUS_APPROVED
	case "PUBLISHED":
		return testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED
	case "UNPUBLISHED":
		return testdatav1.ProductStatus_PRODUCT_STATUS_UNPUBLISHED
	default:
		return testdatav1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED
	}
}

func (c *Category) ToProto() *testdatav1.Category {
	if c == nil {
		return nil
	}
	return &testdatav1.Category{
		Id:        c.ID,
		CreatedAt: timestamppb.New(c.CreatedAt),
		UpdatedAt: timestamppb.New(c.UpdatedAt),
		Name:      c.Name,
		Code:      c.Code,
	}
}

func (p *Product) ToProto() *testdatav1.Product {
	if p == nil {
		return nil
	}
	proto := &testdatav1.Product{
		Id:         p.ID,
		CreatedAt:  timestamppb.New(p.CreatedAt),
		UpdatedAt:  timestamppb.New(p.UpdatedAt),
		Name:       p.Name,
		Code:       p.Code,
		Status:     parseProductStatus(p.Status),
		CategoryId: p.CategoryId,
		Category:   p.Category.ToProto(),
	}
	return proto
}

type ProductService struct {
	db *gorm.DB
}

func NewProductService(db *gorm.DB) *ProductService {
	return &ProductService{db: db}
}

func (s *ProductService) ListProducts(ctx context.Context, req *testdatav1.ListProductsRequest) (*testdatav1.ListProductsResponse, error) {
	orderBy, err := relay.ParseProtoOrderBy(req.OrderBy, []relay.OrderBy{
		{Field: "CreatedAt", Desc: true},
	})
	if err != nil {
		return nil, err
	}

	filterMap, err := filter.ParseProtoFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	applyCursorsFunc := cursor.Base64(
		gormrelay.NewKeysetAdapter[*Product](
			s.db.WithContext(ctx).Scopes(gormfilter.Scope(filterMap)),
		),
	)

	paginator := relay.New(
		applyCursorsFunc,
		relay.EnsurePrimaryOrderBy[*Product](
			relay.OrderBy{Field: "ID", Desc: false},
		),
		relay.EnsureLimits[*Product](10, 100),
	)

	paginateReq := &relay.PaginateRequest[*Product]{
		OrderBys: orderBy,
	}

	if req.Pagination != nil {
		paginateReq.After = req.Pagination.After
		if req.Pagination.First != nil {
			paginateReq.First = lo.ToPtr(int(*req.Pagination.First))
		}
		paginateReq.Before = req.Pagination.Before
		if req.Pagination.Last != nil {
			paginateReq.Last = lo.ToPtr(int(*req.Pagination.Last))
		}
	}

	conn, err := paginator.Paginate(ctx, paginateReq)
	if err != nil {
		return nil, err
	}

	edges := make([]*testdatav1.ProductEdge, 0, len(conn.Edges))
	for _, edge := range conn.Edges {
		edges = append(edges, &testdatav1.ProductEdge{
			Node:   edge.Node.ToProto(),
			Cursor: string(edge.Cursor),
		})
	}

	resp := &testdatav1.ListProductsResponse{
		Edges: edges,
		PageInfo: &relayv1.PageInfo{
			HasNextPage: conn.PageInfo.HasNextPage,
			HasPrevPage: conn.PageInfo.HasPreviousPage,
			StartCursor: conn.PageInfo.StartCursor,
			EndCursor:   conn.PageInfo.EndCursor,
		},
	}

	if conn.TotalCount != nil {
		resp.TotalCount = lo.ToPtr(int64(*conn.TotalCount))
	}

	return resp, nil
}

func resetDB(t *testing.T) {
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS products").Error)
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS categories").Error)
	require.NoError(t, db.AutoMigrate(&Category{}, &Product{}))

	categories := []*Category{
		{
			ID:        "cat-electronics",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Electronics",
			Code:      "ELECTRONICS",
		},
		{
			ID:        "cat-books",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Books",
			Code:      "BOOKS",
		},
		{
			ID:        "cat-clothing",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Name:      "Clothing",
			Code:      "CLOTHING",
		},
	}
	err := db.Session(&gorm.Session{Logger: logger.Discard}).Create(categories).Error
	require.NoError(t, err)

	products := []*Product{}
	for i := 0; i < 30; i++ {
		var status string
		switch i % 3 {
		case 0:
			status = "PUBLISHED"
		case 1:
			status = "PENDING_REVIEW"
		default:
			status = "DRAFT"
		}

		var categoryId string
		switch i % 3 {
		case 0:
			categoryId = "cat-electronics"
		case 1:
			categoryId = "cat-books"
		default:
			categoryId = "cat-clothing"
		}

		products = append(products, &Product{
			ID:         fmt.Sprintf("product-%03d", i),
			CreatedAt:  time.Now().Add(time.Duration(-i) * time.Hour),
			UpdatedAt:  time.Now().Add(time.Duration(-i) * time.Minute),
			Name:       fmt.Sprintf("Product %d", i),
			Code:       fmt.Sprintf("CODE-%03d", i),
			Status:     status,
			CategoryId: categoryId,
		})
	}
	err = db.Session(&gorm.Session{Logger: logger.Discard}).Create(products).Error
	require.NoError(t, err)
}

func TestParseProtoEnum(t *testing.T) {
	tests := []struct {
		name      string
		enum      testdatav1.ProductStatus
		want      string
		wantError bool
	}{
		{
			name: "valid enum - draft",
			enum: testdatav1.ProductStatus_PRODUCT_STATUS_DRAFT,
			want: "DRAFT",
		},
		{
			name: "valid enum - pending review",
			enum: testdatav1.ProductStatus_PRODUCT_STATUS_PENDING_REVIEW,
			want: "PENDING_REVIEW",
		},
		{
			name: "valid enum - published",
			enum: testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED,
			want: "PUBLISHED",
		},
		{
			name:      "unspecified enum value",
			enum:      testdatav1.ProductStatus_PRODUCT_STATUS_UNSPECIFIED,
			wantError: true,
		},
		{
			name:      "invalid enum value",
			enum:      testdatav1.ProductStatus(99999),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := relay.ParseProtoEnum(tt.enum)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseProtoOrderField(t *testing.T) {
	tests := []struct {
		name      string
		field     testdatav1.ProductOrderField
		want      string
		wantError bool
	}{
		{
			name:  "order by id",
			field: testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_ID,
			want:  "Id",
		},
		{
			name:  "order by created_at",
			field: testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_CREATED_AT,
			want:  "CreatedAt",
		},
		{
			name:  "order by updated_at",
			field: testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_UPDATED_AT,
			want:  "UpdatedAt",
		},
		{
			name:      "unspecified order field",
			field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_UNSPECIFIED,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := relay.ParseProtoOrderField(tt.field)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseProtoOrderBy(t *testing.T) {
	tests := []struct {
		name           string
		orderBy        []*testdatav1.ProductOrder
		defaultOrderBy []relay.OrderBy
		want           []relay.OrderBy
		wantError      bool
	}{
		{
			name: "single order - created_at desc",
			orderBy: []*testdatav1.ProductOrder{
				{
					Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_CREATED_AT,
					Direction: relayv1.OrderDirection_ORDER_DIRECTION_DESC,
				},
			},
			want: []relay.OrderBy{
				{Field: "CreatedAt", Desc: true},
			},
		},
		{
			name: "single order - id asc",
			orderBy: []*testdatav1.ProductOrder{
				{
					Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_ID,
					Direction: relayv1.OrderDirection_ORDER_DIRECTION_ASC,
				},
			},
			want: []relay.OrderBy{
				{Field: "Id", Desc: false},
			},
		},
		{
			name: "multiple orders",
			orderBy: []*testdatav1.ProductOrder{
				{
					Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_CREATED_AT,
					Direction: relayv1.OrderDirection_ORDER_DIRECTION_DESC,
				},
				{
					Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_ID,
					Direction: relayv1.OrderDirection_ORDER_DIRECTION_ASC,
				},
			},
			want: []relay.OrderBy{
				{Field: "CreatedAt", Desc: true},
				{Field: "Id", Desc: false},
			},
		},
		{
			name:    "empty order - returns default",
			orderBy: []*testdatav1.ProductOrder{},
			defaultOrderBy: []relay.OrderBy{
				{Field: "CreatedAt", Desc: true},
			},
			want: []relay.OrderBy{
				{Field: "CreatedAt", Desc: true},
			},
		},
		{
			name: "nil order - returns default",
			defaultOrderBy: []relay.OrderBy{
				{Field: "CreatedAt", Desc: false},
			},
			want: []relay.OrderBy{
				{Field: "CreatedAt", Desc: false},
			},
		},
		{
			name: "unspecified field - error",
			orderBy: []*testdatav1.ProductOrder{
				{
					Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_UNSPECIFIED,
					Direction: relayv1.OrderDirection_ORDER_DIRECTION_DESC,
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := relay.ParseProtoOrderBy(tt.orderBy, tt.defaultOrderBy)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtoOrderInterface(t *testing.T) {
	var _ relay.ProtoOrder[testdatav1.ProductOrderField] = (*testdatav1.ProductOrder)(nil)

	order := &testdatav1.ProductOrder{
		Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_CREATED_AT,
		Direction: relayv1.OrderDirection_ORDER_DIRECTION_DESC,
	}

	assert.Equal(t, testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_CREATED_AT, order.GetField())
	assert.Equal(t, relayv1.OrderDirection_ORDER_DIRECTION_DESC, order.GetDirection())
}

func TestProductService_ListProducts(t *testing.T) {
	resetDB(t)

	service := NewProductService(db)

	t.Run("list first page", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(10)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Edges, 10)
		assert.True(t, resp.PageInfo.HasNextPage)
		assert.False(t, resp.PageInfo.HasPrevPage)
		assert.NotNil(t, resp.PageInfo.StartCursor)
		assert.NotNil(t, resp.PageInfo.EndCursor)
	})

	t.Run("list with order by created_at asc", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			OrderBy: []*testdatav1.ProductOrder{
				{
					Field:     testdatav1.ProductOrderField_PRODUCT_ORDER_FIELD_CREATED_AT,
					Direction: relayv1.OrderDirection_ORDER_DIRECTION_ASC,
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(5)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Len(t, resp.Edges, 5)
		assert.Equal(t, "product-029", resp.Edges[0].Node.Id)
		assert.Equal(t, "product-028", resp.Edges[1].Node.Id)
	})

	t.Run("list with filter by status", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Status: &testdatav1.ProductFilter_StatusFilter{
					Eq: lo.ToPtr(testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED),
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 10, len(resp.Edges))

		for _, edge := range resp.Edges {
			assert.Equal(t, testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED, edge.Node.Status)
		}
	})

	t.Run("list with filter by name contains", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Name: &testdatav1.ProductFilter_NameFilter{
					Contains: lo.ToPtr("Product 1"),
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		for _, edge := range resp.Edges {
			assert.Contains(t, edge.Node.Name, "Product 1")
		}
	})

	t.Run("pagination - forward", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(5)),
			},
		}

		resp1, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp1)
		assert.Len(t, resp1.Edges, 5)
		assert.True(t, resp1.PageInfo.HasNextPage)

		req.Pagination.After = resp1.PageInfo.EndCursor
		resp2, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp2)
		assert.Len(t, resp2.Edges, 5)
		assert.True(t, resp2.PageInfo.HasNextPage)
		assert.True(t, resp2.PageInfo.HasPrevPage)

		assert.NotEqual(t, resp1.Edges[0].Node.Id, resp2.Edges[0].Node.Id)
	})

	t.Run("pagination - backward", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Pagination: &relayv1.Pagination{
				Last: lo.ToPtr(int64(5)),
			},
		}

		resp1, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp1)
		assert.Len(t, resp1.Edges, 5)
		assert.True(t, resp1.PageInfo.HasPrevPage)

		req.Pagination.Before = resp1.PageInfo.StartCursor
		resp2, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp2)
		assert.Len(t, resp2.Edges, 5)
		assert.True(t, resp2.PageInfo.HasPrevPage)
		assert.True(t, resp2.PageInfo.HasNextPage)

		assert.NotEqual(t, resp1.Edges[0].Node.Id, resp2.Edges[0].Node.Id)
	})

	t.Run("filter with multiple status values", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Status: &testdatav1.ProductFilter_StatusFilter{
					In: []testdatav1.ProductStatus{
						testdatav1.ProductStatus_PRODUCT_STATUS_DRAFT,
						testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED,
					},
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Greater(t, len(resp.Edges), 0)

		for _, edge := range resp.Edges {
			status := edge.Node.Status
			assert.True(t, status == testdatav1.ProductStatus_PRODUCT_STATUS_DRAFT ||
				status == testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED)
		}
	})

	t.Run("filter by category_id eq", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				CategoryId: &testdatav1.ProductFilter_CategoryIDFilter{
					Eq: lo.ToPtr("cat-electronics"),
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 10, len(resp.Edges))

		for _, edge := range resp.Edges {
			assert.Equal(t, "cat-electronics", edge.Node.CategoryId)
		}
	})

	t.Run("filter by category_id in", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				CategoryId: &testdatav1.ProductFilter_CategoryIDFilter{
					In: []string{"cat-books", "cat-clothing"},
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 20, len(resp.Edges))

		for _, edge := range resp.Edges {
			categoryID := edge.Node.CategoryId
			assert.True(t, categoryID == "cat-books" || categoryID == "cat-clothing")
		}
	})

	t.Run("filter by category name", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Category: &testdatav1.CategoryFilter{
					Name: &testdatav1.CategoryFilter_NameFilter{
						Eq: lo.ToPtr("Electronics"),
					},
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 10, len(resp.Edges))

		for _, edge := range resp.Edges {
			assert.Equal(t, "cat-electronics", edge.Node.CategoryId)
		}
	})

	t.Run("filter by category code contains", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Category: &testdatav1.CategoryFilter{
					Code: &testdatav1.CategoryFilter_CodeFilter{
						In: []string{"BOOKS", "CLOTHING"},
					},
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 20, len(resp.Edges))

		for _, edge := range resp.Edges {
			categoryID := edge.Node.CategoryId
			assert.True(t, categoryID == "cat-books" || categoryID == "cat-clothing")
		}
	})

	t.Run("filter by status and category", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Status: &testdatav1.ProductFilter_StatusFilter{
					Eq: lo.ToPtr(testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED),
				},
				Category: &testdatav1.CategoryFilter{
					Name: &testdatav1.CategoryFilter_NameFilter{
						Eq: lo.ToPtr("Electronics"),
					},
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		for _, edge := range resp.Edges {
			assert.Equal(t, testdatav1.ProductStatus_PRODUCT_STATUS_PUBLISHED, edge.Node.Status)
			assert.Equal(t, "cat-electronics", edge.Node.CategoryId)
		}
	})

	t.Run("filter with category logical operators", func(t *testing.T) {
		req := &testdatav1.ListProductsRequest{
			Filter: &testdatav1.ProductFilter{
				Category: &testdatav1.CategoryFilter{
					Or: []*testdatav1.CategoryFilter{
						{
							Name: &testdatav1.CategoryFilter_NameFilter{
								Eq: lo.ToPtr("Electronics"),
							},
						},
						{
							Code: &testdatav1.CategoryFilter_CodeFilter{
								Eq: lo.ToPtr("BOOKS"),
							},
						},
					},
				},
			},
			Pagination: &relayv1.Pagination{
				First: lo.ToPtr(int64(100)),
			},
		}

		resp, err := service.ListProducts(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 20, len(resp.Edges))

		for _, edge := range resp.Edges {
			categoryID := edge.Node.CategoryId
			assert.True(t, categoryID == "cat-electronics" || categoryID == "cat-books")
		}
	})
}
