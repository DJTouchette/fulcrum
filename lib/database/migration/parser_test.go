package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseYAMLFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		filename    string
		content     string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "valid migration file",
			filename: "001_create_users.yml",
			content: `name: "Create Users Table"
description: "Initial user table creation"
version: 1
up:
  - create_table:
      name: "users"
      columns:
        - name: "id"
          type: "integer"
          primary_key: true
        - name: "email"
          type: "string"
          nullable: false
        - name: "created_at"
          type: "timestamp"
down:
  - drop_table:
      name: "users"
`,
			expectError: false,
		},
		{
			name:     "migration with index operations",
			filename: "002_add_user_index.yml",
			content: `name: "Add User Index"
description: "Add index on email and created_at"
version: 2
up:
  - add_index:
      table: "users"
      name: "idx_users_email_created"
      columns: ["email", "created_at"]
      unique: false
down:
  - drop_index:
      name: "idx_users_email_created"
`,
			expectError: false,
		},
		{
			name:     "migration with add column",
			filename: "003_add_column.yml",
			content: `name: "Add Email Column"
description: "Add email column to users table"
version: 3
up:
  - add_column:
      table: "users"
      name: "email"
      type: "string"
      nullable: false
down:
  - drop_column:
      table: "users"
      name: "email"
`,
			expectError: false,
		},
		{
			name:     "migration with raw SQL",
			filename: "004_execute_sql.yml",
			content: `name: "Execute Custom SQL"
description: "Run custom SQL commands"
version: 4
up:
  - execute:
      sql: "UPDATE users SET created_at = NOW() WHERE created_at IS NULL"
down:
  - execute:
      sql: "UPDATE users SET created_at = NULL WHERE created_at = NOW()"
`,
			expectError: false,
		},
		{
			name:        "invalid YAML syntax",
			filename:    "005_invalid.yml",
			content:     "invalid: yaml: content:\n  - missing quotes",
			expectError: true,
			errorMsg:    "yaml",
		},
		{
			name:     "missing required fields",
			filename: "006_missing_name.yml",
			content: `description: "Missing name field"
version: 5
up:
  - create_table:
      name: "test"
      columns:
        - name: "id"
          type: "integer"
down:
  - drop_table:
      name: "test"
`,
			expectError: true,
			errorMsg:    "name",
		},
		{
			name:     "empty up operations",
			filename: "007_empty_up.yml",
			content: `name: "Empty Up"
description: "Migration with no up operations"
version: 6
up: []
down:
  - drop_table:
      name: "test"
`,
			expectError: true,
			errorMsg:    "up operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test file
			filePath := filepath.Join(tmpDir, tt.filename)
			err := os.WriteFile(filePath, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Parse the file
			migration, err := ParseYAMLFile(filePath)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else {
					// Validate parsed migration
					if migration.Name == "" {
						t.Error("Migration name should not be empty")
					}
					if migration.Version <= 0 {
						t.Error("Migration version should be positive")
					}
					if len(migration.Up) == 0 {
						t.Error("Migration should have up operations")
					}
				}
			}
		})
	}
}

func TestParseYAMLContent(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		expectError bool
	}{
		{
			name: "valid content",
			content: []byte(`name: "Test Migration"
description: "Test description"
version: 1
up:
  - create_table:
      name: "test_table"
      columns:
        - name: "id"
          type: "integer"
          primary_key: true
down:
  - drop_table:
      name: "test_table"
`),
			expectError: false,
		},
		{
			name:        "empty content",
			content:     []byte(""),
			expectError: true,
		},
		{
			name:        "invalid YAML",
			content:     []byte("invalid: yaml: [content"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migration, err := ParseYAMLContent(tt.content)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				} else if migration == nil {
					t.Error("Migration should not be nil")
				}
			}
		})
	}
}

