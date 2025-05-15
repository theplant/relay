package perf

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theplant/testenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Company and User models
type Company struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `gorm:"index;not null" json:"createdAt"`
	UpdatedAt   time.Time      `gorm:"index;not null" json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deletedAt"`
	Name        string         `gorm:"not null" json:"name"`
	Description *string        `json:"description"`
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

// QueryPlanResult contains execution plan metrics
type QueryPlanResult struct {
	PlanningTime   float64
	ExecutionTime  float64
	TotalTime      float64
	TotalCost      float64
	MaxRows        int64
	ContainsJoin   bool
	JoinTypes      []string
	NodeTypes      []string
	ExplainResults []map[string]interface{}
}

// TestConfig holds performance test configuration parameters
type TestConfig struct {
	Runs             int
	CompanyCount     int
	UsersPerCompany  int
	DeletePercentage int
}

var (
	db  *gorm.DB
	dsn string
)

func init() {
	flag.StringVar(&dsn, "dsn", "", "PostgreSQL connection string for performance testing")
}

func TestMain(m *testing.M) {
	if !flag.Parsed() {
		flag.Parse()
	}

	if dsn != "" {
		var err error
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
		if err != nil {
			fmt.Printf("Failed to connect to database: %v\n", err)
			os.Exit(1)
		}

		// Close DB connection after tests complete
		defer func() {
			sqlDB, err := db.DB()
			if err == nil {
				sqlDB.Close()
			}
		}()
	} else {
		env, err := testenv.New().DBEnable(true).SetUp()
		if err != nil {
			panic(err)
		}
		defer env.TearDown()

		db = env.DB
		db.Logger = db.Logger.LogMode(logger.Info)
	}

	m.Run()
}

// TestQueryPerformance compares performance between IN subquery and JOIN approaches
func TestQueryPerformance(t *testing.T) {
	config := TestConfig{
		Runs:             30,
		CompanyCount:     100,
		UsersPerCompany:  1000,
		DeletePercentage: 10,
	}

	db := db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	err := createTestData(t, db, config)
	require.NoError(t, err, "Failed to create test data")

	// Define test queries
	exactInQuery := `SELECT * FROM users WHERE users.company_id IN (SELECT id FROM companies WHERE companies.name = 'Company-50' AND companies.deleted_at IS NULL) AND users.deleted_at IS NULL`
	exactJoinQuery := `SELECT users.* FROM users JOIN companies ON users.company_id = companies.id AND companies.deleted_at IS NULL WHERE companies.name = 'Company-50' AND users.deleted_at IS NULL`
	exactLeftJoinQuery := `SELECT users.* FROM users LEFT JOIN companies ON users.company_id = companies.id AND companies.deleted_at IS NULL WHERE companies.name = 'Company-50' AND users.deleted_at IS NULL`

	patternInQuery := `SELECT * FROM users WHERE users.company_id IN (SELECT id FROM companies WHERE companies.name LIKE 'Company-5%' AND companies.deleted_at IS NULL) AND users.deleted_at IS NULL`
	patternJoinQuery := `SELECT users.* FROM users JOIN companies ON users.company_id = companies.id AND companies.deleted_at IS NULL WHERE companies.name LIKE 'Company-5%' AND users.deleted_at IS NULL`

	// Display reference queries
	t.Log("\n-----------------------------------------------------")
	t.Log("TEST QUERIES: Copy these into your PostgreSQL client")
	t.Log("-----------------------------------------------------")
	t.Log("\nEXPLAIN ANALYZE " + exactInQuery)
	t.Log("\nEXPLAIN ANALYZE " + exactJoinQuery)
	t.Log("\nEXPLAIN ANALYZE " + patternInQuery)
	t.Log("\nEXPLAIN ANALYZE " + patternJoinQuery)
	t.Log("\nEXPLAIN ANALYZE " + exactLeftJoinQuery)

	t.Log("\n-----------------------------------------------------")
	t.Log(fmt.Sprintf("AUTOMATED PERFORMANCE COMPARISON (Running each query %d times)", config.Runs))
	t.Log("-----------------------------------------------------")

	// Execute queries and analyze results
	inQueryResult, err := executeExplainAnalyzeMultiple(t, db, exactInQuery, config.Runs)
	require.NoError(t, err, "Failed to execute IN query explain")

	joinQueryResult, err := executeExplainAnalyzeMultiple(t, db, exactJoinQuery, config.Runs)
	require.NoError(t, err, "Failed to execute JOIN query explain")

	leftJoinQueryResult, err := executeExplainAnalyzeMultiple(t, db, exactLeftJoinQuery, config.Runs)
	require.NoError(t, err, "Failed to execute LEFT JOIN query explain")

	patternInQueryResult, err := executeExplainAnalyzeMultiple(t, db, patternInQuery, config.Runs)
	require.NoError(t, err, "Failed to execute pattern IN query explain")

	patternJoinQueryResult, err := executeExplainAnalyzeMultiple(t, db, patternJoinQuery, config.Runs)
	require.NoError(t, err, "Failed to execute pattern JOIN query explain")

	// Compare exact match queries
	t.Log("\n--- EXACT MATCH COMPARISON (Company-50) ---")
	compareQueryResults(t, "IN Subquery", inQueryResult, "JOIN", joinQueryResult)
	compareQueryResults(t, "IN Subquery", inQueryResult, "LEFT JOIN", leftJoinQueryResult)
	compareQueryResults(t, "JOIN", joinQueryResult, "LEFT JOIN", leftJoinQueryResult)

	// Compare pattern match queries
	t.Log("\n--- PATTERN MATCH COMPARISON (Company-5%) ---")
	compareQueryResults(t, "IN Subquery", patternInQueryResult, "JOIN", patternJoinQueryResult)

	// Analyze whether IN queries were optimized to JOIN
	t.Log("\n--- QUERY PLAN ANALYSIS FOR 'IN' QUERIES ---")
	analyzeQueryPlan(t, "Exact Match IN Query", inQueryResult)
	analyzeQueryPlan(t, "Pattern Match IN Query", patternInQueryResult)
	assert.Equal(t, true, inQueryResult.ContainsJoin)
	assert.Equal(t, true, patternInQueryResult.ContainsJoin)
	t.Log("Test completed successfully.")
}

