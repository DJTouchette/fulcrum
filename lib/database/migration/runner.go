package migration

import (
	"context"
	"fmt"
	"log"
	"sort"

	"fulcrum/lib/database/interfaces"
)

// Runner executes migrations against the database
type Runner struct {
	db           interfaces.Database
	parser       *Parser
	tracker      *Tracker
	sqlGenerator *SQLGenerator
}

// NewRunner creates a new migration runner
func NewRunner(db interfaces.Database, appPath string) *Runner {
	return &Runner{
		db:           db,
		parser:       NewParser(appPath),
		tracker:      NewTracker(db),
		sqlGenerator: NewSQLGenerator(db.GetDriver()),
	}
}

// Initialize sets up the migration system (creates schema_migrations table)
func (r *Runner) Initialize(ctx context.Context) error {
	return r.tracker.InitializeSchema(ctx)
}

// MigrateUp runs all pending migrations
func (r *Runner) MigrateUp(ctx context.Context) error {
	log.Println("🔄 Running pending migrations...")

	// Load all migrations
	allMigrations, err := r.parser.LoadAllMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Get pending migrations
	pendingMigrations, err := r.tracker.GetPendingMigrations(ctx, allMigrations)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pendingMigrations) == 0 {
		log.Println("✅ No pending migrations to run")
		return nil
	}

	// Sort pending migrations by domain and version
	sort.Slice(pendingMigrations, func(i, j int) bool {
		if pendingMigrations[i].Domain == pendingMigrations[j].Domain {
			return pendingMigrations[i].Version < pendingMigrations[j].Version
		}
		return pendingMigrations[i].Domain < pendingMigrations[j].Domain
	})

	log.Printf("📋 Found %d pending migrations", len(pendingMigrations))

	// Execute each migration
	for _, migration := range pendingMigrations {
		if err := r.executeMigrationUp(ctx, migration); err != nil {
			return fmt.Errorf("failed to execute migration %s:%d (%s): %w", 
				migration.Domain, migration.Version, migration.Name, err)
		}
	}

	log.Printf("✅ Successfully applied %d migrations", len(pendingMigrations))
	return nil
}

// MigrateDown rolls back the last migration for each domain
func (r *Runner) MigrateDown(ctx context.Context) error {
	log.Println("🔄 Rolling back migrations...")

	// Get all applied migrations
	appliedMigrations, err := r.tracker.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	if len(appliedMigrations) == 0 {
		log.Println("✅ No migrations to roll back")
		return nil
	}

	// Group by domain and get the latest version for each
	domainLatest := make(map[string]MigrationRecord)
	for _, migration := range appliedMigrations {
		if latest, exists := domainLatest[migration.Domain]; !exists || migration.Version > latest.Version {
			domainLatest[migration.Domain] = migration
		}
	}

	// Load all migrations to find the ones to roll back
	allMigrations, err := r.parser.LoadAllMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Create a map for quick migration lookup
	migrationMap := make(map[string]Migration)
	for _, migration := range allMigrations {
		key := fmt.Sprintf("%s:%d", migration.Domain, migration.Version)
		migrationMap[key] = migration
	}

	// Roll back the latest migration for each domain
	rollbackCount := 0
	for _, latestRecord := range domainLatest {
		key := fmt.Sprintf("%s:%d", latestRecord.Domain, latestRecord.Version)
		if migration, exists := migrationMap[key]; exists {
			if err := r.executeMigrationDown(ctx, migration); err != nil {
				return fmt.Errorf("failed to roll back migration %s:%d (%s): %w", 
					migration.Domain, migration.Version, migration.Name, err)
			}
			rollbackCount++
		}
	}

	log.Printf("✅ Successfully rolled back %d migrations", rollbackCount)
	return nil
}

