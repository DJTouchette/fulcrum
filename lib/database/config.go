package database

import (
	"fmt"
	"fulcrum/lib/database/interfaces"
	"fulcrum/lib/parser"
	"time"
)

// FromParserConfig converts a parser.DBConfig to database.Config
func FromParserConfig(parserConfig parser.DBConfig) (interfaces.Config, error) {
	// Map driver string to DatabaseDriver type
	var driver interfaces.DatabaseDriver
	switch parserConfig.Driver {
	case "postgres", "postgresql":
		driver = interfaces.DriverPostgreSQL
	case "mysql":
		driver = interfaces.DriverMySQL
	case "sqlite":
		driver = interfaces.DriverSQLite
	default:
		return interfaces.Config{}, fmt.Errorf("unsupported database driver: %s", parserConfig.Driver)
	}

	// Convert lifetime from minutes to duration
	var connMaxLifetime time.Duration
	if parserConfig.ConnMaxLifetime > 0 {
		connMaxLifetime = time.Duration(parserConfig.ConnMaxLifetime) * time.Minute
	}

	config := interfaces.Config{
		Driver:          driver,
		Host:            parserConfig.Host,
		Port:            parserConfig.Port,
		Database:        parserConfig.Database,
		Username:        parserConfig.Username,
		Password:        parserConfig.Password,
		SSLMode:         parserConfig.SSLMode,
		MaxOpenConns:    parserConfig.MaxOpenConns,
		MaxIdleConns:    parserConfig.MaxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
		FilePath:        parserConfig.FilePath,
	}

	return config, nil
}

// ToParserConfig converts a database.Config back to parser.DBConfig
func ToParserConfig(dbConfig interfaces.Config) parser.DBConfig {
	var driver string
	switch dbConfig.Driver {
	case interfaces.DriverPostgreSQL:
		driver = "postgres"
	case interfaces.DriverMySQL:
		driver = "mysql"
	case interfaces.DriverSQLite:
		driver = "sqlite"
	}

	// Convert lifetime from duration to minutes
	lifetimeMinutes := int(dbConfig.ConnMaxLifetime.Minutes())

	return parser.DBConfig{
		Driver:          driver,
		Host:            dbConfig.Host,
		Port:            dbConfig.Port,
		Database:        dbConfig.Database,
		Username:        dbConfig.Username,
		Password:        dbConfig.Password,
		SSLMode:         dbConfig.SSLMode,
		MaxOpenConns:    dbConfig.MaxOpenConns,
		MaxIdleConns:    dbConfig.MaxIdleConns,
		ConnMaxLifetime: lifetimeMinutes,
		FilePath:        dbConfig.FilePath,
	}
}
