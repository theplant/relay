package gormrelay

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// testModel is used for testing SQL generation
type testModel struct {
	ID        uint
	Name      string
	Email     string
	ProfileID uint
	Profile   *testProfileModel
}

// testProfileModel as a nested model
type testProfileModel struct {
	ID      uint
	Avatar  string
	Address string
}

// TestAppendSelect verifies AppendSelect behavior in different scenarios
func TestAppendSelect(t *testing.T) {
	t.Run("without existing select clause", func(t *testing.T) {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Scopes(AppendSelect(
				clause.Column{Name: "id", Alias: "user_id"},
				clause.Column{Name: "name", Alias: "user_name"},
			)).Find(&testModel{})
		})

		expectedSQL := `SELECT *,"id" AS "user_id","name" AS "user_name" FROM "test_models"`
		require.Equal(t, expectedSQL, sql)
	})

	t.Run("with existing select clause", func(t *testing.T) {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "email").Scopes(AppendSelect(
				clause.Column{Name: "name", Alias: "user_name"},
			)).Find(&testModel{})
		})

		expectedSQL := `SELECT "id","email","name" AS "user_name" FROM "test_models"`
		require.Equal(t, expectedSQL, sql)
	})

	t.Run("with nested model joins", func(t *testing.T) {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Joins("Profile").
				Scopes(AppendSelect(
					clause.Column{Table: "test_profile_models", Name: "avatar", Alias: "profile_avatar"},
					clause.Column{Table: "test_profile_models", Name: "address", Alias: "profile_address"},
				)).Find(&testModel{})
		})
		require.Equal(t, sql, `SELECT "test_models"."id","test_models"."name","test_models"."email","test_models"."profile_id","Profile"."id" AS "Profile__id","Profile"."avatar" AS "Profile__avatar","Profile"."address" AS "Profile__address","test_profile_models"."avatar" AS "profile_avatar","test_profile_models"."address" AS "profile_address" FROM "test_models" LEFT JOIN "test_profile_models" "Profile" ON "test_models"."profile_id" = "Profile"."id"`)
	})

	t.Run("multiple AppendSelect calls", func(t *testing.T) {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.
				Scopes(AppendSelect(clause.Column{Name: "id", Alias: "user_id"})).
				Scopes(AppendSelect(clause.Column{Name: "name", Alias: "user_name"})).
				Scopes(AppendSelect(clause.Column{Name: "email", Alias: "user_email"})).
				Find(&testModel{})
		})
		require.Equal(t, sql, `SELECT *,"email" AS "user_email","name" AS "user_name","id" AS "user_id" FROM "test_models"`)
	})

	t.Run("multiple Find", func(t *testing.T) {
		queryDB := db.Scopes(AppendSelect(
			clause.Column{Name: "id", Alias: "user_id"},
			clause.Column{Name: "name", Alias: "user_name"},
		))

		require.Equal(t, `SELECT *,"id" AS "user_id","name" AS "user_name" FROM "test_models"`, queryDB.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&testModel{})
		}))
		require.Equal(t, `SELECT *,"id" AS "user_id","name" AS "user_name" FROM "test_models"`, queryDB.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&testModel{})
		}))
	})

	t.Run("session isolation", func(t *testing.T) {
		sessionDB := db.Session(&gorm.Session{})

		sessionDB = sessionDB.Scopes(AppendSelect(
			clause.Column{Name: "name", Alias: "session_name"},
		))

		sessionSQL := sessionDB.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&testModel{})
		})

		originalSQL := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&testModel{})
		})

		expectedSessionSQL := `SELECT *,"name" AS "session_name" FROM "test_models"`
		require.Equal(t, expectedSessionSQL, sessionSQL)

		expectedOriginalSQL := `SELECT * FROM "test_models"`
		require.Equal(t, expectedOriginalSQL, originalSQL)
	})

	t.Run("session preserves appended columns", func(t *testing.T) {
		queryDB := db.Scopes(AppendSelect(
			clause.Column{Name: "id", Alias: "preserve_id"},
		))

		sessionDB := queryDB.Session(&gorm.Session{})

		sql := sessionDB.ToSQL(func(tx *gorm.DB) *gorm.DB {
			return tx.Find(&testModel{})
		})

		expectedSQL := `SELECT *,"id" AS "preserve_id" FROM "test_models"`
		require.Equal(t, expectedSQL, sql)
	})
}