func createTestData(t *testing.T, db *gorm.DB, config TestConfig) error {
	err := createTables(t, db)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	companyIDs, err := insertCompanies(t, db, config.CompanyCount)
	if err != nil {
		return fmt.Errorf("failed to insert companies: %w", err)
	}

	err = insertUsers(t, db, companyIDs, config.UsersPerCompany, config.DeletePercentage)
	if err != nil {
		return fmt.Errorf("failed to insert users: %w", err)
	}

	return nil
}

func executeExplainAnalyzeMultiple(_ *testing.T, db *gorm.DB, query string, runs int) (*QueryPlanResult, error) {
	result := &QueryPlanResult{
		ExplainResults: make([]map[string]interface{}, 0, runs),
	}

	explainQuery := "EXPLAIN (ANALYZE, FORMAT JSON) " + query

	for i := 0; i < runs; i++ {
		var explainOutputStr string
		if err := db.Raw(explainQuery).Scan(&explainOutputStr).Error; err != nil {
			return nil, fmt.Errorf("failed to execute explain analyze (run %d): %w", i+1, err)
		}

		var planData []map[string]interface{}
		if err := json.Unmarshal([]byte(explainOutputStr), &planData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal explain output (run %d): %w", i+1, err)
		}

		if len(planData) == 0 {
			return nil, fmt.Errorf("no plan data found (run %d)", i+1)
		}

		result.ExplainResults = append(result.ExplainResults, planData[0])

		planningTime, executionTime := extractTimes(planData[0])
		result.PlanningTime += planningTime
		result.ExecutionTime += executionTime

		if i == 0 {
			nodeTypes, joinTypes := analyzePlanNodeTypes(planData[0])
			result.NodeTypes = nodeTypes
			result.JoinTypes = joinTypes
			result.ContainsJoin = len(joinTypes) > 0
		}
	}

	if runs > 0 {
		result.PlanningTime /= float64(runs)
		result.ExecutionTime /= float64(runs)
	}
	result.TotalTime = result.PlanningTime + result.ExecutionTime

	totalCost, maxRows := extractPlanNodeMetrics(result.ExplainResults[len(result.ExplainResults)-1])
	result.TotalCost = totalCost
	result.MaxRows = maxRows

	return result, nil
}

func extractTimes(plan map[string]interface{}) (float64, float64) {
	var planningTime, executionTime float64

	if pt, ok := plan["Planning Time"].(float64); ok {
		planningTime = pt
	}

	if et, ok := plan["Execution Time"].(float64); ok {
		executionTime = et
	}

	return planningTime, executionTime
}

