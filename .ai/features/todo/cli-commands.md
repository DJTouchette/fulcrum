# CLI Commands Expansion 🚧

## Status: TODO - MEDIUM PRIORITY

## Description
Expand the CLI command system beyond the basic `dev` command to include production serving, building, code generation, and management commands.

## Current State
- Basic Cobra CLI structure exists
- `fulcrum dev` command implemented (loads config, starts servers)
- Root command has placeholder descriptions
- No production or build commands

## Required Implementation
- [ ] `fulcrum serve` - Production server
- [ ] `fulcrum build` - Build for deployment
- [ ] `fulcrum generate` - Code scaffolding
- [ ] `fulcrum migrate` - Database migrations
- [ ] `fulcrum domains` - Domain management
- [ ] `fulcrum config` - Configuration validation
- [ ] Improved help and documentation

## Command Specifications

### Production Server (`fulcrum serve`)
```bash
fulcrum serve --config app.yml --port 8080 --env production
fulcrum serve --help
```
- Start HTTP and gRPC servers for production
- No file watching or development features
- Performance optimizations enabled
- Environment-specific configuration loading

### Build Command (`fulcrum build`)  
```bash
fulcrum build --output ./dist --target linux-amd64
fulcrum build --docker --tag myapp:latest
```
- Compile Go binary with embedded assets
- Bundle TypeScript domains (compile to JS)
- Include templates and static files
- Optional Docker image creation
- Cross-platform compilation

### Code Generation (`fulcrum generate`)
```bash
fulcrum generate domain users          # Create new domain
fulcrum generate model user            # Create new model
fulcrum generate migration add_users   # Create migration
fulcrum generate config               # Create sample config
```
- Scaffold new domains with templates
- Generate model definitions and migrations
- Create sample configurations
- Generate documentation

### Database Migrations (`fulcrum migrate`)
```bash
fulcrum migrate up                    # Run pending migrations
fulcrum migrate down                  # Rollback last migration
fulcrum migrate status               # Show migration status
fulcrum migrate create add_users     # Create new migration
```
- Database schema migration system
- Support for up/down migrations
- Migration history tracking
- Integration with model definitions

### Domain Management (`fulcrum domains`)
```bash
fulcrum domains list                 # List all domains
fulcrum domains status              # Show domain process status  
fulcrum domains restart users       # Restart specific domain
fulcrum domains logs users --follow # Show domain logs
```
- Domain process monitoring
- Individual domain control
- Log viewing and aggregation
- Health check reporting

### Configuration Management (`fulcrum config`)
```bash
fulcrum config validate             # Validate all configs
fulcrum config show                 # Show parsed configuration
fulcrum config init                 # Create sample config files
```
- Configuration validation
- Configuration debugging
- Sample config generation

## Implementation Details

### Command Structure
```go
// cmd/serve.go
var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the Fulcrum server in production mode", 
    Run:   runServeCommand,
}

// cmd/build.go  
var buildCmd = &cobra.Command{
    Use:   "build",
    Short: "Build Fulcrum application for deployment",
    Run:   runBuildCommand,
}
```

### Flag Management
Global flags:
- `--config` - Configuration file path
- `--env` - Environment (development, production)
- `--verbose` - Verbose logging
- `--help` - Command help

Command-specific flags:
- `serve`: `--port`, `--host`, `--workers`
- `build`: `--output`, `--target`, `--docker`
- `generate`: `--template`, `--force`

### Integration Points
- Config Parser for configuration loading
- Process Management for domain control
- Database Integration for migrations
- Template System for code generation

## Implementation Strategy
1. **Phase 1**: Essential commands
   - `fulcrum serve` for production
   - `fulcrum build` basic functionality
   - Improve existing `dev` command
2. **Phase 2**: Management commands  
   - `fulcrum domains` for process control
   - `fulcrum config` for validation
   - `fulcrum migrate` for database
3. **Phase 3**: Advanced features
   - Code generation system
   - Docker integration
   - Cross-platform builds

## Help System Improvement
```bash
fulcrum --help
# Fulcrum - Config-driven full-stack framework
# 
# Usage:
#   fulcrum [command]
#
# Available Commands:
#   serve     Start production server
#   dev       Start development server with hot reloading
#   build     Build application for deployment
#   generate  Generate code scaffolds
#   migrate   Database migration management
#   domains   Domain process management
#   config    Configuration management
```

## Dependencies
- Process Management (for domain commands)
- Database Integration (for migrate commands)
- Build system integration

## Success Criteria
- [ ] Production server command works without development features
- [ ] Build command creates deployable artifacts
- [ ] Generate commands create valid scaffolds
- [ ] Migration commands manage database schema
- [ ] Domain commands control individual processes
- [ ] Help system is comprehensive and clear

## Estimated Effort
**Medium** (1 week) - Many commands but most are straightforward wrappers around existing functionality.

## Related Features
- Links to Process Management (domains commands)
- Links to Database Integration (migrate commands)
- Links to Single Binary Deployment (build command)

## Notes
This expands the framework from a development tool to a complete application lifecycle management system. Priority should be on `serve` and `build` commands for production readiness.