// MigrateDownTo rolls back to a specific version for a domain
func (r *Runner) MigrateDownTo(ctx context.Context, domain string, targetVersion int) error {
	log.Printf("🔄 Rolling back %s migrations to version %d...", domain, targetVersion)

	// Get applied migrations for the domain
	appliedMigrations, err := r.tracker.GetAppliedMigrationsForDomain(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations for domain %s: %w", domain, err)
	}

	// Filter migrations that need to be rolled back (version > targetVersion)
	var toRollback []MigrationRecord
	for _, migration := range appliedMigrations {
		if migration.Version > targetVersion {
			toRollback = append(toRollback, migration)
		}
	}

	if len(toRollback) == 0 {
		log.Printf("✅ Domain %s is already at or below version %d", domain, targetVersion)
		return nil
	}

	// Sort by version descending (rollback from highest to lowest)
	sort.Slice(toRollback, func(i, j int) bool {
		return toRollback[i].Version > toRollback[j].Version
	})

	// Load all migrations to get the migration definitions
	allMigrations, err := r.parser.LoadDomainMigrations(domain)
	if err != nil {
		return fmt.Errorf("failed to load migrations for domain %s: %w", domain, err)
	}

	// Create a map for quick lookup
	migrationMap := make(map[int]Migration)
	for _, migration := range allMigrations {
		migrationMap[migration.Version] = migration
	}

	// Roll back each migration
	rollbackCount := 0
	for _, record := range toRollback {
		if migration, exists := migrationMap[record.Version]; exists {
			if err := r.executeMigrationDown(ctx, migration); err != nil {
				return fmt.Errorf("failed to roll back migration %s:%d (%s): %w", 
					migration.Domain, migration.Version, migration.Name, err)
			}
			rollbackCount++
		}
	}

	log.Printf("✅ Successfully rolled back %d migrations for domain %s", rollbackCount, domain)
	return nil
}

// GetStatus returns the status of all migrations
func (r *Runner) GetStatus(ctx context.Context) ([]MigrationStatus, error) {
	allMigrations, err := r.parser.LoadAllMigrations()
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}

	return r.tracker.GetMigrationStatus(ctx, allMigrations)
}

// executeMigrationUp executes the up operations of a migration
func (r *Runner) executeMigrationUp(ctx context.Context, migration Migration) error {
	log.Printf("⬆️  Applying migration %s:%d - %s", migration.Domain, migration.Version, migration.Name)

	// Begin transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Execute each up operation
	for i, operation := range migration.Up {
		sql, err := r.sqlGenerator.GenerateSQL(&operation)
		if err != nil {
			return fmt.Errorf("failed to generate SQL for operation %d: %w", i, err)
		}

		log.Printf("   🔨 %s", sql)

		_, err = tx.Exec(ctx, sql)
		if err != nil {
			return fmt.Errorf("failed to execute operation %d (%s): %w", i, sql, err)
		}
	}

	// Record the migration in schema_migrations table
	insertSQL := `
		INSERT INTO schema_migrations (version, domain, name, applied_at)
		VALUES ($1, $2, $3, NOW())`
	
	_, err = tx.Exec(ctx, insertSQL, migration.Version, migration.Domain, migration.Name)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("   ✅ Migration %s:%d applied successfully", migration.Domain, migration.Version)
	return nil
}

// executeMigrationDown executes the down operations of a migration
func (r *Runner) executeMigrationDown(ctx context.Context, migration Migration) error {
	log.Printf("⬇️  Rolling back migration %s:%d - %s", migration.Domain, migration.Version, migration.Name)

	if len(migration.Down) == 0 {
		return fmt.Errorf("migration %s:%d has no down operations defined", migration.Domain, migration.Version)
	}

	// Begin transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Execute each down operation
	for i, operation := range migration.Down {
		sql, err := r.sqlGenerator.GenerateSQL(&operation)
		if err != nil {
			return fmt.Errorf("failed to generate SQL for down operation %d: %w", i, err)
		}

		log.Printf("   🔨 %s", sql)

		_, err = tx.Exec(ctx, sql)
		if err != nil {
			return fmt.Errorf("failed to execute down operation %d (%s): %w", i, sql, err)
		}
	}

	// Remove the migration record from schema_migrations table
	deleteSQL := `
		DELETE FROM schema_migrations 
		WHERE domain = $1 AND version = $2`
	
	_, err = tx.Exec(ctx, deleteSQL, migration.Domain, migration.Version)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("   ✅ Migration %s:%d rolled back successfully", migration.Domain, migration.Version)
	return nil
}
