package gormrelay

import (
	"database/sql"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

// TestSplitScan verifies SplitScan behavior in different scenarios
func TestSplitScan(t *testing.T) {
	resetScanTestDB(t)

	// Map the Category columns to a separate destination
	splitSplitter := map[string]func(columnType *sql.ColumnType) any{
		"_category_id_":   func(columnType *sql.ColumnType) any { return columnType.ScanType() },
		"_category_name_": func(columnType *sql.ColumnType) any { return columnType.ScanType() },
		"_a_null_column_": func(columnType *sql.ColumnType) any { return lo.ToPtr(new(string)) },
	}

	t.Run("basic split scan", func(t *testing.T) {
		// Create a test product with a category
		product := &Product{
			Name:       "Test Product",
			Price:      100.0,
			CategoryID: 1,
			Category: &Category{
				ID:   1,
				Name: "Electronics",
			},
		}
		require.NoError(t, db.Create(product.Category).Error)
		require.NoError(t, db.Create(product).Error)

		var result Product
		var splitResults []map[string]any

		tx := db.Table("products").
			Select("categories.id AS _category_id_, products.*, categories.name AS _category_name_, NULL AS _a_null_column_").
			Joins("LEFT JOIN categories ON products.category_id = categories.id").
			Where("products.id = ?", product.ID)

		// Execute SplitScan
		tx = SplitScan(tx, &result, splitSplitter, &splitResults)
		require.NoError(t, tx.Error)

		// Verify main result
		require.Equal(t, product.ID, result.ID)
		require.Equal(t, product.Name, result.Name)
		require.Equal(t, product.Price, result.Price)
		require.Equal(t, product.CategoryID, result.CategoryID)

		// Verify split results
		require.Len(t, splitResults, 1)
		categoryMap := splitResults[0]
		require.Equal(t, product.Category.ID, int(categoryMap["_category_id_"].(int64)))
		require.Equal(t, product.Category.Name, categoryMap["_category_name_"].(string))
		require.Nil(t, categoryMap["_a_null_column_"])
	})

	t.Run("empty result", func(t *testing.T) {
		var result Product
		var splitResults []map[string]any

		tx := db.Table("products").
			Select("products.*, categories.id AS _category_id_, categories.name AS _category_name_").
			Joins("LEFT JOIN categories ON products.category_id = categories.id").
			Where("products.id = ?", 999999) // Non-existent ID

		tx = SplitScan(tx, &result, splitSplitter, &splitResults)

		// Should have RowsAffected = 0
		require.Equal(t, int64(0), tx.RowsAffected)
		// No error should be returned
		require.NoError(t, tx.Error)
		// Split results should be empty
		require.Empty(t, splitResults)
	})

	t.Run("nil splitter", func(t *testing.T) {
		product := &Product{
			Name:  "Simple Product",
			Price: 50.0,
		}
		require.NoError(t, db.Create(product).Error)

		var result Product
		var splitResults []map[string]any

		// Use nil splitter
		tx := db.Table("products").
			Where("products.id = ?", product.ID)

		// Execute SplitScan with nil splitter
		tx = SplitScan(tx, &result, nil, &splitResults)
		require.NoError(t, tx.Error)

		// Verify main result
		require.Equal(t, product.ID, result.ID)
		require.Equal(t, product.Name, result.Name)
		require.Equal(t, product.Price, result.Price)

		require.Equal(t, tx.RowsAffected, int64(len(splitResults)))
	})

	t.Run("multiple rows", func(t *testing.T) {
		// Create test products
		category := &Category{
			ID:   2,
			Name: "Books",
		}
		require.NoError(t, db.Create(category).Error)

		products := []*Product{
			{
				Name:       "Book 1",
				Price:      20.0,
				CategoryID: category.ID,
			},
			{
				Name:       "Book 2",
				Price:      25.0,
				CategoryID: category.ID,
			},
		}

		for _, p := range products {
			require.NoError(t, db.Create(p).Error)
		}

		tx := db.Table("products").
			Select("products.*, categories.id AS _category_id_, categories.name AS _category_name_").
			Joins("LEFT JOIN categories ON products.category_id = categories.id").
			Where("products.category_id = ?", category.ID).
			Order("products.id ASC")

		{
			// Because result is one row, so splitResults is also one row
			var result Product
			var splitResults []map[string]any

			tx := SplitScan(tx, &result, splitSplitter, &splitResults)
			require.NoError(t, tx.Error)

			// Should only have the first product
			require.Equal(t, products[0].ID, result.ID)
			require.Equal(t, products[0].Name, result.Name)

			// Only one split result
			require.Len(t, splitResults, 1)
			require.Equal(t, category.ID, int(splitResults[0]["_category_id_"].(int64)))
			require.Equal(t, category.Name, splitResults[0]["_category_name_"])
		}
		{
			// Because result is slice, so splitResults is also slice
			var result []Product
			var splitResults []map[string]any

			tx := SplitScan(tx, &result, splitSplitter, &splitResults)
			require.NoError(t, tx.Error)

			// Should have all products
			require.Equal(t, len(products), len(result))
			require.Equal(t, products[0].ID, result[0].ID)
			require.Equal(t, products[0].Name, result[0].Name)

			// Should have all split results
			require.Equal(t, len(products), len(splitResults))
		}
	})

	t.Run("with error handling", func(t *testing.T) {
		var result Product
		var splitResults []map[string]any

		// Create an intentional error by using a non-existent table
		tx := db.Table("non_existent_table").
			Select("*")

		tx = SplitScan(tx, &result, splitSplitter, &splitResults)

		// Should have an error
		require.Error(t, tx.Error)
		// splitResults should not be populated on error
		require.Empty(t, splitResults)
	})
}

// Helper structs for testing
type Product struct {
	ID         int       `gorm:"primarykey;not null;" json:"id"`
	Name       string    `gorm:"not null;" json:"name"`
	Price      float64   `gorm:"not null;" json:"price"`
	CategoryID int       `gorm:"index;" json:"categoryId"`
	Category   *Category `gorm:"-" json:"category"`
}

type Category struct {
	ID   int    `gorm:"primarykey;not null;" json:"id"`
	Name string `gorm:"not null;" json:"name"`
}

func resetScanTestDB(t *testing.T) {
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS products").Error)
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS categories").Error)
	require.NoError(t, db.AutoMigrate(&Product{}, &Category{}))
}
