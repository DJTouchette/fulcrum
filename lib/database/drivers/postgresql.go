package drivers

import (
	"context"
	"database/sql"
	"fmt"
	"fulcrum/lib/database/interfaces"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgreSQLDB implements the Database interface for PostgreSQL
type PostgreSQLDB struct {
	config interfaces.Config
	db     *sql.DB
}

// NewPostgreSQLDB creates a new PostgreSQL database connection
func NewPostgreSQLDB(config interfaces.Config) (interfaces.Database, error) {
	return &PostgreSQLDB{
		config: config,
	}, nil
}

// Connect establishes a connection to PostgreSQL
func (p *PostgreSQLDB) Connect(ctx context.Context) error {
	connStr := p.GetConnectionString()

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Configure connection pool
	if p.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(p.config.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(25) // Default
	}

	if p.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(p.config.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(10) // Default
	}

	if p.config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(p.config.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(5 * time.Minute) // Default
	}

	p.db = db
	return nil
}

// Close closes the database connection
func (p *PostgreSQLDB) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Ping tests the database connection
func (p *PostgreSQLDB) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// Stats returns database connection statistics
func (p *PostgreSQLDB) Stats() sql.DBStats {
	return p.db.Stats()
}

// Query executes a query that returns rows
func (p *PostgreSQLDB) Query(ctx context.Context, query string, args ...interface{}) (interfaces.Rows, error) {
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// QueryRow executes a query that returns at most one row
func (p *PostgreSQLDB) QueryRow(ctx context.Context, query string, args ...interface{}) interfaces.Row {
	row := p.db.QueryRowContext(ctx, query, args...)
	return row
}

// Exec executes a query without returning any rows
func (p *PostgreSQLDB) Exec(ctx context.Context, query string, args ...interface{}) (interfaces.Result, error) {
	result, err := p.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Begin starts a transaction
func (p *PostgreSQLDB) Begin(ctx context.Context) (interfaces.Tx, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLTx{tx: tx}, nil
}

// BeginTx starts a transaction with options
func (p *PostgreSQLDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (interfaces.Tx, error) {
	tx, err := p.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLTx{tx: tx}, nil
}

// CreateTable creates a table with the given schema
func (p *PostgreSQLDB) CreateTable(ctx context.Context, tableName string, schema interfaces.TableSchema) error {
	query := p.buildCreateTableQuery(tableName, schema)
	_, err := p.Exec(ctx, query)
	return err
}

// DropTable drops a table
func (p *PostgreSQLDB) DropTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	_, err := p.Exec(ctx, query)
	return err
}

// TableExists checks if a table exists
func (p *PostgreSQLDB) TableExists(ctx context.Context, tableName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`

	var exists bool
	err := p.QueryRow(ctx, query, tableName).Scan(&exists)
	return exists, err
}

// GetDriver returns the database driver type
func (p *PostgreSQLDB) GetDriver() interfaces.DatabaseDriver {
	return interfaces.DriverPostgreSQL
}

// GetConnectionString builds the PostgreSQL connection string
func (p *PostgreSQLDB) GetConnectionString() string {
	sslMode := p.config.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.config.Host,
		p.config.Port,
		p.config.Username,
		p.config.Password,
		p.config.Database,
		sslMode,
	)
}

// buildCreateTableQuery builds a CREATE TABLE query for PostgreSQL
func (p *PostgreSQLDB) buildCreateTableQuery(tableName string, schema interfaces.TableSchema) string {
	var parts []string

	// Add columns
	var columns []string
	for _, col := range schema.Columns {
		colDef := p.buildColumnDefinition(col)
		columns = append(columns, colDef)
	}

	// Add primary key
	if len(schema.PrimaryKey) > 0 {
		pkCols := strings.Join(schema.PrimaryKey, ", ")
		columns = append(columns, fmt.Sprintf("PRIMARY KEY (%s)", pkCols))
	}

	// Add foreign keys
	for _, fk := range schema.ForeignKeys {
		fkDef := fmt.Sprintf(
			"CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			fk.Name, fk.Column, fk.ReferencedTable, fk.ReferencedColumn,
		)
		if fk.OnDelete != "" {
			fkDef += fmt.Sprintf(" ON DELETE %s", fk.OnDelete)
		}
		if fk.OnUpdate != "" {
			fkDef += fmt.Sprintf(" ON UPDATE %s", fk.OnUpdate)
		}
		columns = append(columns, fkDef)
	}

	parts = append(parts, fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(columns, ", ")))

	return strings.Join(parts, ";\n")
}

func (p *PostgreSQLDB) buildColumnDefinition(col interfaces.ColumnDefinition) string {
	def := fmt.Sprintf("%s %s", col.Name, p.mapDataType(col.Type, nil))

	if col.NotNull { // Changed from !col.Nullable to col.NotNull
		def += " NOT NULL"
	}

	if col.DefaultValue != nil {
		def += fmt.Sprintf(" DEFAULT %s", *col.DefaultValue) // Changed to *col.DefaultValue since it's *string
	}

	return def
}

// mapDataType maps generic data types to PostgreSQL specific types
func (p *PostgreSQLDB) mapDataType(dataType string, length *int) string {
	switch strings.ToLower(dataType) {
	case "text", "string":
		if length != nil {
			return fmt.Sprintf("VARCHAR(%d)", *length)
		}
		return "TEXT"
	case "integer", "int":
		return "INTEGER"
	case "bigint", "int64":
		return "BIGINT"
	case "boolean", "bool":
		return "BOOLEAN"
	case "timestamp", "datetime":
		return "TIMESTAMP"
	case "date":
		return "DATE"
	case "time":
		return "TIME"
	case "decimal", "numeric":
		return "DECIMAL"
	case "float":
		return "REAL"
	case "double":
		return "DOUBLE PRECISION"
	case "uuid":
		return "UUID"
	case "json":
		return "JSON"
	case "jsonb":
		return "JSONB"
	default:
		return strings.ToUpper(dataType)
	}
}

// PostgreSQL-specific wrapper types

// PostgreSQLRows wraps sql.Rows
type PostgreSQLRows struct {
	rows *sql.Rows
}

func (r *PostgreSQLRows) Close() error                   { return r.rows.Close() }
func (r *PostgreSQLRows) Next() bool                     { return r.rows.Next() }
func (r *PostgreSQLRows) Scan(dest ...interface{}) error { return r.rows.Scan(dest...) }
func (r *PostgreSQLRows) Columns() ([]string, error)     { return r.rows.Columns() }
func (r *PostgreSQLRows) Err() error                     { return r.rows.Err() }

// PostgreSQLRow wraps sql.Row
type PostgreSQLRow struct {
	row *sql.Row
}

func (r *PostgreSQLRow) Scan(dest ...interface{}) error { return r.row.Scan(dest...) }

// PostgreSQLResult wraps sql.Result
type PostgreSQLResult struct {
	result sql.Result
}

func (r *PostgreSQLResult) LastInsertId() (int64, error) { return r.result.LastInsertId() }
func (r *PostgreSQLResult) RowsAffected() (int64, error) { return r.result.RowsAffected() }

// PostgreSQLTx wraps sql.Tx
type PostgreSQLTx struct {
	tx *sql.Tx
}

func (t *PostgreSQLTx) Query(ctx context.Context, query string, args ...interface{}) (interfaces.Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (t *PostgreSQLTx) QueryRow(ctx context.Context, query string, args ...interface{}) interfaces.Row {
	row := t.tx.QueryRowContext(ctx, query, args...)
	return row
}

func (t *PostgreSQLTx) Exec(ctx context.Context, query string, args ...interface{}) (interfaces.Result, error) {
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLResult{result: result}, nil
}

func (t *PostgreSQLTx) Commit() error   { return t.tx.Commit() }
func (t *PostgreSQLTx) Rollback() error { return t.tx.Rollback() }
