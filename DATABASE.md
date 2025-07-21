# Fulcrum Database Integration

This document explains how to set up and use the database integration in Fulcrum.

## Quick Start with PostgreSQL

### 1. Start PostgreSQL with Docker

```bash
# Start PostgreSQL container
docker-compose up postgres -d

# Or with pgAdmin for database management
docker-compose --profile admin up -d
```

### 2. Configure Your Application

Update your `fulcrum.yml`:

```yaml
db:
  driver: postgres
  host: localhost
  port: 5432
  database: fulcrum_dev
  username: fulcrum
  password: fulcrum_pass
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 10
  conn_max_lifetime_minutes: 5
```

### 3. Test the Connection

```bash
# Install dependencies
go mod tidy

# Run the connection test
go run test_db.go
```

## Database Drivers

Fulcrum supports multiple database vendors through a unified interface:

### PostgreSQL ✅ (Implemented)
- Driver: `postgres` or `postgresql`
- Uses `github.com/lib/pq` driver
- Full feature support

### MySQL 🚧 (Coming Soon)
- Driver: `mysql`
- Will use `github.com/go-sql-driver/mysql`

### SQLite 🚧 (Coming Soon)
- Driver: `sqlite`
- Will use `github.com/mattn/go-sqlite3`

## Configuration Options

### Common Options
- `driver`: Database type (`postgres`, `mysql`, `sqlite`)
- `host`: Database host (not used for SQLite)
- `port`: Database port (not used for SQLite)
- `database`: Database name
- `username`: Database username
- `password`: Database password

### Connection Pool Options
- `max_open_conns`: Maximum number of open connections (default: 25)
- `max_idle_conns`: Maximum number of idle connections (default: 10)
- `conn_max_lifetime_minutes`: Connection lifetime in minutes (default: 5)

### PostgreSQL Options
- `ssl_mode`: SSL mode (`disable`, `require`, `verify-ca`, `verify-full`)

### SQLite Options
- `file_path`: Path to SQLite database file

## Architecture

The database integration is designed with a vendor-agnostic approach:

```
┌─────────────────┐
│   Application   │
└─────────────────┘
         │
┌─────────────────┐
│ Database Manager│  ← Unified interface
└─────────────────┘
         │
┌─────────────────┐
│   Driver Layer  │  ← Vendor-specific implementations
│  - PostgreSQL   │
│  - MySQL        │
│  - SQLite       │
└─────────────────┘
```

### Key Components

- **`database.Database`**: Common interface all drivers implement
- **`database.Manager`**: Manages driver instances and connections
- **`drivers/`**: Vendor-specific implementations
- **Configuration Bridge**: Converts between parser and database configs

## Development Database

The included `docker-compose.yml` provides:

### PostgreSQL
- **Host**: localhost:5432
- **Database**: fulcrum_dev
- **Username**: fulcrum
- **Password**: fulcrum_pass
- **Test Database**: fulcrum_test (for testing)

### pgAdmin (Optional)
- **URL**: http://localhost:8081
- **Email**: admin@fulcrum.dev
- **Password**: admin

Start with: `docker-compose --profile admin up -d`

## Usage Examples

### Basic Connection
```go
import "fulcrum/lib/database"

// Load config from parser
dbConfig, err := database.FromParserConfig(appConfig.DB)
dbManager, err := database.NewManager(dbConfig)
err = dbManager.Connect(context.Background())
```

### Queries
```go
db := dbManager.GetDatabase()

// Query rows
rows, err := db.Query(ctx, "SELECT * FROM users WHERE active = $1", true)

// Single row
var name string
err := db.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", userID).Scan(&name)

// Execute
result, err := db.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", name, email)
```

### Schema Operations
```go
// Create table
schema := database.TableSchema{
    Columns: []database.ColumnSchema{
        {Name: "id", Type: "integer", Nullable: false},
        {Name: "name", Type: "text", Nullable: false},
    },
    PrimaryKey: []string{"id"},
}
err := db.CreateTable(ctx, "users", schema)

// Check existence
exists, err := db.TableExists(ctx, "users")
```

## Next Steps

1. **Schema Generation**: Automatic table creation from YAML models
2. **Migration System**: Database migration management
3. **Query Builder**: Higher-level query construction
4. **Model Integration**: Connect to Fulcrum's model validation system
