package database

import (
	"context"
	"fmt"
	"fulcrum/lib/database/drivers"
	"fulcrum/lib/database/interfaces"
	"log"
)

// Manager handles database connections and operations
type Manager struct {
	config   interfaces.Config
	database interfaces.Database
}

// NewManager creates a new database manager
func NewManager(config interfaces.Config) (*Manager, error) {
	manager := &Manager{
		config: config,
	}

	// Create the appropriate database driver
	db, err := manager.createDriver()
	if err != nil {
		return nil, fmt.Errorf("failed to create database driver: %w", err)
	}

	manager.database = db
	return manager, nil
}

// createDriver creates the appropriate database driver based on configuration
func (m *Manager) createDriver() (interfaces.Database, error) {
	switch m.config.Driver {
	case interfaces.DriverPostgreSQL:
		return drivers.NewPostgreSQLDB(m.config)
	case interfaces.DriverMySQL:
		return drivers.NewMySQLDB(m.config)
	case interfaces.DriverSQLite:
		return drivers.NewSQLiteDB(m.config)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", m.config.Driver)
	}
}

// Connect establishes a connection to the database
func (m *Manager) Connect(ctx context.Context) error {
	log.Printf("Connecting to %s database...", m.config.Driver)

	err := m.database.Connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test the connection
	err = m.database.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("Successfully connected to %s database", m.config.Driver)
	return nil
}

// Close closes the database connection
func (m *Manager) Close() error {
	log.Printf("Closing database connection...")
	return m.database.Close()
}

// GetDatabase returns the database instance
func (m *Manager) GetDatabase() interfaces.Database {
	return m.database
}

// GetConfig returns the database configuration
func (m *Manager) GetConfig() interfaces.Config {
	return m.config
}

// HealthCheck performs a health check on the database
func (m *Manager) HealthCheck(ctx context.Context) error {
	return m.database.Ping(ctx)
}

// GetStats returns database connection statistics
func (m *Manager) GetStats() interface{} {
	return m.database.Stats()
}
