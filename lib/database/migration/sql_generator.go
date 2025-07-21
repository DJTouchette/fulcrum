package migration

import (
	"fmt"
	"strings"

	"fulcrum/lib/database/interfaces"
)

// SQLGenerator generates SQL statements from migration operations
type SQLGenerator struct {
	driver interfaces.DatabaseDriver
}

// NewSQLGenerator creates a new SQL generator for the specified database driver
func NewSQLGenerator(driver interfaces.DatabaseDriver) *SQLGenerator {
	return &SQLGenerator{
		driver: driver,
	}
}

// GenerateSQL generates SQL for a migration operation
func (g *SQLGenerator) GenerateSQL(operation *MigrationOperation) (string, error) {
	switch {
	case operation.CreateTable != nil:
		return g.generateCreateTable(operation.CreateTable)
	case operation.DropTable != nil:
		return g.generateDropTable(operation.DropTable)
	case operation.AddColumn != nil:
		return g.generateAddColumn(operation.AddColumn)
	case operation.DropColumn != nil:
		return g.generateDropColumn(operation.DropColumn)
	case operation.ChangeColumn != nil:
		return g.generateChangeColumn(operation.ChangeColumn)
	case operation.AddIndex != nil:
		return g.generateAddIndex(operation.AddIndex)
	case operation.DropIndex != nil:
		return g.generateDropIndex(operation.DropIndex)
	case operation.AddForeignKey != nil:
		return g.generateAddForeignKey(operation.AddForeignKey)
	case operation.DropForeignKey != nil:
		return g.generateDropForeignKey(operation.DropForeignKey)
	case operation.Execute != nil:
		return operation.Execute.SQL, nil
	default:
		return "", fmt.Errorf("unknown migration operation")
	}
}

// generateCreateTable generates CREATE TABLE SQL
func (g *SQLGenerator) generateCreateTable(op *CreateTableOp) (string, error) {
	var columns []string
	var constraints []string

	for _, col := range op.Columns {
		colSQL, err := g.generateColumnDefinition(&col)
		if err != nil {
			return "", fmt.Errorf("failed to generate column definition for %s: %w", col.Name, err)
		}
		columns = append(columns, colSQL)

		// Handle primary key constraint
		if col.PrimaryKey {
			constraints = append(constraints, fmt.Sprintf("PRIMARY KEY (%s)", col.Name))
		}

		// Handle unique constraint
		if col.Unique && !col.PrimaryKey {
			constraints = append(constraints, fmt.Sprintf("UNIQUE (%s)", col.Name))
		}
	}

	// Combine columns and constraints
	var parts []string
	parts = append(parts, columns...)
	parts = append(parts, constraints...)

	sql := fmt.Sprintf("CREATE TABLE %s (%s)", op.Name, strings.Join(parts, ", "))
	return sql, nil
}

// generateDropTable generates DROP TABLE SQL
func (g *SQLGenerator) generateDropTable(op *DropTableOp) (string, error) {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s", op.Name), nil
}

// generateAddColumn generates ALTER TABLE ADD COLUMN SQL
func (g *SQLGenerator) generateAddColumn(op *AddColumnOp) (string, error) {
	colDef, err := g.generateColumnDefinitionFromAddColumn(op)
	if err != nil {
		return "", fmt.Errorf("failed to generate column definition: %w", err)
	}

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", op.Table, colDef)
	return sql, nil
}

// generateDropColumn generates ALTER TABLE DROP COLUMN SQL
func (g *SQLGenerator) generateDropColumn(op *DropColumnOp) (string, error) {
	sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", op.Table, op.Name)
	return sql, nil
}

