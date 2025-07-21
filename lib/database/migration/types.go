package migration

import "time"

// Migration represents a single migration file
type Migration struct {
	Version     int                    `yaml:"version"`
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Up          []MigrationOperation   `yaml:"up"`
	Down        []MigrationOperation   `yaml:"down"`
	Domain      string                 // Set during parsing
	FilePath    string                 // Set during parsing
}

// MigrationOperation represents a single operation in a migration
type MigrationOperation struct {
	CreateTable   *CreateTableOp   `yaml:"create_table,omitempty"`
	DropTable     *DropTableOp     `yaml:"drop_table,omitempty"`
	AddColumn     *AddColumnOp     `yaml:"add_column,omitempty"`
	DropColumn    *DropColumnOp    `yaml:"drop_column,omitempty"`
	ChangeColumn  *ChangeColumnOp  `yaml:"change_column,omitempty"`
	AddIndex      *AddIndexOp      `yaml:"add_index,omitempty"`
	DropIndex     *DropIndexOp     `yaml:"drop_index,omitempty"`
	AddForeignKey *AddForeignKeyOp `yaml:"add_foreign_key,omitempty"`
	DropForeignKey *DropForeignKeyOp `yaml:"drop_foreign_key,omitempty"`
	Execute       *ExecuteOp       `yaml:"execute,omitempty"`
}

// CreateTableOp creates a new table
type CreateTableOp struct {
	Name    string             `yaml:"name"`
	Columns []MigrationColumn  `yaml:"columns"`
}

// DropTableOp drops an existing table
type DropTableOp struct {
	Name string `yaml:"name"`
}

// AddColumnOp adds a column to an existing table
type AddColumnOp struct {
	Table  string          `yaml:"table"`
	Name   string          `yaml:"name"`
	Type   string          `yaml:"type"`
	Length *int            `yaml:"length,omitempty"`
	Nullable bool          `yaml:"nullable,omitempty"`
	Default interface{}    `yaml:"default,omitempty"`
	Unique  bool           `yaml:"unique,omitempty"`
}

// DropColumnOp drops a column from a table
type DropColumnOp struct {
	Table string `yaml:"table"`
	Name  string `yaml:"name"`
}

// ChangeColumnOp modifies an existing column
type ChangeColumnOp struct {
	Table    string       `yaml:"table"`
	Name     string       `yaml:"name"`
	Type     string       `yaml:"type,omitempty"`
	Length   *int         `yaml:"length,omitempty"`
	Nullable *bool        `yaml:"nullable,omitempty"`
	Default  interface{}  `yaml:"default,omitempty"`
}

// AddIndexOp adds an index to a table
type AddIndexOp struct {
	Table   string   `yaml:"table"`
	Columns []string `yaml:"columns"`
	Name    string   `yaml:"name,omitempty"`
	Unique  bool     `yaml:"unique,omitempty"`
}

// DropIndexOp drops an index
type DropIndexOp struct {
	Name string `yaml:"name"`
}

// AddForeignKeyOp adds a foreign key constraint
type AddForeignKeyOp struct {
	Table           string `yaml:"table"`
	Column          string `yaml:"column"`
	ReferencedTable string `yaml:"referenced_table"`
	ReferencedColumn string `yaml:"referenced_column"`
	Name            string `yaml:"name,omitempty"`
	OnDelete        string `yaml:"on_delete,omitempty"`
	OnUpdate        string `yaml:"on_update,omitempty"`
}

// DropForeignKeyOp drops a foreign key constraint
type DropForeignKeyOp struct {
	Name string `yaml:"name"`
}

// ExecuteOp executes raw SQL
type ExecuteOp struct {
	SQL string `yaml:"sql"`
}

// MigrationColumn represents a column in a CREATE TABLE operation
type MigrationColumn struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"`
	Length     *int        `yaml:"length,omitempty"`
	Nullable   bool        `yaml:"nullable,omitempty"`
	Default    interface{} `yaml:"default,omitempty"`
	PrimaryKey bool        `yaml:"primary_key,omitempty"`
	Unique     bool        `yaml:"unique,omitempty"`
}

// MigrationRecord represents a migration that has been applied
type MigrationRecord struct {
	Version   int       `json:"version"`
	Domain    string    `json:"domain"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
}

// MigrationStatus represents the status of migrations
type MigrationStatus struct {
	Domain            string            `json:"domain"`
	PendingMigrations []Migration       `json:"pending_migrations"`
	AppliedMigrations []MigrationRecord `json:"applied_migrations"`
}
