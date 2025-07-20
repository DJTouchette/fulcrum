# Database Integration 🚧

## Status: TODO - HIGH PRIORITY

## Description
Real database integration to replace the current mock implementations. Should support PostgreSQL (as configured) with schema generation from YAML model definitions.

## Current State
- Database configuration parsing is implemented (`db.type: postgres`)
- Model definitions are parsed from YAML
- Database operations are **mocked** in `grpc.go` (returns fake data)
- Validation rules are parsed but not enforced

## Required Implementation
- [ ] PostgreSQL connection management
- [ ] Schema generation from YAML models
- [ ] ORM/Query builder integration
- [ ] Database migrations system
- [ ] Connection pooling
- [ ] Transaction support
- [ ] Model validation enforcement

## Technical Requirements

### Connection Management
```go
type DatabaseManager struct {
    conn *sql.DB
    config DBConfig
}

func (dm *DatabaseManager) Connect() error
func (dm *DatabaseManager) Close() error
```

### Schema Generation
Generate PostgreSQL schema from YAML models:
```yaml
models:
  - user:
      email:
        type: text
        validations:
          - nullable: false
          - length: {min: 5, max: 80}
```

Should create:
```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(80) NOT NULL CHECK (LENGTH(email) >= 5),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

### ORM Integration Options
1. **GORM** - Popular Go ORM with good PostgreSQL support
2. **SQLBoiler** - Code generation based ORM
3. **Ent** - Facebook's entity framework for Go
4. **Custom SQL Builder** - Lightweight option

### Message Types to Implement
Replace mocked handlers in `grpc.go`:
- `db_create` - Insert records
- `db_update` - Update records  
- `db_find` - Query records
- `db_delete` - Delete records

## Integration Points
- Replace mock implementations in `lib/lang/adapters/grpc.go:processMessage()`
- Connect to config parsing in `lib/parser/models.go`
- Integrate with validation system
- Support TypeScript fluent API calls

## Migration Strategy
1. Choose ORM/database library
2. Implement connection management
3. Create schema generation from parsed models
4. Replace mocked database handlers
5. Add migration command to CLI
6. Test with example app

## Dependencies
- PostgreSQL driver (pq or pgx)
- Selected ORM library
- Database migration library

## Success Criteria
- [ ] Real PostgreSQL connections working
- [ ] Schema auto-generated from YAML models
- [ ] CRUD operations working via gRPC
- [ ] TypeScript domain methods return real data
- [ ] Model validations enforced at database level
- [ ] Migration system for schema changes

## Estimated Effort
**Medium-Large** (1-2 weeks) - Core database functionality is substantial but well-scoped.

## Notes
This is the highest priority missing piece that will make the framework actually functional for real applications.
