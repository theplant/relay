package gormrelay_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/theplant/relay/cursor"
	"github.com/theplant/relay/gormrelay"
)

func TestWithComputedResult(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	tests := []struct {
		name           string
		object         any
		computedFields map[string]any
		expected       string
	}{
		{
			name: "basic object with no computed fields",
			object: User{
				ID:   1,
				Name: "John",
			},
			computedFields: map[string]any{},
			expected:       `{"ID":1,"Name":"John"}`,
		},
		{
			name: "object with computed fields",
			object: User{
				ID:   1,
				Name: "John",
			},
			computedFields: map[string]any{
				"extraField": "extra value",
				"IsAdmin":    true,
				"score":      99.5,
			},
			expected: `{"extraField":"extra value","ID":1,"IsAdmin":true,"Name":"John","score":99.5}`,
		},
		{
			name: "object with nested computed fields",
			object: User{
				ID:   1,
				Name: "John",
			},
			computedFields: map[string]any{
				"profile.avatar": "avatar.jpg",
				"profile.Role":   "admin",
			},
			expected: `{"ID":1,"Name":"John","profile":{"avatar":"avatar.jpg","Role":"admin"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := gormrelay.WithComputedResult(tt.object, tt.computedFields)

			b, err := cursor.JSONMarshal(v)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(b))
		})
	}
}

// TestComputedValidate verifies the validation logic of the Computed struct.
// It tests various invalid configurations to ensure they are properly rejected:
// - Empty Columns map
// - Nil ForScan function
// - Columns with pre-defined Alias (which should be set during query execution)
// - Duplicate column aliases that would cause SQL errors
func TestComputedValidate(t *testing.T) {
	// Mock ForScan function for testing
	mockForScan := func(db *gorm.DB) (dest any, toCursorNodes func(computedResults []map[string]any) []cursor.Node[*struct{}], err error) {
		return nil, nil, nil
	}

	tests := []struct {
		name        string
		computed    *gormrelay.Computed[*struct{}]
		expectError bool
		errorSubstr string
	}{
		{
			name: "valid computed",
			computed: &gormrelay.Computed[*struct{}]{
				Columns: map[string]clause.Column{
					"TotalCount": {Name: "(COUNT(*))", Raw: true},
				},
				ForScan: mockForScan,
			},
			expectError: false,
		},
		{
			name: "empty columns",
			computed: &gormrelay.Computed[*struct{}]{
				Columns: map[string]clause.Column{},
				ForScan: mockForScan,
			},
			expectError: true,
			errorSubstr: "Columns must not be empty",
		},
		{
			name: "nil ForScan",
			computed: &gormrelay.Computed[*struct{}]{
				Columns: map[string]clause.Column{
					"TotalCount": {Name: "(COUNT(*))", Raw: true},
				},
				ForScan: nil,
			},
			expectError: true,
			errorSubstr: "ForScan function must not be nil",
		},
		{
			name: "column with non-empty alias",
			computed: &gormrelay.Computed[*struct{}]{
				Columns: map[string]clause.Column{
					"TotalCount": {Name: "(COUNT(*))", Raw: true, Alias: "already_set"},
				},
				ForScan: mockForScan,
			},
			expectError: true,
			errorSubstr: "should have empty Alias",
		},
		{
			name: "duplicate column alias",
			computed: &gormrelay.Computed[*struct{}]{
				Columns: map[string]clause.Column{
					"UserScore":  {Name: "(AVG(score))", Raw: true},
					"user_score": {Name: "(SUM(score)/COUNT(*))", Raw: true},
				},
				ForScan: mockForScan,
			},
			expectError: true,
			errorSubstr: "duplicate computed field aliases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.computed.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDefaultForScan verifies the DefaultForScan function correctly creates a ForScan
// function that combines entities with their computed results.
func TestDefaultForScan(t *testing.T) {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	// Create test data
	users := []*User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}

	// Create computed results
	computedResults := []map[string]any{
		{"score": 95, "IsAdmin": true},
		{"score": 85, "IsAdmin": false},
	}

	db := (&gorm.DB{Statement: &gorm.Statement{}}).Model(&User{})

	// Test 1: Model type matches generic type
	dest1, toCursorNodes1, err := gormrelay.DefaultForScan[*User](db)
	require.NoError(t, err)

	// Check destination type when model matches generic type
	userSlicePtr1, ok := dest1.(*[]*User)
	require.True(t, ok, "dest should be *[]*User")

	// Populate user slice
	*userSlicePtr1 = users

	// Convert to cursor nodes
	nodes1 := toCursorNodes1(computedResults)
	require.Len(t, nodes1, 2)

	// Check first node
	obj1, err := cursor.JSONMarshal(nodes1[0])
	require.NoError(t, err)
	assert.Contains(t, string(obj1), `"ID":1`)
	assert.Contains(t, string(obj1), `"Name":"Alice"`)
	assert.Contains(t, string(obj1), `"score":95`)
	assert.Contains(t, string(obj1), `"IsAdmin":true`)

	// Check second node
	obj2, err := cursor.JSONMarshal(nodes1[1])
	require.NoError(t, err)
	assert.Contains(t, string(obj2), `"ID":2`)
	assert.Contains(t, string(obj2), `"Name":"Bob"`)
	assert.Contains(t, string(obj2), `"score":85`)
	assert.Contains(t, string(obj2), `"IsAdmin":false`)

	// Verify unwrap functionality through type assertion
	nodeWrapper1, ok := nodes1[0].(*cursor.NodeWrapper[*User])
	require.True(t, ok, "should be a NodeWrapper")
	user1 := nodeWrapper1.Unwrap()
	assert.Equal(t, 1, user1.ID)
	assert.Equal(t, "Alice", user1.Name)

	nodeWrapper2, ok := nodes1[1].(*cursor.NodeWrapper[*User])
	require.True(t, ok, "should be a NodeWrapper")
	user2 := nodeWrapper2.Unwrap()
	assert.Equal(t, 2, user2.ID)
	assert.Equal(t, "Bob", user2.Name)

	// Test 2: Model type does NOT match generic type
	dest2, toCursorNodes2, err := gormrelay.DefaultForScan[any](db)
	require.NoError(t, err)

	// Populate result slice with compatible nodes
	sliceValue := reflect.ValueOf(dest2).Elem()
	for _, user := range users {
		u := &User{ID: user.ID, Name: user.Name}
		sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(u)))
	}

	// Convert to cursor nodes
	nodes2 := toCursorNodes2(computedResults)
	require.Len(t, nodes2, 2)

	// Verify nodes
	anyNodeWrapper, ok := nodes2[0].(*cursor.NodeWrapper[any])
	require.True(t, ok)
	require.NotNil(t, anyNodeWrapper)
	assert.Equal(t, 1, anyNodeWrapper.Unwrap().(*User).ID)
	assert.Equal(t, "Alice", anyNodeWrapper.Unwrap().(*User).Name)
}
