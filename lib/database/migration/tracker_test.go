package migration

import (
	"fmt"
	"testing"
)

// ValidateMigration validates a migration structure
func ValidateMigration(migration Migration) error {
	if migration.Domain == "" {
		return fmt.Errorf("migration domain cannot be empty")
	}
	if migration.Name == "" {
		return fmt.Errorf("migration name cannot be empty")
	}
	if migration.Version <= 0 {
		return fmt.Errorf("migration version must be positive")
	}
	return nil
}

func testParseExample(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    Migration
		expectError bool
	}{
		{
			name: "valid create table migration",
			input: `{
				"version": 1,
				"domain": "users", 
				"name": "create_users_table",
				"up": [
					{
						"create_table": {
							"name": "users",
							"columns": [
								{
									"name": "id",
									"type": "integer",
									"primary_key": true
								},
								{
									"name": "email",
									"type": "string",
									"nullable": false
								}
							]
						}
					}
				]
			}`,
			expected: Migration{
				Version: 1,
				Domain:  "users",
				Name:    "create_users_table",
				Up: []MigrationOperation{
					{
						CreateTable: &CreateTableOp{
							Name: "users",
							Columns: []MigrationColumn{
								{
									Name:       "id",
									Type:       "integer",
									PrimaryKey: true,
								},
								{
									Name:     "email",
									Type:     "string",
									Nullable: false,
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "add column migration",
			expected: Migration{
				Version: 2,
				Domain:  "users",
				Name:    "add_email_column",
				Up: []MigrationOperation{
					{
						AddColumn: &AddColumnOp{
							Table:    "users",
							Name:     "email",
							Type:     "string",
							Nullable: false,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "drop table migration",
			expected: Migration{
				Version: 3,
				Domain:  "users",
				Name:    "drop_old_table",
				Up: []MigrationOperation{
					{
						DropTable: &DropTableOp{
							Name: "old_users",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "execute raw SQL migration",
			expected: Migration{
				Version: 4,
				Domain:  "users",
				Name:    "custom_sql",
				Up: []MigrationOperation{
					{
						Execute: &ExecuteOp{
							SQL: "UPDATE users SET created_at = NOW() WHERE created_at IS NULL",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "add index migration",
			expected: Migration{
				Version: 5,
				Domain:  "users",
				Name:    "add_email_index",
				Up: []MigrationOperation{
					{
						AddIndex: &AddIndexOp{
							Table:   "users",
							Columns: []string{"email"},
							Name:    "idx_users_email",
							Unique:  true,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty migration",
			expected: Migration{
				Version: 6,
				Domain:  "users",
				Name:    "empty_migration",
				Up:      []MigrationOperation{},
			},
			expectError: false,
		},
	}

	// Test the ValidateMigration function correctly:
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMigration(tt.expected)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func testValidateMigration(t *testing.T) {
	tests := []struct {
		name        string
		migration   Migration
		expectError bool
	}{
		{
			name: "valid migration",
			migration: Migration{
				Version: 1,
				Domain:  "users",
				Name:    "create_table",
			},
			expectError: false,
		},
		{
			name: "missing domain",
			migration: Migration{
				Version: 1,
				Name:    "create_table",
				// Domain is missing
			},
			expectError: true,
		},
		{
			name: "missing name",
			migration: Migration{
				Version: 1,
				Domain:  "users",
				// Name is missing
			},
			expectError: true,
		},
		{
			name: "invalid version",
			migration: Migration{
				Version: 0, // Invalid version
				Domain:  "users",
				Name:    "create_table",
			},
			expectError: true,
		},
		{
			name: "negative version",
			migration: Migration{
				Version: -1, // Invalid version
				Domain:  "users",
				Name:    "create_table",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMigration(tt.migration)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func testMigrationColumn(t *testing.T) {
	col := MigrationColumn{
		Name:       "id",
		Type:       "integer",
		PrimaryKey: true,
		Nullable:   false,
	}

	if col.Name != "id" {
		t.Errorf("Expected column name 'id', got '%s'", col.Name)
	}
	if !col.PrimaryKey {
		t.Error("Expected column to be primary key")
	}
	if col.Nullable {
		t.Error("Expected column to be not nullable")
	}
}

func testMigrationOperations(t *testing.T) {
	t.Run("CreateTableOp", func(t *testing.T) {
		op := MigrationOperation{
			CreateTable: &CreateTableOp{
				Name: "users",
				Columns: []MigrationColumn{
					{
						Name:       "id",
						Type:       "integer",
						PrimaryKey: true,
					},
				},
			},
		}

		if op.CreateTable == nil {
			t.Fatal("Expected CreateTable operation to be set")
		}
		if op.CreateTable.Name != "users" {
			t.Errorf("Expected table name 'users', got '%s'", op.CreateTable.Name)
		}
	})

	t.Run("AddColumnOp", func(t *testing.T) {
		op := MigrationOperation{
			AddColumn: &AddColumnOp{
				Table:    "users",
				Name:     "email",
				Type:     "string",
				Nullable: false,
			},
		}

		if op.AddColumn == nil {
			t.Fatal("Expected AddColumn operation to be set")
		}
		if op.AddColumn.Table != "users" {
			t.Errorf("Expected table 'users', got '%s'", op.AddColumn.Table)
		}
		if op.AddColumn.Name != "email" {
			t.Errorf("Expected column name 'email', got '%s'", op.AddColumn.Name)
		}
	})

	t.Run("DropTableOp", func(t *testing.T) {
		op := MigrationOperation{
			DropTable: &DropTableOp{
				Name: "old_table",
			},
		}

		if op.DropTable == nil {
			t.Fatal("Expected DropTable operation to be set")
		}
		if op.DropTable.Name != "old_table" {
			t.Errorf("Expected table name 'old_table', got '%s'", op.DropTable.Name)
		}
	})

	t.Run("ExecuteOp", func(t *testing.T) {
		sql := "CREATE INDEX idx_users_email ON users(email)"
		op := MigrationOperation{
			Execute: &ExecuteOp{
				SQL: sql,
			},
		}

		if op.Execute == nil {
			t.Fatal("Expected Execute operation to be set")
		}
		if op.Execute.SQL != sql {
			t.Errorf("Expected SQL '%s', got '%s'", sql, op.Execute.SQL)
		}
	})
}
