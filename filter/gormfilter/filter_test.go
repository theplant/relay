package gormfilter_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/theplant/testenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/theplant/relay"
	"github.com/theplant/relay/cursor"
	"github.com/theplant/relay/filter"
	"github.com/theplant/relay/filter/gormfilter"
	"github.com/theplant/relay/gormrelay"
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
	Users       []*User        `json:"users"`
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
	Users       *UserFilter      `json:"users"`
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
	Profile     *Profile       `json:"profile"`
}

type Profile struct {
	ID        string         `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Bio       string         `json:"bio"`
	Avatar    *string        `json:"avatar"`
	UserID    string         `gorm:"not null;uniqueIndex" json:"userId"`
	User      *User          `json:"user"`
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
	Profile     *ProfileFilter `json:"profile"`
}

type ProfileFilter struct {
	Not       *ProfileFilter   `json:"not"`
	And       []*ProfileFilter `json:"and"`
	Or        []*ProfileFilter `json:"or"`
	ID        *filter.ID       `json:"id"`
	CreatedAt *filter.Time     `json:"createdAt"`
	UpdatedAt *filter.Time     `json:"updatedAt"`
	Bio       *filter.String   `json:"bio"`
	Avatar    *filter.String   `json:"avatar"`
	UserID    *filter.ID       `json:"userId"`
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
			{
				name: "has_one filter with bio equals",
				filter: &UserFilter{
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Eq: lo.ToPtr("Software Engineer"),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."id" IN (SELECT "profiles"."user_id" FROM "profiles" WHERE "profiles"."bio" = $1 AND "profiles"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"Software Engineer"},
			},
			{
				name: "has_one filter with bio contains",
				filter: &UserFilter{
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Contains: lo.ToPtr("engineer"),
							Fold:     true,
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."id" IN (SELECT "profiles"."user_id" FROM "profiles" WHERE LOWER("profiles"."bio") LIKE $1 AND "profiles"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"%engineer%"},
			},
			{
				name: "has_one filter with multiple conditions",
				filter: &UserFilter{
					Profile: &ProfileFilter{
						Bio: &filter.String{
							StartsWith: lo.ToPtr("Software"),
						},
						Avatar: &filter.String{
							IsNull: lo.ToPtr(false),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."id" IN (SELECT "profiles"."user_id" FROM "profiles" WHERE ("profiles"."avatar" IS NOT NULL AND "profiles"."bio" LIKE $1) AND "profiles"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"Software%"},
			},
			{
				name: "has_one filter combined with user filter",
				filter: &UserFilter{
					Age: &filter.Int{
						Gte: lo.ToPtr(25),
					},
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Contains: lo.ToPtr("engineer"),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 AND "users"."id" IN (SELECT "profiles"."user_id" FROM "profiles" WHERE "profiles"."bio" LIKE $2 AND "profiles"."deleted_at" IS NULL)) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{float64(25), "%engineer%"},
			},
			{
				name: "has_one and belongs_to combined",
				filter: &UserFilter{
					Company: &CompanyFilter{
						Name: &filter.String{
							Eq: lo.ToPtr("Tech Corp"),
						},
					},
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Contains: lo.ToPtr("developer"),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE ("users"."company_id" IN (SELECT "companies"."id" FROM "companies" WHERE "companies"."name" = $1 AND "companies"."deleted_at" IS NULL) AND "users"."id" IN (SELECT "profiles"."user_id" FROM "profiles" WHERE "profiles"."bio" LIKE $2 AND "profiles"."deleted_at" IS NULL)) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"Tech Corp", "%developer%"},
			},
			{
				name: "not has_one filter",
				filter: &UserFilter{
					Not: &UserFilter{
						Profile: &ProfileFilter{
							Bio: &filter.String{
								Contains: lo.ToPtr("manager"),
							},
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."id" NOT IN (SELECT "profiles"."user_id" FROM "profiles" WHERE "profiles"."bio" LIKE $1 AND "profiles"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"%manager%"},
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

	t.Run("unsupported relationship types", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		tests := []struct {
			name       string
			filter     *CompanyFilter
			wantErrMsg string
		}{
			{
				name: "has_many relationship should be rejected",
				filter: &CompanyFilter{
					Users: &UserFilter{
						Age: &filter.Int{
							Gte: lo.ToPtr(18),
						},
					},
				},
				wantErrMsg: `unsupported relationship type "has_many" for field "Users" (only belongs_to and has_one are supported)`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stmt := db.Model(&Company{}).
					Scopes(gormfilter.Scope(tt.filter)).
					Session(&gorm.Session{DryRun: true}).
					Find(&[]Company{})

				require.ErrorContains(t, stmt.Error, tt.wantErrMsg)
			})
		}
	})

	t.Run("disable belongs_to filter option", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{}, &Profile{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{}, &Profile{})
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
				name: "has_one filter should still work when belongs_to is disabled",
				filter: &UserFilter{
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Contains: lo.ToPtr("engineer"),
						},
					},
				},
				wantSQL:  `SELECT * FROM "users" WHERE "users"."id" IN (SELECT "profiles"."user_id" FROM "profiles" WHERE "profiles"."bio" LIKE $1 AND "profiles"."deleted_at" IS NULL) AND "users"."deleted_at" IS NULL`,
				wantVars: []any{"%engineer%"},
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

	t.Run("disable has_one filter option", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{}, &Profile{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{}, &Profile{})
		require.NoError(t, err)

		tests := []struct {
			name       string
			filter     *UserFilter
			wantSQL    string
			wantVars   []any
			wantErrMsg string
		}{
			{
				name: "normal filter should work when has_one is disabled",
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
				name: "belongs_to filter should still work when has_one is disabled",
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
				name: "has_one filter should be disabled",
				filter: &UserFilter{
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Contains: lo.ToPtr("engineer"),
						},
					},
				},
				wantErrMsg: `has_one filter is disabled for field "Profile"`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stmt := db.Model(&User{}).
					Scopes(gormfilter.Scope(tt.filter, gormfilter.WithDisableHasOne())).
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

	t.Run("disable all relationships filter option", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{}, &Profile{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{}, &Profile{})
		require.NoError(t, err)

		tests := []struct {
			name       string
			filter     *UserFilter
			wantSQL    string
			wantVars   []any
			wantErrMsg string
		}{
			{
				name: "normal filter should work when all relationships are disabled",
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
				name: "has_one filter should be disabled",
				filter: &UserFilter{
					Profile: &ProfileFilter{
						Bio: &filter.String{
							Contains: lo.ToPtr("engineer"),
						},
					},
				},
				wantErrMsg: `has_one filter is disabled for field "Profile"`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				stmt := db.Model(&User{}).
					Scopes(gormfilter.Scope(tt.filter, gormfilter.WithDisableRelationships())).
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

func TestErrorHandling(t *testing.T) {
	t.Run("nil filter", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(nil)).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.NoError(t, stmt.Error)
		require.Equal(t, `SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL`, stmt.Statement.SQL.String())
	})

	t.Run("missing field in schema", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"NonExistentField": map[string]any{
					"Eq": "test",
				},
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, `missing field "NonExistentField" in schema`)
	})

	t.Run("invalid AND filter format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"And": "invalid",
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, "invalid AND filter format")
	})

	t.Run("invalid OR filter format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Or": "invalid",
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, "invalid OR filter format")
	})

	t.Run("invalid NOT filter format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Not": "invalid",
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, "invalid NOT filter format")
	})

	t.Run("invalid field filter format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Name": 123,
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, "invalid filter format for field Name")
	})

	t.Run("invalid In values format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Name": map[string]any{
					"In": "not-an-array",
				},
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, `invalid In values for field "Name"`)
	})

	t.Run("invalid IsNull value format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Name": map[string]any{
					"IsNull": "not-a-bool",
				},
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, `invalid IS NULL value for field "Name"`)
	})

	t.Run("invalid Contains value format", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Name": map[string]any{
					"Contains": 123,
				},
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, `invalid Contains value for field "Name"`)
	})

	t.Run("unknown operator", func(t *testing.T) {
		err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
		require.NoError(t, err)
		err = db.AutoMigrate(&Country{}, &Company{}, &User{})
		require.NoError(t, err)

		stmt := db.Model(&User{}).
			Scopes(gormfilter.Scope(map[string]any{
				"Name": map[string]any{
					"UnknownOp": "test",
				},
			})).
			Session(&gorm.Session{DryRun: true}).
			Find(&[]User{})

		require.ErrorContains(t, stmt.Error, `unknown operator UnknownOp for field "Name"`)
	})
}

func TestAdditionalOperators(t *testing.T) {
	err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
	require.NoError(t, err)
	err = db.AutoMigrate(&Country{}, &Company{}, &User{})
	require.NoError(t, err)

	tests := []struct {
		name     string
		filter   *UserFilter
		wantSQL  string
		wantVars []any
	}{
		{
			name: "greater than condition",
			filter: &UserFilter{
				Age: &filter.Int{
					Gt: lo.ToPtr(30),
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE "users"."age" > $1 AND "users"."deleted_at" IS NULL`,
			wantVars: []any{float64(30)},
		},
		{
			name: "combined Gt and Lt",
			filter: &UserFilter{
				Age: &filter.Int{
					Gt: lo.ToPtr(18),
					Lt: lo.ToPtr(65),
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" > $1 AND "users"."age" < $2) AND "users"."deleted_at" IS NULL`,
			wantVars: []any{float64(18), float64(65)},
		},
		{
			name: "not in empty array already tested",
			filter: &UserFilter{
				Age: &filter.Int{
					In: []int{},
				},
			},
			wantSQL:  "",
			wantVars: nil,
		},
		{
			name: "empty And array",
			filter: &UserFilter{
				And: []*UserFilter{},
			},
			wantSQL:  `SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL`,
			wantVars: nil,
		},
		{
			name: "empty Or array",
			filter: &UserFilter{
				Or: []*UserFilter{},
			},
			wantSQL:  `SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL`,
			wantVars: nil,
		},
		{
			name: "nested Not conditions",
			filter: &UserFilter{
				Not: &UserFilter{
					Not: &UserFilter{
						Name: &filter.String{
							Eq: lo.ToPtr("John"),
						},
					},
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE NOT "users"."name" <> $1 AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"John"},
		},
		{
			name: "Not with multiple conditions",
			filter: &UserFilter{
				Not: &UserFilter{
					Name: &filter.String{
						Contains: lo.ToPtr("john"),
					},
					Age: &filter.Int{
						Gt: lo.ToPtr(30),
					},
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" <= $1 OR "users"."name" NOT LIKE $2) AND "users"."deleted_at" IS NULL`,
			wantVars: []any{float64(30), "%john%"},
		},
		{
			name: "StartsWith without fold",
			filter: &UserFilter{
				Name: &filter.String{
					StartsWith: lo.ToPtr("J"),
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE "users"."name" LIKE $1 AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"J%"},
		},
		{
			name: "EndsWith with fold",
			filter: &UserFilter{
				Name: &filter.String{
					EndsWith: lo.ToPtr("SON"),
					Fold:     true,
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE LOWER("users"."name") LIKE $1 AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"%son"},
		},
		{
			name: "Eq with fold",
			filter: &UserFilter{
				Name: &filter.String{
					Eq:   lo.ToPtr("JOHN"),
					Fold: true,
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE LOWER("users"."name") = $1 AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"john"},
		},
		{
			name: "Neq with fold",
			filter: &UserFilter{
				Name: &filter.String{
					Neq:  lo.ToPtr("JOHN"),
					Fold: true,
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE LOWER("users"."name") <> $1 AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"john"},
		},
		{
			name: "Lt and Lte with fold",
			filter: &UserFilter{
				Name: &filter.String{
					Lt:   lo.ToPtr("M"),
					Lte:  lo.ToPtr("Z"),
					Fold: true,
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE (LOWER("users"."name") < $1 AND LOWER("users"."name") <= $2) AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"m", "z"},
		},
		{
			name: "Gt and Gte with fold",
			filter: &UserFilter{
				Name: &filter.String{
					Gt:   lo.ToPtr("A"),
					Gte:  lo.ToPtr("B"),
					Fold: true,
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE (LOWER("users"."name") > $1 AND LOWER("users"."name") >= $2) AND "users"."deleted_at" IS NULL`,
			wantVars: []any{"a", "b"},
		},
		{
			name: "combined field filter with And operator",
			filter: &UserFilter{
				Name: &filter.String{
					StartsWith: lo.ToPtr("J"),
					EndsWith:   lo.ToPtr("n"),
				},
				And: []*UserFilter{
					{
						Age: &filter.Int{
							Gte: lo.ToPtr(20),
						},
					},
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE ("users"."age" >= $1 AND ("users"."name" LIKE $2 AND "users"."name" LIKE $3)) AND "users"."deleted_at" IS NULL`,
			wantVars: []any{float64(20), "%n", "J%"},
		},
		{
			name: "Not with Or",
			filter: &UserFilter{
				Not: &UserFilter{
					Or: []*UserFilter{
						{
							Age: &filter.Int{
								Lt: lo.ToPtr(20),
							},
						},
						{
							Age: &filter.Int{
								Gt: lo.ToPtr(60),
							},
						},
					},
				},
			},
			wantSQL:  `SELECT * FROM "users" WHERE NOT ("users"."age" < $1 OR "users"."age" > $2) AND "users"."deleted_at" IS NULL`,
			wantVars: []any{float64(20), float64(60)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantSQL == "" {
				// Skip tests that expect errors (already handled elsewhere)
				return
			}

			stmt := db.Model(&User{}).
				Scopes(gormfilter.Scope(tt.filter)).
				Session(&gorm.Session{DryRun: true}).
				Find(&[]User{})

			require.NoError(t, stmt.Error)

			sql := stmt.Statement.SQL.String()
			vars := stmt.Statement.Vars

			require.Equal(t, tt.wantSQL, sql)
			require.Equal(t, tt.wantVars, vars)
		})
	}
}

func TestCombiningWithPagination(t *testing.T) {
	err := db.Migrator().DropTable(&User{}, &Company{}, &Country{})
	require.NoError(t, err)
	err = db.AutoMigrate(&Country{}, &Company{}, &User{})
	require.NoError(t, err)

	countries := []*Country{
		{ID: "us", Name: "United States", Code: "US"},
		{ID: "uk", Name: "United Kingdom", Code: "UK"},
		{ID: "jp", Name: "Japan", Code: "JP"},
	}
	err = db.Create(&countries).Error
	require.NoError(t, err)

	companies := []*Company{
		{ID: "tech-corp-us", Name: "Tech Corp", CountryID: "us"},
		{ID: "tech-solutions-uk", Name: "Tech Solutions", CountryID: "uk"},
		{ID: "software-inc-us", Name: "Software Inc", CountryID: "us"},
		{ID: "media-company-jp", Name: "Media Company", CountryID: "jp"},
	}
	err = db.Create(&companies).Error
	require.NoError(t, err)

	users := []*User{
		{ID: "1", Name: "Alice", Age: 25, CompanyID: "tech-corp-us"},
		{ID: "2", Name: "Bob", Age: 30, CompanyID: "tech-corp-us"},
		{ID: "3", Name: "Charlie", Age: 22, CompanyID: "tech-solutions-uk"},
		{ID: "4", Name: "David", Age: 35, CompanyID: "software-inc-us"},
		{ID: "5", Name: "Eve", Age: 28, CompanyID: "tech-solutions-uk"},
		{ID: "6", Name: "Frank", Age: 17, CompanyID: "media-company-jp"},
		{ID: "7", Name: "Grace", Age: 40, CompanyID: "tech-corp-us"},
		{ID: "8", Name: "Henry", Age: 19, CompanyID: "software-inc-us"},
	}
	err = db.Create(&users).Error
	require.NoError(t, err)

	t.Run("keyset pagination with filter", func(t *testing.T) {
		p := relay.New(
			cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return gormrelay.NewKeysetAdapter[*User](
					db.WithContext(ctx).Scopes(gormfilter.Scope(&UserFilter{
						Age: &filter.Int{
							Gte: lo.ToPtr(18),
						},
						Company: &CompanyFilter{
							Name: &filter.String{
								Contains: lo.ToPtr("Tech"),
								Fold:     true,
							},
						},
					})),
				)(ctx, req)
			}),
			relay.EnsureLimits[*User](2, 100),
			relay.EnsurePrimaryOrderBy[*User](
				relay.OrderBy{Field: "ID", Desc: false},
			),
		)

		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(2),
		})
		require.NoError(t, err)
		require.Equal(t, 5, *conn.TotalCount)
		require.Len(t, conn.Edges, 2)
		require.Equal(t, "1", conn.Edges[0].Node.ID)
		require.Equal(t, "Alice", conn.Edges[0].Node.Name)
		require.Equal(t, 25, conn.Edges[0].Node.Age)
		require.Equal(t, "2", conn.Edges[1].Node.ID)
		require.Equal(t, "Bob", conn.Edges[1].Node.Name)
		require.Equal(t, 30, conn.Edges[1].Node.Age)
		require.True(t, conn.PageInfo.HasNextPage)
		require.False(t, conn.PageInfo.HasPreviousPage)

		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(2),
			After: conn.PageInfo.EndCursor,
		})
		require.NoError(t, err)
		require.Equal(t, 5, *conn.TotalCount)
		require.Len(t, conn.Edges, 2)
		require.Equal(t, "3", conn.Edges[0].Node.ID)
		require.Equal(t, "Charlie", conn.Edges[0].Node.Name)
		require.Equal(t, "5", conn.Edges[1].Node.ID)
		require.Equal(t, "Eve", conn.Edges[1].Node.Name)
		require.True(t, conn.PageInfo.HasNextPage)
		require.True(t, conn.PageInfo.HasPreviousPage)
	})

	t.Run("offset pagination with filter", func(t *testing.T) {
		p := relay.New(
			cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return gormrelay.NewOffsetAdapter[*User](
					db.WithContext(ctx).Scopes(gormfilter.Scope(&UserFilter{
						Age: &filter.Int{
							Gte: lo.ToPtr(18),
						},
						Company: &CompanyFilter{
							Name: &filter.String{
								Contains: lo.ToPtr("Tech"),
								Fold:     true,
							},
						},
					})),
				)(ctx, req)
			}),
			relay.EnsureLimits[*User](2, 100),
			relay.EnsurePrimaryOrderBy[*User](
				relay.OrderBy{Field: "ID", Desc: false},
			),
		)

		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(2),
		})
		require.NoError(t, err)
		require.Equal(t, 5, *conn.TotalCount)
		require.Len(t, conn.Edges, 2)
		require.Equal(t, "1", conn.Edges[0].Node.ID)
		require.Equal(t, "2", conn.Edges[1].Node.ID)
		require.True(t, conn.PageInfo.HasNextPage)
		require.False(t, conn.PageInfo.HasPreviousPage)

		conn, err = p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(2),
			After: conn.PageInfo.EndCursor,
		})
		require.NoError(t, err)
		require.Equal(t, 5, *conn.TotalCount)
		require.Len(t, conn.Edges, 2)
		require.Equal(t, "3", conn.Edges[0].Node.ID)
		require.Equal(t, "5", conn.Edges[1].Node.ID)
		require.True(t, conn.PageInfo.HasNextPage)
		require.True(t, conn.PageInfo.HasPreviousPage)
	})

	t.Run("nested relationship filter with pagination", func(t *testing.T) {
		p := relay.New(
			cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return gormrelay.NewKeysetAdapter[*User](
					db.WithContext(ctx).Scopes(gormfilter.Scope(&UserFilter{
						Age: &filter.Int{
							Gte: lo.ToPtr(20),
						},
						Company: &CompanyFilter{
							Country: &CountryFilter{
								Code: &filter.String{
									Eq: lo.ToPtr("US"),
								},
							},
						},
					})),
				)(ctx, req)
			}),
			relay.EnsureLimits[*User](10, 100),
			relay.EnsurePrimaryOrderBy[*User](
				relay.OrderBy{Field: "ID", Desc: false},
			),
		)

		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, 4, *conn.TotalCount)
		require.Len(t, conn.Edges, 4)
		require.Equal(t, "1", conn.Edges[0].Node.ID)
		require.Equal(t, "2", conn.Edges[1].Node.ID)
		require.Equal(t, "4", conn.Edges[2].Node.ID)
		require.Equal(t, "7", conn.Edges[3].Node.ID)
		require.False(t, conn.PageInfo.HasNextPage)
		require.False(t, conn.PageInfo.HasPreviousPage)
	})

	t.Run("complex filter with OR and pagination", func(t *testing.T) {
		p := relay.New(
			cursor.Base64(func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[*User], error) {
				return gormrelay.NewKeysetAdapter[*User](
					db.WithContext(ctx).Scopes(gormfilter.Scope(&UserFilter{
						Or: []*UserFilter{
							{
								Age: &filter.Int{
									Lt: lo.ToPtr(20),
								},
							},
							{
								Age: &filter.Int{
									Gt: lo.ToPtr(35),
								},
							},
						},
					})),
				)(ctx, req)
			}),
			relay.EnsureLimits[*User](10, 100),
			relay.EnsurePrimaryOrderBy[*User](
				relay.OrderBy{Field: "Age", Desc: false},
				relay.OrderBy{Field: "ID", Desc: false},
			),
		)

		conn, err := p.Paginate(context.Background(), &relay.PaginateRequest[*User]{
			First: lo.ToPtr(10),
		})
		require.NoError(t, err)
		require.Equal(t, 3, *conn.TotalCount)
		require.Len(t, conn.Edges, 3)
		require.Equal(t, "6", conn.Edges[0].Node.ID)
		require.Equal(t, 17, conn.Edges[0].Node.Age)
		require.Equal(t, "8", conn.Edges[1].Node.ID)
		require.Equal(t, 19, conn.Edges[1].Node.Age)
		require.Equal(t, "7", conn.Edges[2].Node.ID)
		require.Equal(t, 40, conn.Edges[2].Node.Age)
		require.False(t, conn.PageInfo.HasNextPage)
		require.False(t, conn.PageInfo.HasPreviousPage)
	})
}
