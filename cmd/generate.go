package cmd

import (
	"fmt"
	"fulcrum/lib/database/migration"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate code scaffolds and migrations",
	Long: `Generate various code scaffolds including migrations, domains, and models.

Available subcommands:
  migration  - Generate a new migration file`,
}

// generateMigrationCmd generates new migration files
var generateMigrationCmd = &cobra.Command{
	Use:   "migration [name]",
	Short: "Generate a new migration file",
	Long: `Generate a new YAML migration file in the specified domain.

Usage:
  fulcrum generate migration create_users --domain=users
  fulcrum generate migration add_email_index --domain=users

The migration name should describe what the migration does.`,
	Args: cobra.ExactArgs(1),
	Run:  runGenerateMigration,
}

var generateDomain string

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(generateMigrationCmd)

	// Flags for generate migration
	generateMigrationCmd.Flags().StringVar(&generateDomain, "domain", "", "Domain to create the migration in (required)")
	generateMigrationCmd.MarkFlagRequired("domain")
}

func runGenerateMigration(cmd *cobra.Command, args []string) {
	migrationName := args[0]

	// Get current working directory as app path
	appPath, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Validate domain exists
	domainPath := filepath.Join(appPath, "domains", generateDomain)
	if _, err := os.Stat(domainPath); os.IsNotExist(err) {
		log.Fatalf("Domain '%s' does not exist. Create the domain directory first.", generateDomain)
	}

	// Create migrations directory if it doesn't exist
	migrationsDir := filepath.Join(domainPath, "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		log.Fatalf("Failed to create migrations directory: %v", err)
	}

	// Get next version number
	parser := migration.NewParser(appPath)
	existingMigrations, err := parser.LoadDomainMigrations(generateDomain)
	if err != nil {
		log.Fatalf("Failed to load existing migrations: %v", err)
	}

	nextVersion := 1
	for _, existing := range existingMigrations {
		if existing.Version >= nextVersion {
			nextVersion = existing.Version + 1
		}
	}

	// Generate filename
	versionStr := fmt.Sprintf("%03d", nextVersion)
	fileName := fmt.Sprintf("%s_%s.yml", versionStr, migrationName)
	filePath := filepath.Join(migrationsDir, fileName)

	// Check if file already exists
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		log.Fatalf("Migration file already exists: %s", filePath)
	}

	// Generate migration template
	template := generateMigrationTemplate(nextVersion, migrationName)

	// Write the file
	if err := os.WriteFile(filePath, []byte(template), 0644); err != nil {
		log.Fatalf("Failed to write migration file: %v", err)
	}

	fmt.Printf("✅ Created migration: %s\n", filePath)
	fmt.Printf("📝 Edit the file to add your migration operations\n")
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Edit %s\n", filePath)
	fmt.Printf("  2. Add your up and down operations\n")
	fmt.Printf("  3. Run: fulcrum migrate up\n")
}

func generateMigrationTemplate(version int, name string) string {
	// Generate a more helpful template based on the migration name
	template := fmt.Sprintf(`version: %d
name: %s
description: "TODO: Add description of what this migration does"

up:
  # Add your up operations here
  # Example operations:
  # - create_table:
  #     name: example_table
  #     columns:
  #       - name: id
  #         type: serial
  #         primary_key: true
  #       - name: name
  #         type: varchar
  #         length: 255
  #         nullable: false
  # 
  # - add_column:
  #     table: existing_table
  #     name: new_column
  #     type: text
  #     nullable: true
  #
  # - add_index:
  #     table: table_name
  #     columns: [column1, column2]
  #     unique: false
  - execute:
      sql: "-- TODO: Add your SQL here"

down:
  # Add your down operations here (to reverse the up operations)
  # Remember: down operations should undo what up operations do
  - execute:
      sql: "-- TODO: Add your rollback SQL here"
`, version, name)

	// Add specific templates based on migration name patterns
	lowerName := strings.ToLower(name)

	if strings.HasPrefix(lowerName, "create_") {
		tableName := strings.TrimPrefix(lowerName, "create_")
		template = fmt.Sprintf(`version: %d
name: %s
description: "Create %s table"

up:
  - create_table:
      name: %s
      columns:
        - name: id
          type: serial
          primary_key: true
        - name: created_at
          type: timestamp
          nullable: false
          default: "NOW()"
        - name: updated_at
          type: timestamp
          nullable: false
          default: "NOW()"
        # TODO: Add your columns here

down:
  - drop_table:
      name: %s
`, version, name, tableName, tableName, tableName)
	} else if strings.HasPrefix(lowerName, "add_") && strings.Contains(lowerName, "_to_") {
		// Pattern: add_column_to_table
		parts := strings.Split(lowerName, "_to_")
		if len(parts) == 2 {
			columnName := strings.TrimPrefix(parts[0], "add_")
			tableName := parts[1]
			template = fmt.Sprintf(`version: %d
name: %s
description: "Add %s column to %s table"

up:
  - add_column:
      table: %s
      name: %s
      type: text  # TODO: Change to appropriate type
      nullable: true  # TODO: Set appropriate nullability

down:
  - drop_column:
      table: %s
      name: %s
`, version, name, columnName, tableName, tableName, columnName, tableName, columnName)
		}
	} else if strings.HasPrefix(lowerName, "add_index") {
		template = fmt.Sprintf(`version: %d
name: %s
description: "Add database index"

up:
  - add_index:
      table: table_name  # TODO: Set table name
      columns: [column_name]  # TODO: Set column names
      unique: false  # TODO: Set to true if unique index
      name: idx_table_column  # TODO: Set index name

down:
  - drop_index:
      name: idx_table_column  # TODO: Match index name from up
`, version, name)
	}

	return template
}