func analyzePlanNodeTypes(node map[string]interface{}) ([]string, []string) {
	var nodeTypes []string
	var joinTypes []string

	// Extract Plan from the JSON structure if available
	actualPlan, ok := node["Plan"].(map[string]interface{})
	if !ok {
		// If not a Plan wrapper, use the node directly
		actualPlan = node
	}

	if nodeType, ok := actualPlan["Node Type"].(string); ok {
		nodeTypes = append(nodeTypes, nodeType)

		// Consider various join operations, including Nested Loop which is also a join
		if strings.Contains(nodeType, "Join") || nodeType == "Nested Loop" {
			joinTypes = append(joinTypes, nodeType)
		}
	}

	// Also check for Join Type field which exists in some PostgreSQL plans
	if joinType, ok := actualPlan["Join Type"].(string); ok && joinType != "" {
		joinStr := fmt.Sprintf("%s Join", joinType)
		if !lo.Contains(joinTypes, joinStr) {
			joinTypes = append(joinTypes, joinStr)
		}
	}

	if plans, ok := actualPlan["Plans"].([]interface{}); ok {
		for _, subPlan := range plans {
			if subPlanMap, ok := subPlan.(map[string]interface{}); ok {
				subNodeTypes, subJoinTypes := analyzePlanNodeTypes(subPlanMap)
				nodeTypes = append(nodeTypes, subNodeTypes...)
				joinTypes = append(joinTypes, subJoinTypes...)
			}
		}
	}

	return nodeTypes, joinTypes
}

func extractPlanNodeMetrics(node map[string]interface{}) (float64, int64) {
	var totalCost float64
	var maxRows int64

	// Extract Plan from the JSON structure if available
	actualPlan, ok := node["Plan"].(map[string]interface{})
	if !ok {
		// If not a Plan wrapper, use the node directly
		actualPlan = node
	}

	if costValue, ok := actualPlan["Total Cost"].(float64); ok {
		totalCost = costValue
	}

	if rowsValue, ok := actualPlan["Plan Rows"].(float64); ok {
		maxRows = int64(rowsValue)
	}

	if plans, ok := actualPlan["Plans"].([]interface{}); ok {
		for _, subPlan := range plans {
			if subPlanMap, ok := subPlan.(map[string]interface{}); ok {
				subCost, subRows := extractPlanNodeMetrics(subPlanMap)
				totalCost += subCost
				if subRows > maxRows {
					maxRows = subRows
				}
			}
		}
	}

	return totalCost, maxRows
}

func compareQueryResults(t *testing.T, name1 string, result1 *QueryPlanResult, name2 string, result2 *QueryPlanResult) {
	planningDiff := calculatePercentDifference(result1.PlanningTime, result2.PlanningTime)
	executionDiff := calculatePercentDifference(result1.ExecutionTime, result2.ExecutionTime)
	totalTimeDiff := calculatePercentDifference(result1.TotalTime, result2.TotalTime)
	costDiff := calculatePercentDifference(result1.TotalCost, result2.TotalCost)

	var winner string
	var timeDiffPercent float64
	if result1.ExecutionTime < result2.ExecutionTime {
		winner = name1
		timeDiffPercent = (result2.ExecutionTime - result1.ExecutionTime) / result1.ExecutionTime * 100
	} else if result2.ExecutionTime < result1.ExecutionTime {
		winner = name2
		timeDiffPercent = (result1.ExecutionTime - result2.ExecutionTime) / result2.ExecutionTime * 100
	} else {
		winner = "Tie"
		timeDiffPercent = 0
	}

	t.Logf("--- %s vs %s ---", name1, name2)
	t.Logf("%s: Planning: %.3fms, Execution: %.3fms, Total: %.3fms, Cost: %.2f, Max Rows: %d, Contains JOIN: %v",
		name1, result1.PlanningTime, result1.ExecutionTime, result1.TotalTime, result1.TotalCost, result1.MaxRows, result1.ContainsJoin)
	if result1.ContainsJoin {
		t.Logf("%s JOIN Types: %v", name1, result1.JoinTypes)
	}

	t.Logf("%s: Planning: %.3fms, Execution: %.3fms, Total: %.3fms, Cost: %.2f, Max Rows: %d, Contains JOIN: %v",
		name2, result2.PlanningTime, result2.ExecutionTime, result2.TotalTime, result2.TotalCost, result2.MaxRows, result2.ContainsJoin)
	if result2.ContainsJoin {
		t.Logf("%s JOIN Types: %v", name2, result2.JoinTypes)
	}

	t.Logf("Difference: Planning: %s, Execution: %s, Total Time: %s, Cost: %s",
		formatPercentage(planningDiff), formatPercentage(executionDiff),
		formatPercentage(totalTimeDiff), formatPercentage(costDiff))

	if winner != "Tie" {
		t.Logf("WINNER: %s is %.2f%% faster in execution time", winner, timeDiffPercent)
	} else {
		t.Log("RESULT: Performance is identical")
	}
	t.Log("-----------------------------------------------------\n")
}

