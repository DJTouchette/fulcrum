# Config Parser ✅

## Status: COMPLETE

## Description
YAML-based configuration system that parses both application-level and domain-level configs to define models, routes, and framework settings.

## Implementation Details
- **Location**: `lib/parser/`
- **Format**: YAML configurations
- **Structure**: Hierarchical config loading (app + domains)

## Features Implemented
- [x] YAML parsing with `gopkg.in/yaml.v2`
- [x] Domain discovery via file system traversal
- [x] Model definitions with field types and validations
- [x] HTTP route configuration
- [x] Database configuration parsing
- [x] View path resolution
- [x] Validation rule parsing (length, nullable, etc.)

## Files
- `lib/parser/config.go` - Main config loading logic
- `lib/parser/models.go` - Data structures and helper methods
- `lib/parser/syntax.go` - Constants and syntax definitions

## Configuration Structure
```yaml
# Root fulcrum.yml
db:
  type: postgres
  host: localhost
  port: 5432

# Domain fulcrum.yml
models:
  - user:
      email:
        type: text
        validations:
          - nullable: false
          - length: {min: 5, max: 80}
logic:
  http:
    restful: true
    routes:
      - method: GET
        link: user_index_request
        view: index
```

## API Usage
```go
appConfig, err := parser.GetAppConfig("/path/to/app")
// Returns fully parsed AppConfig with all domains
```

## Notes
- Solid foundation with good error handling
- Extensible validation system
- Minor typo in syntax.go (ValidateLengthMax = "maz" should be "max")
