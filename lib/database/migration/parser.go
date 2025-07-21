package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

// Parser handles loading and parsing migration files
type Parser struct {
	appPath string
}

// NewParser creates a new migration parser
func NewParser(appPath string) *Parser {
	return &Parser{
		appPath: appPath,
	}
}

// LoadAllMigrations loads all migration files from all domains
func (p *Parser) LoadAllMigrations() ([]Migration, error) {
	var allMigrations []Migration
	
	// Find all domain directories
	domains, err := p.findDomainDirectories()
	if err != nil {
		return nil, fmt.Errorf("failed to find domain directories: %w", err)
	}

	// Load migrations from each domain
	for _, domainPath := range domains {
		domainName := filepath.Base(domainPath)
		migrations, err := p.LoadDomainMigrations(domainName)
		if err != nil {
			return nil, fmt.Errorf("failed to load migrations for domain %s: %w", domainName, err)
		}
		allMigrations = append(allMigrations, migrations...)
	}

	// Sort migrations by version
	sort.Slice(allMigrations, func(i, j int) bool {
		if allMigrations[i].Domain == allMigrations[j].Domain {
			return allMigrations[i].Version < allMigrations[j].Version
		}
		return allMigrations[i].Domain < allMigrations[j].Domain
	})

	return allMigrations, nil
}

// LoadDomainMigrations loads migrations for a specific domain
func (p *Parser) LoadDomainMigrations(domainName string) ([]Migration, error) {
	migrationsDir := filepath.Join(p.appPath, "domains", domainName, "migrations")
	
	// Check if migrations directory exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		return []Migration{}, nil // No migrations directory is ok
	}

	// Find all migration files
	migrationFiles, err := p.findMigrationFiles(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find migration files: %w", err)
	}

	var migrations []Migration
	for _, filePath := range migrationFiles {
		migration, err := p.parseMigrationFile(filePath, domainName)
		if err != nil {
			return nil, fmt.Errorf("failed to parse migration file %s: %w", filePath, err)
		}
		migrations = append(migrations, migration)
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// findDomainDirectories finds all domain directories
func (p *Parser) findDomainDirectories() ([]string, error) {
	domainsDir := filepath.Join(p.appPath, "domains")
	if _, err := os.Stat(domainsDir); os.IsNotExist(err) {
		return []string{}, nil // No domains directory
	}

	var domainDirs []string
	err := filepath.WalkDir(domainsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only look at immediate subdirectories of domains/
		if d.IsDir() && path != domainsDir {
			relPath, _ := filepath.Rel(domainsDir, path)
			// Only include direct subdirectories (no nested paths)
			if !strings.Contains(relPath, string(filepath.Separator)) {
				domainDirs = append(domainDirs, path)
			}
		}
		return nil
	})

	return domainDirs, err
}

// findMigrationFiles finds all .yml files in the migrations directory
func (p *Parser) findMigrationFiles(migrationsDir string) ([]string, error) {
	var migrationFiles []string
	
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() && (strings.HasSuffix(file.Name(), ".yml") || strings.HasSuffix(file.Name(), ".yaml")) {
			migrationFiles = append(migrationFiles, filepath.Join(migrationsDir, file.Name()))
		}
	}

	sort.Strings(migrationFiles)
	return migrationFiles, nil
}

// parseMigrationFile parses a single migration YAML file
func (p *Parser) parseMigrationFile(filePath, domainName string) (Migration, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return Migration{}, fmt.Errorf("failed to read migration file: %w", err)
	}

	var migration Migration
	err = yaml.Unmarshal(content, &migration)
	if err != nil {
		return Migration{}, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Set metadata
	migration.Domain = domainName
	migration.FilePath = filePath

	// Validate migration
	if err := p.validateMigration(&migration); err != nil {
		return Migration{}, fmt.Errorf("invalid migration: %w", err)
	}

	return migration, nil
}

// validateMigration validates a parsed migration
func (p *Parser) validateMigration(migration *Migration) error {
	if migration.Version <= 0 {
		return fmt.Errorf("version must be greater than 0")
	}

	if migration.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(migration.Up) == 0 {
		return fmt.Errorf("up operations are required")
	}

	// Validate each operation
	for i, op := range migration.Up {
		if err := p.validateOperation(&op); err != nil {
			return fmt.Errorf("invalid up operation %d: %w", i, err)
		}
	}

	for i, op := range migration.Down {
		if err := p.validateOperation(&op); err != nil {
			return fmt.Errorf("invalid down operation %d: %w", i, err)
		}
	}

	return nil
}

// ParseYAMLFile parses a migration YAML file by path
func ParseYAMLFile(filePath string) (*Migration, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migration file: %w", err)
	}
	
	return ParseYAMLContent(content)
}

// ParseYAMLContent parses migration YAML content from bytes
func ParseYAMLContent(content []byte) (*Migration, error) {
	var migration Migration
	err := yaml.Unmarshal(content, &migration)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	
	// Create a temporary parser for validation
	p := &Parser{}
	if err := p.validateMigration(&migration); err != nil {
		return nil, fmt.Errorf("invalid migration: %w", err)
	}
	
	return &migration, nil
}

// validateOperation validates a single migration operation
func (p *Parser) validateOperation(op *MigrationOperation) error {
	operationCount := 0
	
	if op.CreateTable != nil {
		operationCount++
		if op.CreateTable.Name == "" {
			return fmt.Errorf("create_table: table name is required")
		}
		if len(op.CreateTable.Columns) == 0 {
			return fmt.Errorf("create_table: at least one column is required")
		}
	}
	
	if op.DropTable != nil {
		operationCount++
		if op.DropTable.Name == "" {
			return fmt.Errorf("drop_table: table name is required")
		}
	}
	
	if op.AddColumn != nil {
		operationCount++
		if op.AddColumn.Table == "" || op.AddColumn.Name == "" || op.AddColumn.Type == "" {
			return fmt.Errorf("add_column: table, name, and type are required")
		}
	}
	
	if op.DropColumn != nil {
		operationCount++
		if op.DropColumn.Table == "" || op.DropColumn.Name == "" {
			return fmt.Errorf("drop_column: table and name are required")
		}
	}
	
	if op.ChangeColumn != nil {
		operationCount++
		if op.ChangeColumn.Table == "" || op.ChangeColumn.Name == "" {
			return fmt.Errorf("change_column: table and name are required")
		}
	}
	
	if op.AddIndex != nil {
		operationCount++
		if op.AddIndex.Table == "" || len(op.AddIndex.Columns) == 0 {
			return fmt.Errorf("add_index: table and columns are required")
		}
	}
	
	if op.DropIndex != nil {
		operationCount++
		if op.DropIndex.Name == "" {
			return fmt.Errorf("drop_index: name is required")
		}
	}
	
	if op.AddForeignKey != nil {
		operationCount++
		fk := op.AddForeignKey
		if fk.Table == "" || fk.Column == "" || fk.ReferencedTable == "" || fk.ReferencedColumn == "" {
			return fmt.Errorf("add_foreign_key: table, column, referenced_table, and referenced_column are required")
		}
	}
	
	if op.DropForeignKey != nil {
		operationCount++
		if op.DropForeignKey.Name == "" {
			return fmt.Errorf("drop_foreign_key: name is required")
		}
	}
	
	if op.Execute != nil {
		operationCount++
		if op.Execute.SQL == "" {
			return fmt.Errorf("execute: sql is required")
		}
	}

	if operationCount == 0 {
		return fmt.Errorf("operation must have exactly one operation type specified")
	}
	
	if operationCount > 1 {
		return fmt.Errorf("operation can only have one operation type specified")
	}

	return nil
}
