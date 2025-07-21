package drivers

import (
	"fmt"
	"fulcrum/lib/database/interfaces"
)

// MySQLDB implements the Database interface for MySQL
type MySQLDB struct {
	config interfaces.Config
}

// NewMySQLDB creates a new MySQL database connection
func NewMySQLDB(config interfaces.Config) (interfaces.Database, error) {
	return nil, fmt.Errorf("MySQL driver not implemented yet")
}
