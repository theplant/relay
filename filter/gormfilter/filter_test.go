package gormfilter_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/theplant/relay/filter"
	"github.com/theplant/relay/filter/gormfilter"
	"github.com/theplant/testenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Country struct {
	ID        string         `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Name      string         `gorm:"not null" json:"name"`
	Code      string         `gorm:"not null" json:"code"`
}

type CountryFilter struct {
	Not       *CountryFilter   `json:"not"`
	And       []*CountryFilter `json:"and"`
	Or        []*CountryFilter `json:"or"`
	ID        *filter.ID       `json:"id"`
	CreatedAt *filter.Time     `json:"createdAt"`
	UpdatedAt *filter.Time     `json:"updatedAt"`
	Name      *filter.String   `json:"name"`
	Code      *filter.String   `json:"code"`
}

type Company struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Name        string         `gorm:"not null" json:"name"`
	Description *string        `json:"description"`
	CountryID   string         `gorm:"not null" json:"countryId"`
	Country     *Country       `json:"country"`
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
	Country     *CountryFilter   `json:"country"`
}

type User struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Name        string         `gorm:"not null" json:"name"`
	Description *string        `json:"description"`
	Age         int            `gorm:"not null" json:"age"`
	CompanyID   string         `gorm:"not null" json:"companyId"`
	Company     *Company       `json:"company"`
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
	CompanyID   *filter.ID     `json:"companyId"`
	Company     *CompanyFilter `json:"company"`
}

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

func TestScope(t *testing.T) {
	t.Run("json number type conversion", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		country := &Country{ID: "country1", Name: "Test Country", Code: "TC"}
		err = db.Create(country).Error
		require.NoError(t, err)

		company := &Company{ID: "company1", Name: "Test Company", CountryID: "country1"}
		err = db.Create(company).Error
		require.NoError(t, err)

		users := []*User{
			{ID: "1", Name: "user1", Age: 18, CompanyID: "company1"},
			{ID: "2", Name: "user2", Age: 20, CompanyID: "company1"},
		}
		err = db.Create(&users).Error
		require.NoError(t, err)

		var result []*User
		err = db.Scopes(
			gormfilter.Scope(&UserFilter{
				Age: &filter.Int{Eq: lo.ToPtr(18)},
			}),
		).Find(&result).Error
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, 18, result[0].Age)
	})

	t.Run("sql", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
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
				name:     "empty filter",
				filter:   &UserFilter{},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL`,
				wantVars: nil,
			},
			{
				name: "simple equals",
				filter: &UserFilter{
					Name: &filter.String{
						Eq: lo.ToPtr("John"),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."name" = $1 AND "users"."deleted_at" IS NULL`,
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
				wantSQL:  `SELECT * FROM "users" WHERE LOWER("users"."name") LIKE $1 AND "users"."deleted_at" IS NULL`,
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
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 AND LOWER("users"."name") LIKE $2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(18), "%john%"},
			},
			{
				name: "multiple conditions with OR",
				filter: &UserFilter{
					Or: []*UserFilter{
						{
							Name: &filter.String{
								Contains: lo.ToPtr("joHn"),
								Fold:     true,
							},
						},
						{
							Age: &filter.Int{
								Gte: lo.ToPtr(18),
							},
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE (LOWER("users"."name") LIKE $1 OR "users"."age" >= $2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"%john%", float64(18)},
			},
			{
				name: "multiple conditions in one field filter",
				filter: &UserFilter{
					Description: &filter.String{
						In:     []string{"desc1", "desc2"},
						IsNull: lo.ToPtr(false),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."description" IN ($1,$2) AND "users"."description" IS NOT NULL) AND "users"."deleted_at" IS NULL`,
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
				wantSQL:  `SELECT * FROM "users" WHERE (LOWER("users"."name") LIKE $1 OR ("users"."age" > $2 AND "users"."created_at" > $3)) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"j%", float64(30), "2024-01-01T00:00:00Z"},
			},
			{
				name: "complex filter with AND",
				filter: &UserFilter{
					And: []*UserFilter{
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
						},
					},
					CreatedAt: &filter.Time{
						Gt: lo.ToPtr(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ((LOWER("users"."name") LIKE $1 AND "users"."age" > $2) AND "users"."created_at" > $3) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"j%", float64(30), "2024-01-01T00:00:00Z"},
			},
			{
				name: "with pre-existing where clause",
				filter: &UserFilter{
					Name: &filter.String{
						Eq: lo.ToPtr("john"),
					},
					Description: &filter.String{
						IsNull: lo.ToPtr(true),
					},
				},
				before: func(db *gorm.DB) *gorm.DB {
					return db.Where("age > ? OR age < ?", 100, 200)
				},
				wantSQL:  `SELECT * FROM "users" WHERE (age > $1 OR age < $2) AND ("users"."description" IS NULL AND "users"."name" = $3) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{100, 200, "john"},
			},
			{
				name: "not condition",
				filter: &UserFilter{
					Not: &UserFilter{
						Name: &filter.String{
							Eq:   lo.ToPtr("joHn"),
							Fold: true,
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE LOWER("users"."name") <> $1 AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"john"},
			},
			{
				name: "not equal condition",
				filter: &UserFilter{
					Age: &filter.Int{
						Neq: lo.ToPtr(20),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."age" <> $1 AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(20)},
			},
			{
				name: "in with case insensitive",
				filter: &UserFilter{
					Name: &filter.String{
						In:   []string{"jOhn", "jaNe"},
						Fold: true,
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE LOWER("users"."name") IN ($1,$2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"john", "jane"},
			},
			{
				name: "not in condition",
				filter: &UserFilter{
					Age: &filter.Int{
						NotIn: []int{18, 20, 25},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."age" NOT IN ($1,$2,$3) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(18), float64(20), float64(25)},
			},
			{
				name: "less than condition",
				filter: &UserFilter{
					Age: &filter.Int{
						Lt: lo.ToPtr(30),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."age" < $1 AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(30)},
			},
			{
				name: "less than or equal condition",
				filter: &UserFilter{
					Age: &filter.Int{
						Lte: lo.ToPtr(25),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."age" <= $1 AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(25)},
			},
			{
				name: "ends with condition",
				filter: &UserFilter{
					Name: &filter.String{
						EndsWith: lo.ToPtr("son"),
					},
					Age: &filter.Int{
						Lt: lo.ToPtr(30),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" < $1 AND "users"."name" LIKE $2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(30), "%son"},
			},
			{
				name: "not ends with condition",
				filter: &UserFilter{
					Not: &UserFilter{
						Name: &filter.String{
							EndsWith: lo.ToPtr("son"),
						},
						Age: &filter.Int{
							Lt: lo.ToPtr(30),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 OR "users"."name" NOT LIKE $2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(30), "%son"},
			},
			{
				name: "in times",
				filter: &UserFilter{
					CreatedAt: &filter.Time{
						In: []time.Time{
							time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
							time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."created_at" IN ($1,$2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"2024-01-01T00:00:00Z", "2024-01-02T00:00:00Z"},
			},
			{
				name: "not in empty array",
				filter: &UserFilter{
					CreatedAt: &filter.Time{
						NotIn: []time.Time{},
					},
				},
				wantErrMsg: `empty NotIn values for field "CreatedAt"`,
			},
			{
				name: "belongs_to filter with name equals",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Name: &filter.String{
							Eq: lo.ToPtr("company1"),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE "companies"."name" = $1 AND "companies"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"company1"},
			},
			{
				name: "belongs_to filter with name contains",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Name: &filter.String{
							Contains: lo.ToPtr("tech"),
							Fold:     true,
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE LOWER("companies"."name") LIKE $1 AND "companies"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"%tech%"},
			},
			{
				name: "belongs_to filter with multiple conditions",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Name: &filter.String{
							StartsWith: lo.ToPtr("Tech"),
						},
						Description: &filter.String{
							IsNull: lo.ToPtr(false),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE ("companies"."description" IS NOT NULL AND "companies"."name" LIKE $1) AND "companies"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"Tech%"},
			},
			{
				name: "filter by company_id",
				filter: &UserFilter{
					CompanyID: &filter.ID{
						Eq: lo.ToPtr("company1"),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" = $1 AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"company1"},
			},
			{
				name: "filter by company_id with multiple values",
				filter: &UserFilter{
					CompanyID: &filter.ID{
						In: []string{"company1", "company2"},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" IN ($1,$2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"company1", "company2"},
			},
			{
				name: "belongs_to filter combined with user filter",
				filter: &UserFilter{
					Age: &filter.Int{
						Gte: lo.ToPtr(25),
					},
					Company: &CompanyFilter{
						Name: &filter.String{
							Eq: lo.ToPtr("company1"),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 AND "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE "companies"."name" = $2 AND "companies"."deleted_at" IS NULL)) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(25), "company1"},
			},
			{
				name: "two level nested belongs_to filter",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Country: &CountryFilter{
							Code: &filter.String{
								Eq: lo.ToPtr("US"),
							},
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE "companies"."country_id" IN (SELECT "countries"."id" FROM "countries" WHERE "countries"."code" = $1 AND "countries"."deleted_at" IS NULL) AND "companies"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"US"},
			},
			{
				name: "two level nested belongs_to filter with multiple conditions",
				filter: &UserFilter{
					Age: &filter.Int{
						Gte: lo.ToPtr(21),
					},
					Company: &CompanyFilter{
						Name: &filter.String{
							Contains: lo.ToPtr("Tech"),
						},
						Country: &CountryFilter{
							Name: &filter.String{
								Eq: lo.ToPtr("United States"),
							},
							Code: &filter.String{
								Eq: lo.ToPtr("US"),
							},
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 AND "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE ("companies"."country_id" IN (SELECT "countries"."id" FROM "countries" WHERE ("countries"."code" = $2 AND "countries"."name" = $3) AND "countries"."deleted_at" IS NULL) AND "companies"."name" LIKE $4) AND "companies"."deleted_at" IS NULL)) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(21), "US", "United States", "%Tech%"},
			},
			{
				name: "two level nested with case insensitive",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Name: &filter.String{
							StartsWith: lo.ToPtr("tech"),
							Fold:       true,
						},
						Country: &CountryFilter{
							Name: &filter.String{
								Contains: lo.ToPtr("america"),
								Fold:     true,
							},
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE ("companies"."country_id" IN (SELECT "countries"."id" FROM "countries" WHERE LOWER("countries"."name") LIKE $1 AND "countries"."deleted_at" IS NULL) AND LOWER("companies"."name") LIKE $2) AND "companies"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"%america%", "tech%"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				q := db.Model(&User{})
				if tt.before != nil {
					q = tt.before(q)
				}
				stmt := q.Scopes(gormfilter.Scope(tt.filter)).Session(&gorm.Session{DryRun: true})
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
	})

	t.Run("disable belongs_to filter option", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		tests := []struct {
			name       string
			filter     *UserFilter
			wantSQL    string
			wantVars   []any
			wantErrMsg string
		}{
			{
				name: "normal filter should work when belongs_to is disabled",
				filter: &UserFilter{
					Age: &filter.Int{
						Gte: lo.ToPtr(18),
					},
					Name: &filter.String{
						Contains: lo.ToPtr("user"),
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 AND "users"."name" LIKE $2) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(18), "%user%"},
			},
			{
				name: "belongs_to filter should be disabled",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Name: &filter.String{
							Eq: lo.ToPtr("company1"),
						},
					},
				},
				wantErrMsg: `belongs_to filter is disabled for field "Company"`,
			},
			{
				name: "two level nested should be disabled",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Country: &CountryFilter{
							Code: &filter.String{
								Eq: lo.ToPtr("US"),
							},
						},
					},
				},
				wantErrMsg: `belongs_to filter is disabled for field "Company"`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stmt := db.Model(&User{}).
					Scopes(gormfilter.Scope(tt.filter, gormfilter.WithDisableBelongsTo())).
					Session(&gorm.Session{DryRun: true}).
					Find(&[]User{})

				if tt.wantErrMsg != "" {
					require.ErrorContains(t, stmt.Error, tt.wantErrMsg)
					return
				}

				require.NoError(t, stmt.Error)

				if tt.wantSQL != "" {
					sql := stmt.Statement.SQL.String()
					vars := stmt.Statement.Vars
					require.Equal(t, tt.wantSQL, sql)
					require.Equal(t, tt.wantVars, vars)
				}
			})
		}
	})
}