// generateChangeColumn generates ALTER TABLE ALTER COLUMN SQL
func (g *SQLGenerator) generateChangeColumn(op *ChangeColumnOp) (string, error) {
	// PostgreSQL syntax for changing column type
	var alterations []string

	if op.Type != "" {
		dataType := g.mapDataType(op.Type, op.Length)
		alterations = append(alterations, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", op.Table, op.Name, dataType))
	}

	if op.Nullable != nil {
		if *op.Nullable {
			alterations = append(alterations, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", op.Table, op.Name))
		} else {
			alterations = append(alterations, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", op.Table, op.Name))
		}
	}

	if op.Default != nil {
		alterations = append(alterations, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %v", op.Table, op.Name, op.Default))
	}

	if len(alterations) == 0 {
		return "", fmt.Errorf("change_column operation must specify at least one change")
	}

	return strings.Join(alterations, ";\n"), nil
}

// generateAddIndex generates CREATE INDEX SQL
func (g *SQLGenerator) generateAddIndex(op *AddIndexOp) (string, error) {
	indexName := op.Name
	if indexName == "" {
		// Generate index name if not provided
		indexName = fmt.Sprintf("idx_%s_%s", op.Table, strings.Join(op.Columns, "_"))
	}

	indexType := ""
	if op.Unique {
		indexType = "UNIQUE "
	}

	columns := strings.Join(op.Columns, ", ")
	sql := fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", indexType, indexName, op.Table, columns)
	return sql, nil
}

// generateDropIndex generates DROP INDEX SQL
func (g *SQLGenerator) generateDropIndex(op *DropIndexOp) (string, error) {
	sql := fmt.Sprintf("DROP INDEX IF EXISTS %s", op.Name)
	return sql, nil
}

// generateAddForeignKey generates ALTER TABLE ADD CONSTRAINT SQL
func (g *SQLGenerator) generateAddForeignKey(op *AddForeignKeyOp) (string, error) {
	constraintName := op.Name
	if constraintName == "" {
		// Generate constraint name if not provided
		constraintName = fmt.Sprintf("fk_%s_%s_%s", op.Table, op.Column, op.ReferencedTable)
	}

	sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
		op.Table, constraintName, op.Column, op.ReferencedTable, op.ReferencedColumn)

	if op.OnDelete != "" {
		sql += fmt.Sprintf(" ON DELETE %s", op.OnDelete)
	}

	if op.OnUpdate != "" {
		sql += fmt.Sprintf(" ON UPDATE %s", op.OnUpdate)
	}

	return sql, nil
}

// generateDropForeignKey generates ALTER TABLE DROP CONSTRAINT SQL
func (g *SQLGenerator) generateDropForeignKey(op *DropForeignKeyOp) (string, error) {
	sql := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s", op.Name, op.Name)
	return sql, nil
}

// generateColumnDefinition generates a column definition from MigrationColumn
func (g *SQLGenerator) generateColumnDefinition(col *MigrationColumn) (string, error) {
	dataType := g.mapDataType(col.Type, col.Length)
	def := fmt.Sprintf("%s %s", col.Name, dataType)

	if !col.Nullable {
		def += " NOT NULL"
	}

	if col.Default != nil {
		if str, ok := col.Default.(string); ok && strings.ToUpper(str) == "NOW()" {
			def += " DEFAULT NOW()"
		} else {
			def += fmt.Sprintf(" DEFAULT %v", col.Default)
		}
	}

	return def, nil
}

// generateColumnDefinitionFromAddColumn generates a column definition from AddColumnOp
func (g *SQLGenerator) generateColumnDefinitionFromAddColumn(op *AddColumnOp) (string, error) {
	dataType := g.mapDataType(op.Type, op.Length)
	def := fmt.Sprintf("%s %s", op.Name, dataType)

	if !op.Nullable {
		def += " NOT NULL"
	}

	if op.Default != nil {
		if str, ok := op.Default.(string); ok && strings.ToUpper(str) == "NOW()" {
			def += " DEFAULT NOW()"
		} else {
			def += fmt.Sprintf(" DEFAULT %v", op.Default)
		}
	}

	if op.Unique {
		def += " UNIQUE"
	}

	return def, nil
}

// mapDataType maps migration data types to database-specific types
func (g *SQLGenerator) mapDataType(dataType string, length *int) string {
	switch g.driver {
	case interfaces.DriverPostgreSQL:
		return g.mapPostgreSQLType(dataType, length)
	case interfaces.DriverMySQL:
		return g.mapMySQLType(dataType, length)
	case interfaces.DriverSQLite:
		return g.mapSQLiteType(dataType, length)
	default:
		return strings.ToUpper(dataType)
	}
}

// mapPostgreSQLType maps types to PostgreSQL
func (g *SQLGenerator) mapPostgreSQLType(dataType string, length *int) string {
	switch strings.ToLower(dataType) {
	case "serial":
		return "SERIAL"
	case "text", "string":
		if length != nil {
			return fmt.Sprintf("VARCHAR(%d)", *length)
		}
		return "TEXT"
	case "varchar":
		if length != nil {
			return fmt.Sprintf("VARCHAR(%d)", *length)
		}
		return "VARCHAR(255)"
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

// mapMySQLType maps types to MySQL
func (g *SQLGenerator) mapMySQLType(dataType string, length *int) string {
	switch strings.ToLower(dataType) {
	case "serial":
		return "INT AUTO_INCREMENT"
	case "text", "string":
		if length != nil {
			return fmt.Sprintf("VARCHAR(%d)", *length)
		}
		return "TEXT"
	case "varchar":
		if length != nil {
			return fmt.Sprintf("VARCHAR(%d)", *length)
		}
		return "VARCHAR(255)"
	case "integer", "int":
		return "INT"
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
		return "FLOAT"
	case "double":
		return "DOUBLE"
	case "json":
		return "JSON"
	default:
		return strings.ToUpper(dataType)
	}
}

// mapSQLiteType maps types to SQLite
func (g *SQLGenerator) mapSQLiteType(dataType string, length *int) string {
	switch strings.ToLower(dataType) {
	case "serial":
		return "INTEGER"
	case "text", "string", "varchar":
		return "TEXT"
	case "integer", "int", "bigint", "int64":
		return "INTEGER"
	case "boolean", "bool":
		return "INTEGER"
	case "timestamp", "datetime", "date", "time":
		return "TEXT"
	case "decimal", "numeric", "float", "double":
		return "REAL"
	default:
		return "TEXT"
	}
}
