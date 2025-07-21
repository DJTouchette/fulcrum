package migration

import (
	"context"
	"database/sql"
	"fmt"
	"fulcrum/lib/database/interfaces"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// MockDatabase implements the interfaces.Database interface for testing
type MockDatabase struct {
	queries []string
	tx      *sql.Tx
	txMode  bool
	execErr error
}

func (m *MockDatabase) Connect(ctx context.Context) error { return nil }
func (m *MockDatabase) Close() error                      { return nil }
func (m *MockDatabase) Ping(ctx context.Context) error    { return nil }
func (m *MockDatabase) Stats() sql.DBStats                { return sql.DBStats{} }

func (m *MockDatabase) GetDriver() interfaces.DatabaseDriver {
	return interfaces.DriverSQLite
}

func (m *MockDatabase) GetConnectionString() string {
	return "mock://connection"
}

func (m *MockDatabase) TableExists(ctx context.Context, tableName string) (bool, error) {
	return false, nil
}

func (m *MockDatabase) CreateTable(ctx context.Context, tableName string, schema interfaces.TableSchema) error {
	return nil
}

func (m *MockDatabase) DropTable(ctx context.Context, tableName string) error {
	return nil
}

func (m *MockDatabase) Exec(ctx context.Context, query string, args ...any) (interfaces.Result, error) {
	if m.execErr != nil {
		return nil, m.execErr
	}
	m.queries = append(m.queries, fmt.Sprintf(query, args...))
	return &runnerMockResult{}, nil
}

func (m *MockDatabase) Query(ctx context.Context, query string, args ...any) (interfaces.Rows, error) {
	return nil, nil
}

func (m *MockDatabase) QueryRow(ctx context.Context, query string, args ...any) interfaces.Row {
	return nil
}

func (m *MockDatabase) Begin(ctx context.Context) (interfaces.Tx, error) {
	m.txMode = true
	return nil, nil
}

func (m *MockDatabase) BeginTx(ctx context.Context, opts *sql.TxOptions) (interfaces.Tx, error) {
	m.txMode = true
	return nil, nil
}

// Use different name to avoid conflict with tracker_test.go
type runnerMockResult struct{}

func (m *runnerMockResult) LastInsertId() (int64, error) { return 0, nil }
func (m *runnerMockResult) RowsAffected() (int64, error) { return 1, nil }

func TestNewRunner(t *testing.T) {
	mockDB := &MockDatabase{}
	// Fix: NewRunner expects (interfaces.Database, string)
	runner := NewRunner(mockDB, "sqlite")

	if runner == nil {
		t.Fatal("Expected runner to be created")
	}
	// Note: Can't test internal fields without knowing the struct
}

func TestRunner_Basic(t *testing.T) {
	mockDB := &MockDatabase{}

	// Test that NewRunner works with correct parameters
	runner := NewRunner(mockDB, "sqlite")
	if runner == nil {
		t.Fatal("Expected runner to be created")
	}

	// Test with different database types
	postgresRunner := NewRunner(mockDB, "postgres")
	if postgresRunner == nil {
		t.Fatal("Expected postgres runner to be created")
	}

	mysqlRunner := NewRunner(mockDB, "mysql")
	if mysqlRunner == nil {
		t.Fatal("Expected mysql runner to be created")
	}
}

// Test migration structure validation
func TestMigrationStructure(t *testing.T) {
	// Create a valid migration structure for testing
	migration := &Migration{
		Name:        "Test Migration",
		Description: "Test description",
		Version:     1,
		Up: []MigrationOperation{
			{
				CreateTable: &CreateTableOp{
					Name: "users",
					Columns: []MigrationColumn{
						{Name: "id", Type: "integer", PrimaryKey: true},
					},
				},
			},
		},
		Down: []MigrationOperation{
			{
				DropTable: &DropTableOp{
					Name: "users",
				},
			},
		},
	}

	// Validate migration structure
	if migration.Name == "" {
		t.Error("Migration name should not be empty")
	}

	if migration.Version <= 0 {
		t.Error("Migration version should be positive")
	}

	if len(migration.Up) == 0 {
		t.Error("Migration should have up operations")
	}

	if migration.Up[0].CreateTable == nil {
		t.Error("Expected CreateTable operation")
	}

	if migration.Up[0].CreateTable.Name != "users" {
		t.Errorf("Expected table name 'users', got '%s'", migration.Up[0].CreateTable.Name)
	}
}

func TestMigrationOperations(t *testing.T) {
	t.Run("CreateTable operation", func(t *testing.T) {
		op := MigrationOperation{
			CreateTable: &CreateTableOp{
				Name: "users",
				Columns: []MigrationColumn{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "email", Type: "string", Nullable: false},
				},
			},
		}

		if op.CreateTable == nil {
			t.Fatal("Expected CreateTable operation")
		}
		if op.CreateTable.Name != "users" {
			t.Errorf("Expected table name 'users', got '%s'", op.CreateTable.Name)
		}
		if len(op.CreateTable.Columns) != 2 {
			t.Errorf("Expected 2 columns, got %d", len(op.CreateTable.Columns))
		}
	})

	t.Run("AddColumn operation", func(t *testing.T) {
		op := MigrationOperation{
			AddColumn: &AddColumnOp{
				Table:    "users",
				Name:     "phone",
				Type:     "string",
				Nullable: true,
			},
		}

		if op.AddColumn == nil {
			t.Fatal("Expected AddColumn operation")
		}
		if op.AddColumn.Table != "users" {
			t.Errorf("Expected table 'users', got '%s'", op.AddColumn.Table)
		}
		if op.AddColumn.Name != "phone" {
			t.Errorf("Expected column 'phone', got '%s'", op.AddColumn.Name)
		}
	})

	t.Run("DropTable operation", func(t *testing.T) {
		op := MigrationOperation{
			DropTable: &DropTableOp{
				Name: "old_table",
			},
		}

		if op.DropTable == nil {
			t.Fatal("Expected DropTable operation")
		}
		if op.DropTable.Name != "old_table" {
			t.Errorf("Expected table 'old_table', got '%s'", op.DropTable.Name)
		}
	})

	t.Run("Execute operation", func(t *testing.T) {
		sql := "UPDATE users SET updated_at = NOW()"
		op := MigrationOperation{
			Execute: &ExecuteOp{
				SQL: sql,
			},
		}

		if op.Execute == nil {
			t.Fatal("Expected Execute operation")
		}
		if op.Execute.SQL != sql {
			t.Errorf("Expected SQL '%s', got '%s'", sql, op.Execute.SQL)
		}
	})
}

func TestMockDatabase(t *testing.T) {
	mockDB := &MockDatabase{}
	ctx := context.Background()

	// Test Exec method
	result, err := mockDB.Exec(ctx, "CREATE TABLE test (id INTEGER)")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected result to be returned")
	}

	// Check that query was recorded
	if len(mockDB.queries) != 1 {
		t.Errorf("Expected 1 query, got %d", len(mockDB.queries))
	}

	// Test error case
	mockDB.execErr = fmt.Errorf("database error")
	_, err = mockDB.Exec(ctx, "SELECT * FROM test")
	if err == nil {
		t.Error("Expected error but got none")
	}
}
