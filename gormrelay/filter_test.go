package gormrelay_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/theplant/relay/filter"
	"github.com/theplant/relay/gormrelay"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Company struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Name        string         `gorm:"not null" json:"name"`
	Description *string        `json:"description"`
}

type CompanyFilter struct {
	Not         *CompanyFilter   `json:"not"`
	And         []*CompanyFilter `json:"and"`
	Or          []*CompanyFilter `json:"or"`
	ID          *filter.ID       `json:"id"`
	CreatedAt   *filter.Time     `json:"createdAt"`
	UpdatedAt   *filter.Time     `json:"updatedAt"`
	Name        *filter.String   `json:"name"`
	Description *filter.String   `json:"description"`
}

type User struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Name        string         `gorm:"not null" json:"name"`
	Description *string        `json:"description"`
	Age         int            `gorm:"not null" json:"age"`
}

type UserFilter struct {
	Not         *UserFilter    `json:"not"`
	And         []*UserFilter  `json:"and"`
	Or          []*UserFilter  `json:"or"`
	ID          *filter.ID     `json:"id"`
	CreatedAt   *filter.Time   `json:"createdAt"`
	UpdatedAt   *filter.Time   `json:"updatedAt"`
	Name        *filter.String `json:"name"`
	Description *filter.String `json:"description"`
	Age         *filter.Int    `json:"age"`
}

func TestScopeFilter(t *testing.T) {
	// TODO: postgres
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	tests := []struct {
		name       string
		filter     *UserFilter
		before     func(db *gorm.DB) *gorm.DB
		wantSQL    string
		wantVars   []any
		wantErrMsg string
	}{
		{
			name: "simple equals",
			filter: &UserFilter{
				Name: &filter.String{
					Equals: lo.ToPtr("John"),
				},
			},
			wantSQL:  "SELECT * FROM `users` WHERE `users`.`name` = ? AND `users`.`deleted_at` IS NULL",
			wantVars: []any{"John"},
		},
		{
			name: "case insensitive contains",
			filter: &UserFilter{
				Name: &filter.String{
					Contains: lo.ToPtr("John"),
					Fold:     true,
				},
			},
			wantSQL:  "SELECT * FROM `users` WHERE LOWER(`users`.`name`) LIKE ? AND `users`.`deleted_at` IS NULL",
			wantVars: []any{"%john%"},
		},
		{
			name: "multiple conditions with AND",
			filter: &UserFilter{
				Name: &filter.String{
					Contains: lo.ToPtr("joHn"),
					Fold:     true,
				},
				Age: &filter.Int{
					Gte: lo.ToPtr(18),
				},
			},
			wantSQL:  "SELECT * FROM `users` WHERE (`users`.`age` >= ? AND LOWER(`users`.`name`) LIKE ?) AND `users`.`deleted_at` IS NULL",
			wantVars: []any{float64(18), "%john%"},
		},
		{
			name: "multiple conditions in one field filter",
			filter: &UserFilter{
				Description: &filter.String{
					In:     []string{"desc1", "desc2"},
					IsNull: lo.ToPtr(false),
				},
			},
			wantSQL:  "SELECT * FROM `users` WHERE (`users`.`description` IN (?,?) AND `users`.`description` IS NOT NULL) AND `users`.`deleted_at` IS NULL",
			wantVars: []any{"desc1", "desc2"},
		},
		{
			name: "complex filter with OR and time range",
			filter: &UserFilter{
				Or: []*UserFilter{
					{
						Name: &filter.String{
							StartsWith: lo.ToPtr("j"),
							Fold:       true,
						},
					},
					{
						Age: &filter.Int{
							Gt: lo.ToPtr(30),
						},
						CreatedAt: &filter.Time{
							Gt: lo.ToPtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
						},
					},
				},
			},
			wantSQL:  "SELECT * FROM `users` WHERE (LOWER(`users`.`name`) LIKE ? OR (`users`.`age` > ? AND `users`.`created_at` > ?)) AND `users`.`deleted_at` IS NULL",
			wantVars: []any{"j%", float64(30), "2024-01-01T00:00:00Z"},
		},
		{
			name: "with pre-existing where clause",
			filter: &UserFilter{
				Name: &filter.String{
					Equals: lo.ToPtr("john"),
				},
				Description: &filter.String{
					IsNull: lo.ToPtr(true),
				},
			},
			before: func(db *gorm.DB) *gorm.DB {
				return db.Where("age > ? OR age < ?", 100, 200)
			},
			wantSQL:  "SELECT * FROM `users` WHERE (age > ? OR age < ?) AND (`users`.`description` IS NULL AND `users`.`name` = ?) AND `users`.`deleted_at` IS NULL",
			wantVars: []any{100, 200, "john"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := db.Model(&User{})
			if tt.before != nil {
				q = tt.before(q)
			}
			stmt := q.Scopes(gormrelay.ScopeFilter(tt.filter)).Session(&gorm.Session{DryRun: true})
			stmt = stmt.Find(&[]User{})

			if tt.wantErrMsg != "" {
				require.ErrorContains(t, stmt.Error, tt.wantErrMsg)
				return
			}

			require.NoError(t, stmt.Error)

			sql := stmt.Statement.SQL.String()
			vars := stmt.Statement.Vars

			require.Equal(t, tt.wantSQL, sql)
			require.Equal(t, tt.wantVars, vars)
		})
	}
}