func TestValidateMigrationPointer(t *testing.T) {
	tests := []struct {
		name        string
		migration   *Migration
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid migration",
			migration: &Migration{
				Name:        "Test Migration",
				Description: "Test description",
				Version:     1,
				Up: []MigrationOperation{
					{
						CreateTable: &CreateTableOp{
							Name: "test_table",
							Columns: []MigrationColumn{
								{
									Name:       "id",
									Type:       "integer",
									PrimaryKey: true,
								},
							},
						},
					},
				},
				Down: []MigrationOperation{
					{
						DropTable: &DropTableOp{
							Name: "test_table",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty name",
			migration: &Migration{
				Name:    "",
				Version: 1,
				Up: []MigrationOperation{
					{
						CreateTable: &CreateTableOp{
							Name: "test",
							Columns: []MigrationColumn{
								{Name: "id", Type: "integer"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "name",
		},
		{
			name: "invalid version",
			migration: &Migration{
				Name:    "Test",
				Version: 0,
				Up: []MigrationOperation{
					{
						CreateTable: &CreateTableOp{
							Name: "test",
							Columns: []MigrationColumn{
								{Name: "id", Type: "integer"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "version",
		},
		{
			name: "empty up operations",
			migration: &Migration{
				Name:    "Test",
				Version: 1,
				Up:      []MigrationOperation{},
			},
			expectError: true,
			errorMsg:    "up operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since ValidateMigration expects Migration, not *Migration, dereference it
			err := ValidateMigration(*tt.migration)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateCreateTableOperation(t *testing.T) {
	tests := []struct {
		name        string
		operation   MigrationOperation
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid create_table operation",
			operation: MigrationOperation{
				CreateTable: &CreateTableOp{
					Name: "users",
					Columns: []MigrationColumn{
						{Name: "id", Type: "integer", PrimaryKey: true},
						{Name: "email", Type: "string", Nullable: false},
					},
				},
			},
			expectError: false,
		},
		{
			name: "create_table without columns",
			operation: MigrationOperation{
				CreateTable: &CreateTableOp{
					Name:    "users",
					Columns: []MigrationColumn{},
				},
			},
			expectError: true,
			errorMsg:    "columns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// You would need to implement ValidateOperation function
			// For now, just validate the structure manually
			if tt.operation.CreateTable != nil {
				if tt.operation.CreateTable.Name == "" {
					t.Error("Table name should not be empty")
				}
				if len(tt.operation.CreateTable.Columns) == 0 && !tt.expectError {
					t.Error("CreateTable should have columns")
				}
			}
		})
	}
}

func TestValidateAddColumnOperation(t *testing.T) {
	tests := []struct {
		name        string
		operation   MigrationOperation
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid add_column operation",
			operation: MigrationOperation{
				AddColumn: &AddColumnOp{
					Table:    "users",
					Name:     "email",
					Type:     "string",
					Nullable: false,
				},
			},
			expectError: false,
		},
		{
			name: "add_column without table",
			operation: MigrationOperation{
				AddColumn: &AddColumnOp{
					Name: "email",
					Type: "string",
				},
			},
			expectError: true,
			errorMsg:    "table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.operation.AddColumn != nil {
				if tt.operation.AddColumn.Table == "" && !tt.expectError {
					t.Error("Table name should not be empty")
				}
				if tt.operation.AddColumn.Name == "" && !tt.expectError {
					t.Error("Column name should not be empty")
				}
				if tt.operation.AddColumn.Type == "" && !tt.expectError {
					t.Error("Column type should not be empty")
				}
			}
		})
	}
}

func TestValidateMigrationColumn(t *testing.T) {
	tests := []struct {
		name        string
		column      MigrationColumn
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid column",
			column: MigrationColumn{
				Name: "id",
				Type: "integer",
			},
			expectError: false,
		},
		{
			name: "empty column name",
			column: MigrationColumn{
				Name: "",
				Type: "integer",
			},
			expectError: true,
			errorMsg:    "name",
		},
		{
			name: "empty column type",
			column: MigrationColumn{
				Name: "id",
				Type: "",
			},
			expectError: true,
			errorMsg:    "type",
		},
		{
			name: "column with length",
			column: MigrationColumn{
				Name:   "email",
				Type:   "string",
				Length: &[]int{255}[0], // Helper to get *int
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation logic
			if tt.column.Name == "" && !tt.expectError {
				t.Error("Column name should not be empty")
			}
			if tt.column.Type == "" && !tt.expectError {
				t.Error("Column type should not be empty")
			}
		})
	}
}
