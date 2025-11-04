package gormfilter_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
	"github.com/theplant/relay/filter"
	"github.com/theplant/relay/filter/gormfilter"
	"github.com/theplant/relay/gormrelay"
)

type Product struct {
	gorm.Model

	Name       string `gorm:"not null" json:"name"`
	Code       string `gorm:"not null" json:"code"`
	CategoryID string `gorm:"not null" json:"categoryID"`

	Snapshot datatypes.JSONType[*Product] `gorm:"not null;default:'{}'" json:"snapshot"`
}

type ProductFilter struct {
	Not        *ProductFilter
	And        []*ProductFilter
	Or         []*ProductFilter
	Name       *filter.String
	Code       *filter.String
	CategoryID *filter.String
}

func TestProductSnapshotWithRelay(t *testing.T) {
	err := db.Migrator().DropTable(&Product{})
	require.NoError(t, err)
	err = db.AutoMigrate(&Product{})
	require.NoError(t, err)

	products := []*Product{
		{
			Name:       "Alpha",
			Code:       "A001",
			CategoryID: "cat-1",
			Snapshot: datatypes.NewJSONType(&Product{
				Name:       "Alpha Old",
				Code:       "A001-OLD",
				CategoryID: "cat-1",
			}),
		},
		{
			Name:       "Beta",
			Code:       "B001",
			CategoryID: "cat-2",
			Snapshot: datatypes.NewJSONType(&Product{
				Name:       "Beta Old",
				Code:       "B001-OLD",
				CategoryID: "cat-2",
			}),
		},
		{
			Name:       "Gamma",
			Code:       "C001",
			CategoryID: "cat-1",
			Snapshot: datatypes.NewJSONType(&Product{
				Name:       "Gamma Old",
				Code:       "C001-OLD",
				CategoryID: "cat-1",
			}),
		},
		{
			Name:       "Delta",
			Code:       "D001",
			CategoryID: "cat-3",
			Snapshot: datatypes.NewJSONType(&Product{
				Name:       "Delta Old",
				Code:       "D001-OLD",
				CategoryID: "cat-3",
			}),
		},
	}
	require.NoError(t, db.Create(products).Error)

	// Define computed columns for snapshot fields (reusable definition)
	snapshotColumns := map[string]string{
		"Name":       `"snapshot"->>'name'`,
		"Code":       `"snapshot"->>'code'`,
		"CategoryID": `"snapshot"->>'categoryID'`,
	}

	// Create field column hook from computed columns (reuse the definitions)
	snapshotFieldColumnHook := func(next gormfilter.FieldColumnFunc) gormfilter.FieldColumnFunc {
		return func(input *gormfilter.FieldColumnInput) (*gormfilter.FieldColumnOutput, error) {
			// Reuse computed columns definition
			if col, ok := snapshotColumns[input.FieldName]; ok {
				var column any = clause.Column{Name: col, Raw: true}
				if input.Fold {
					column = clause.Expr{SQL: "LOWER(?)", Vars: []any{column}}
				}
				return &gormfilter.FieldColumnOutput{
					Column: column,
				}, nil
			}
			return next(input)
		}
	}

	// Define computed for gormrelay
	computed := &gormrelay.Computed[*Product]{
		Columns: gormrelay.NewComputedColumns(snapshotColumns),
		Scanner: gormrelay.NewComputedScanner[*Product],
	}

	// Create paginator with keyset pagination
	p := relay.New(
		cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*Product], error) {
			return gormrelay.NewKeysetAdapter(
				db.WithContext(ctx).Scopes(gormfilter.Scope(
					&ProductFilter{
						Or: []*ProductFilter{
							{Name: &filter.String{Contains: lo.ToPtr("oLd"), Fold: true}},
							{CategoryID: &filter.String{Eq: lo.ToPtr("cat-1")}},
						},
					},
					gormfilter.WithFieldColumnHook(snapshotFieldColumnHook),
				)),
				gormrelay.WithComputed(computed),
			)(ctx, req)
		}),
		relay.EnsureLimits[*Product](2, 100),
		relay.EnsurePrimaryOrderBy[*Product](
			relay.Order{Field: "ID", Direction: relay.OrderDirectionAsc},
		),
	)

	// First page
	conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*Product]{
		First: lo.ToPtr(2),
	})
	require.NoError(t, err)

	// Should match all 4 products:
	// - Alpha: Snapshot.Name contains "Old" and CategoryID = "cat-1"
	// - Beta: Snapshot.Name contains "Old"
	// - Gamma: Snapshot.Name contains "Old" and CategoryID = "cat-1"
	// - Delta: Snapshot.Name contains "Old"
	require.Equal(t, 4, *conn.TotalCount)
	require.Len(t, conn.Edges, 2)

	// Verify first page
	assert.Equal(t, "Alpha", conn.Edges[0].Node.Name)
	assert.Contains(t, conn.Edges[0].Node.Snapshot.Data().Name, "Old")
	assert.Equal(t, "Beta", conn.Edges[1].Node.Name)
	assert.Contains(t, conn.Edges[1].Node.Snapshot.Data().Name, "Old")

	require.True(t, conn.PageInfo.HasNextPage)
	require.False(t, conn.PageInfo.HasPreviousPage)

	// Second page
	conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*Product]{
		First: lo.ToPtr(2),
		After: conn.PageInfo.EndCursor,
	})
	require.NoError(t, err)

	require.Equal(t, 4, *conn.TotalCount)
	require.Len(t, conn.Edges, 2)

	// Verify second page
	assert.Equal(t, "Gamma", conn.Edges[0].Node.Name)
	assert.Contains(t, conn.Edges[0].Node.Snapshot.Data().Name, "Old")
	assert.Equal(t, "Delta", conn.Edges[1].Node.Name)
	assert.Contains(t, conn.Edges[1].Node.Snapshot.Data().Name, "Old")

	require.False(t, conn.PageInfo.HasNextPage)
	require.True(t, conn.PageInfo.HasPreviousPage)
}