func analyzeQueryPlan(t *testing.T, queryName string, result *QueryPlanResult) {
	t.Logf("--- %s Plan Analysis ---", queryName)
	t.Logf("Contains JOIN operations: %v", result.ContainsJoin)

	if result.ContainsJoin {
		t.Logf("JOIN Types found: %v", result.JoinTypes)
		t.Logf("This suggests PostgreSQL optimized the IN subquery into a JOIN operation")
	} else {
		t.Logf("No JOIN operations found in the execution plan")
		t.Logf("Node Types found: %v", result.NodeTypes)
	}
}

func calculatePercentDifference(val1, val2 float64) float64 {
	if val1 == 0 && val2 == 0 {
		return 0
	}
	if val1 == 0 {
		return 100
	}
	return (val2 - val1) / val1 * 100
}

func formatPercentage(value float64) string {
	if value > 0 {
		return fmt.Sprintf("+%.2f%%", value)
	}
	return fmt.Sprintf("%.2f%%", value)
}

func createTables(t *testing.T, db *gorm.DB) error {
	err := db.Migrator().DropTable(&User{})
	if err != nil {
		return fmt.Errorf("failed to drop users table: %w", err)
	}

	err = db.Migrator().DropTable(&Company{})
	if err != nil {
		return fmt.Errorf("failed to drop companies table: %w", err)
	}

	err = db.AutoMigrate(&Company{})
	if err != nil {
		return fmt.Errorf("failed to create companies table: %w", err)
	}

	err = db.AutoMigrate(&User{})
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	t.Log("Tables created successfully")
	return nil
}

func insertCompanies(t *testing.T, db *gorm.DB, count int) ([]string, error) {
	now := time.Now()
	companyIDs := make([]string, count)

	batchSize := 100
	for i := 0; i < count; i += batchSize {
		end := i + batchSize
		if end > count {
			end = count
		}

		batch := make([]*Company, end-i)
		for j := i; j < end; j++ {
			id := fmt.Sprintf("company-%d", j)
			companyIDs[j] = id
			description := fmt.Sprintf("Description for company %d", j)

			batch[j-i] = &Company{
				ID:          id,
				CreatedAt:   now,
				UpdatedAt:   now,
				Name:        fmt.Sprintf("Company-%d", j),
				Description: &description,
			}
		}

		err := db.CreateInBatches(batch, len(batch)).Error
		if err != nil {
			return nil, fmt.Errorf("failed to insert companies batch: %w", err)
		}
	}

	t.Logf("Inserted %d companies", count)
	return companyIDs, nil
}

func insertUsers(t *testing.T, db *gorm.DB, companyIDs []string, usersPerCompany int, deletePercentage int) error {
	now := time.Now()
	batchSize := 1000
	totalUsers := len(companyIDs) * usersPerCompany
	inserted := 0

	for i := 0; i < totalUsers; i += batchSize {
		end := i + batchSize
		if end > totalUsers {
			end = totalUsers
		}

		batch := make([]*User, end-i)
		for j := i; j < end; j++ {
			companyIndex := j % len(companyIDs)
			companyID := companyIDs[companyIndex]
			userID := fmt.Sprintf("user-%d", j)
			age := rand.Intn(50) + 18
			description := fmt.Sprintf("Description for user %d", j)

			batch[j-i] = &User{
				ID:          userID,
				CreatedAt:   now,
				UpdatedAt:   now,
				Name:        fmt.Sprintf("User-%d", j),
				Description: &description,
				Age:         age,
				CompanyID:   companyID,
			}
		}

		err := db.CreateInBatches(batch, len(batch)).Error
		if err != nil {
			return fmt.Errorf("failed to insert users batch: %w", err)
		}

		inserted += (end - i)
		t.Logf("Inserted %d/%d users", inserted, totalUsers)
	}

	deleteCount := totalUsers * deletePercentage / 100
	err := db.Exec(`
		UPDATE users 
		SET deleted_at = NOW() 
		WHERE id IN (
			SELECT id FROM users 
			ORDER BY RANDOM() 
			LIMIT ?
		)
	`, deleteCount).Error
	if err != nil {
		return fmt.Errorf("failed to mark users as deleted: %w", err)
	}

	t.Logf("Completed inserting %d users across %d companies", totalUsers, len(companyIDs))
	t.Logf("Added deleted_at timestamp to ~%d users (%d%%)", deleteCount, deletePercentage)

	return nil
}
