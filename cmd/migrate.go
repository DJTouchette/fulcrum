package cmd

import (
	"context"
	"fmt"
	"fulcrum/lib/database"
	"fulcrum/lib/database/interfaces"
	"fulcrum/lib/database/migration"
	"fulcrum/lib/parser"
	"log"
	"os"

	"github.com/spf13/cobra"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration management",
	Long: `Manage database migrations for your Fulcrum application.

Available subcommands:
  up      - Apply pending migrations
  down    - Roll back migrations  
  status  - Show migration status
  reset   - Reset database (drop and recreate)`,
}

// migrateUpCmd applies pending migrations
var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply pending migrations",
	Long: `Apply all pending migrations to the database.

This will run all migration files that haven't been applied yet,
in the correct order (by domain and version).`,
	Run: runMigrateUp,
}

// migrateDownCmd rolls back migrations
var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back migrations",
	Long: `Roll back the most recent migration for each domain.

Use --to flag to roll back to a specific version for a domain:
  fulcrum migrate down --domain=users --to=2`,
	Run: runMigrateDown,
}

// migrateStatusCmd shows migration status
var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long: `Show the status of all migrations, including which have been
applied and which are still pending.`,
	Run: runMigrateStatus,
}

// migrateResetCmd resets the database
var migrateResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset database (DANGEROUS)",
	Long: `Drop all tables and re-run all migrations from scratch.

WARNING: This will delete all data in your database!
Only use this in development environments.`,
	Run: runMigrateReset,
}

var (
	migrateDomain     string
	migrateToVersion  int
	migrateForceReset bool
)

func init() {
	rootCmd.AddCommand(migrateCmd)

	// Add subcommands
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateResetCmd)

	// Flags for migrate down
	migrateDownCmd.Flags().StringVar(&migrateDomain, "domain", "", "Domain to roll back (required with --to)")
	migrateDownCmd.Flags().IntVar(&migrateToVersion, "to", 0, "Version to roll back to (requires --domain)")

	// Flags for reset
	migrateResetCmd.Flags().BoolVar(&migrateForceReset, "force", false, "Skip confirmation prompt")
}

func runMigrateUp(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Load configuration and setup database
	dbManager, appPath, err := setupDatabase(ctx)
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer dbManager.Close()

	// Create migration runner
	runner := migration.NewRunner(dbManager.GetDatabase(), appPath)

	// Initialize migration system
	if err := runner.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize migration system: %v", err)
	}

	// Run migrations
	if err := runner.MigrateUp(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
}

func runMigrateDown(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Load configuration and setup database
	dbManager, appPath, err := setupDatabase(ctx)
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer dbManager.Close()

	// Create migration runner
	runner := migration.NewRunner(dbManager.GetDatabase(), appPath)

	// Initialize migration system
	if err := runner.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize migration system: %v", err)
	}

	// Handle specific domain and version rollback
	if migrateDomain != "" && migrateToVersion >= 0 {
		if err := runner.MigrateDownTo(ctx, migrateDomain, migrateToVersion); err != nil {
			log.Fatalf("Failed to roll back %s to version %d: %v", migrateDomain, migrateToVersion, err)
		}
		return
	}

	// Handle --to flag without domain
	if migrateToVersion >= 0 && migrateDomain == "" {
		log.Fatalf("--to flag requires --domain flag")
	}

	// Default: roll back latest migration for each domain
	if err := runner.MigrateDown(ctx); err != nil {
		log.Fatalf("Failed to roll back migrations: %v", err)
	}
}

func runMigrateStatus(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Load configuration and setup database
	dbManager, appPath, err := setupDatabase(ctx)
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer dbManager.Close()

	// Create migration runner
	runner := migration.NewRunner(dbManager.GetDatabase(), appPath)

	// Initialize migration system
	if err := runner.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize migration system: %v", err)
	}

	// Get status
	statuses, err := runner.GetStatus(ctx)
	if err != nil {
		log.Fatalf("Failed to get migration status: %v", err)
	}

	// Display status
	fmt.Println("📋 Migration Status")
	fmt.Println("==================")

	if len(statuses) == 0 {
		fmt.Println("No domains with migrations found")
		return
	}

	for _, status := range statuses {
		fmt.Printf("\n🏗️  Domain: %s\n", status.Domain)

		if len(status.AppliedMigrations) > 0 {
			fmt.Printf("✅ Applied Migrations (%d):\n", len(status.AppliedMigrations))
			for _, applied := range status.AppliedMigrations {
				fmt.Printf("   %d - %s (applied %s)\n",
					applied.Version, applied.Name, applied.AppliedAt.Format("2006-01-02 15:04:05"))
			}
		}

		if len(status.PendingMigrations) > 0 {
			fmt.Printf("⏳ Pending Migrations (%d):\n", len(status.PendingMigrations))
			for _, pending := range status.PendingMigrations {
				fmt.Printf("   %d - %s\n", pending.Version, pending.Name)
			}
		}

		if len(status.AppliedMigrations) == 0 && len(status.PendingMigrations) == 0 {
			fmt.Println("   No migrations found")
		}
	}
}

func runMigrateReset(cmd *cobra.Command, args []string) {
	// Safety check
	if !migrateForceReset {
		fmt.Println("⚠️  WARNING: This will delete ALL data in your database!")
		fmt.Print("Are you sure you want to continue? (type 'yes' to confirm): ")

		var confirmation string
		fmt.Scanln(&confirmation)

		if confirmation != "yes" {
			fmt.Println("Reset cancelled.")
			return
		}
	}

	ctx := context.Background()

	// Load configuration and setup database
	dbManager, appPath, err := setupDatabase(ctx)
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer dbManager.Close()

	db := dbManager.GetDatabase()

	fmt.Println("🗑️  Dropping all tables...")

	// Get all table names
	tables, err := getAllTables(ctx, db)
	if err != nil {
		log.Fatalf("Failed to get table names: %v", err)
	}

	// Drop all tables
	for _, table := range tables {
		fmt.Printf("   Dropping table: %s\n", table)
		if err := db.DropTable(ctx, table); err != nil {
			log.Printf("Warning: Failed to drop table %s: %v", table, err)
		}
	}

	fmt.Println("🔄 Re-running all migrations...")

	// Create migration runner and initialize
	runner := migration.NewRunner(db, appPath)
	if err := runner.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize migration system: %v", err)
	}

	// Run all migrations
	if err := runner.MigrateUp(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	fmt.Println("✅ Database reset complete!")
}

// setupDatabase loads configuration and creates database manager
func setupDatabase(ctx context.Context) (*database.Manager, string, error) {
	// Get current working directory as app path
	appPath, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Load app configuration
	appConfig, err := parser.GetAppConfig(appPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load app config: %w", err)
	}

	// Convert to database config
	dbConfig, err := database.FromParserConfig(appConfig.DB)
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert database config: %w", err)
	}

	// Create database manager
	dbManager, err := database.NewManager(dbConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create database manager: %w", err)
	}

	// Connect to database
	if err := dbManager.Connect(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to connect to database: %w", err)
	}

	return dbManager, appPath, nil
}

// getAllTables returns all table names in the database (PostgreSQL specific)
func getAllTables(ctx context.Context, db interfaces.Database) ([]string, error) {
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public'`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}
