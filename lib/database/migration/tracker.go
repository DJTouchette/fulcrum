package migration

import (
	"context"
	"fmt"
	"time"

	"fulcrum/lib/database/interfaces"
)
// Tracker manages migration records in the database
type Tracker struct {
	db interfaces.Database
}

// NewTracker creates a new migration tracker
func NewTracker(db interfaces.Database) *Tracker {
	return &Tracker{
		db: db,
	}
}

// InitializeSchema creates the schema_migrations table if it doesn't exist
func (t *Tracker) InitializeSchema(ctx context.Context) error {
	// Check if schema_migrations table exists
	exists, err := t.db.TableExists(ctx, "schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to check if schema_migrations table exists: %w", err)
	}

	if exists {
		return nil // Table already exists
	}

	// Create schema_migrations table
	schema := interfaces.TableSchema{
		Columns: []interfaces.ColumnDefinition{
			{
				Name:    "version",
				Type:    "integer",
				NotNull: true,
			},
			{
				Name:    "domain",
				Type:    "varchar(255)",
				NotNull: true,
			},
			{
				Name:    "name",
				Type:    "varchar(255)",
				NotNull: true,
			},
			{
				Name:         "applied_at",
				Type:         "timestamp",
				NotNull:      true,
				DefaultValue: func() *string { s := "NOW()"; return &s }(),
			},
		},
		PrimaryKey: []string{"version", "domain"},
	}

	err = t.db.CreateTable(ctx, "schema_migrations", schema)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	return nil
}

// GetAppliedMigrations returns all applied migrations
func (t *Tracker) GetAppliedMigrations(ctx context.Context) ([]MigrationRecord, error) {
	query := `
		SELECT version, domain, name, applied_at 
		FROM schema_migrations 
		ORDER BY domain, version`
	
	rows, err := t.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var migrations []MigrationRecord
	for rows.Next() {
		var record MigrationRecord
		err := rows.Scan(&record.Version, &record.Domain, &record.Name, &record.AppliedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}
		migrations = append(migrations, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migration records: %w", err)
	}

	return migrations, nil
}

// GetAppliedMigrationsForDomain returns applied migrations for a specific domain
func (t *Tracker) GetAppliedMigrationsForDomain(ctx context.Context, domain string) ([]MigrationRecord, error) {
	query := `
		SELECT version, domain, name, applied_at 
		FROM schema_migrations 
		WHERE domain = $1 
		ORDER BY version`
	
	rows, err := t.db.Query(ctx, query, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations for domain %s: %w", domain, err)
	}
	defer rows.Close()

	var migrations []MigrationRecord
	for rows.Next() {
		var record MigrationRecord
		err := rows.Scan(&record.Version, &record.Domain, &record.Name, &record.AppliedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}
		migrations = append(migrations, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migration records: %w", err)
	}

	return migrations, nil
}

// IsMigrationApplied checks if a specific migration has been applied
func (t *Tracker) IsMigrationApplied(ctx context.Context, domain string, version int) (bool, error) {
	query := `
		SELECT COUNT(*) 
		FROM schema_migrations 
		WHERE domain = $1 AND version = $2`
	
	var count int
	err := t.db.QueryRow(ctx, query, domain, version).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if migration is applied: %w", err)
	}

	return count > 0, nil
}

// RecordMigration records that a migration has been applied
func (t *Tracker) RecordMigration(ctx context.Context, migration Migration) error {
	query := `
		INSERT INTO schema_migrations (version, domain, name, applied_at)
		VALUES ($1, $2, $3, $4)`
	
	_, err := t.db.Exec(ctx, query, migration.Version, migration.Domain, migration.Name, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// RemoveMigrationRecord removes a migration record (used for rollbacks)
func (t *Tracker) RemoveMigrationRecord(ctx context.Context, domain string, version int) error {
	query := `
		DELETE FROM schema_migrations 
		WHERE domain = $1 AND version = $2`
	
	result, err := t.db.Exec(ctx, query, domain, version)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("migration %s:%d was not found in migration records", domain, version)
	}

	return nil
}

// GetLatestVersion returns the latest migration version for a domain
func (t *Tracker) GetLatestVersion(ctx context.Context, domain string) (int, error) {
	query := `
		SELECT COALESCE(MAX(version), 0) 
		FROM schema_migrations 
		WHERE domain = $1`
	
	var version int
	err := t.db.QueryRow(ctx, query, domain).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest version for domain %s: %w", domain, err)
	}

	return version, nil
}

// GetPendingMigrations returns migrations that haven't been applied yet
func (t *Tracker) GetPendingMigrations(ctx context.Context, allMigrations []Migration) ([]Migration, error) {
	appliedMigrations, err := t.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Create a map of applied migrations for quick lookup
	appliedMap := make(map[string]bool)
	for _, applied := range appliedMigrations {
		key := fmt.Sprintf("%s:%d", applied.Domain, applied.Version)
		appliedMap[key] = true
	}

	// Find pending migrations
	var pending []Migration
	for _, migration := range allMigrations {
		key := fmt.Sprintf("%s:%d", migration.Domain, migration.Version)
		if !appliedMap[key] {
			pending = append(pending, migration)
		}
	}

	return pending, nil
}

// GetMigrationStatus returns the status of migrations for all domains
func (t *Tracker) GetMigrationStatus(ctx context.Context, allMigrations []Migration) ([]MigrationStatus, error) {
	applied, err := t.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	pending, err := t.GetPendingMigrations(ctx, allMigrations)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending migrations: %w", err)
	}

	// Group by domain
	domainMap := make(map[string]*MigrationStatus)
	
	// Add applied migrations
	for _, migration := range applied {
		if _, exists := domainMap[migration.Domain]; !exists {
			domainMap[migration.Domain] = &MigrationStatus{
				Domain:            migration.Domain,
				AppliedMigrations: []MigrationRecord{},
				PendingMigrations: []Migration{},
			}
		}
		domainMap[migration.Domain].AppliedMigrations = append(domainMap[migration.Domain].AppliedMigrations, migration)
	}
	
	// Add pending migrations
	for _, migration := range pending {
		if _, exists := domainMap[migration.Domain]; !exists {
			domainMap[migration.Domain] = &MigrationStatus{
				Domain:            migration.Domain,
				AppliedMigrations: []MigrationRecord{},
				PendingMigrations: []Migration{},
			}
		}
		domainMap[migration.Domain].PendingMigrations = append(domainMap[migration.Domain].PendingMigrations, migration)
	}

	// Convert to slice
	var result []MigrationStatus
	for _, status := range domainMap {
		result = append(result, *status)
	}

	return result, nil
}
