// Package interfaces defines the core database interfaces used by drivers and clients
package interfaces

import (
	"context"
	"database/sql"
	"time"
)

// DatabaseDriver represents supported database drivers
type DatabaseDriver string

const (
	DriverPostgreSQL DatabaseDriver = "postgresql"
	DriverMySQL      DatabaseDriver = "mysql"
	DriverSQLite     DatabaseDriver = "sqlite"
)

// Config holds database configuration
type Config struct {
	Driver          DatabaseDriver
	Host            string
	Port            int
	Username        string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	FilePath        string
}

// Database interface defines the main database operations
type Database interface {
	Connect(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
	Stats() sql.DBStats
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) Row
	Exec(ctx context.Context, query string, args ...any) (Result, error)
	Begin(ctx context.Context) (Tx, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
	CreateTable(ctx context.Context, tableName string, schema TableSchema) error
	DropTable(ctx context.Context, tableName string) error
	TableExists(ctx context.Context, tableName string) (bool, error)
	GetDriver() DatabaseDriver
	GetConnectionString() string
}

// Rows interface wraps sql.Rows
type Rows interface {
	Close() error
	ColumnTypes() ([]*sql.ColumnType, error)
	Columns() ([]string, error)
	Err() error
	Next() bool
	NextResultSet() bool
	Scan(dest ...any) error
}

// Row interface wraps sql.Row
type Row interface {
	Err() error
	Scan(dest ...any) error
}

// Result interface wraps sql.Result
type Result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// Tx interface wraps sql.Tx
type Tx interface {
	Commit() error
	Rollback() error
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) Row
	Exec(ctx context.Context, query string, args ...any) (Result, error)
}

// TableSchema represents a database table schema
type TableSchema struct {
	Columns     []ColumnDefinition
	PrimaryKey  []string
	ForeignKeys []ForeignKey
	Indexes     []Index
}

// ColumnDefinition represents a table column
type ColumnDefinition struct {
	Name          string
	Type          string
	NotNull       bool
	DefaultValue  *string
	AutoIncrement bool
}

// ForeignKey represents a foreign key constraint
type ForeignKey struct {
	Name             string
	Column           string
	ReferencedTable  string
	ReferencedColumn string
	OnDelete         string
	OnUpdate         string
}

// Index represents a table index
type Index struct {
	Name    string
	Columns []string
	Unique  bool
}

// ColumnSchema represents a column definition
type ColumnSchema struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Nullable     bool   `json:"nullable"`
	DefaultValue any    `json:"default_value"`
	Length       *int   `json:"length,omitempty"`
	Precision    *int   `json:"precision,omitempty"`
	Scale        *int   `json:"scale,omitempty"`
}
