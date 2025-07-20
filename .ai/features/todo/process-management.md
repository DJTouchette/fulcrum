# Process Management 🚧

## Status: TODO - HIGH PRIORITY

## Description
Automatic spawning, monitoring, and lifecycle management of TypeScript domain processes. Currently domains must be started manually.

## Current State
- TypeScript domain framework is complete
- Domains can connect manually to gRPC server
- No automatic process spawning or management
- No health monitoring or restart logic

## Required Implementation
- [ ] Domain process discovery and spawning
- [ ] Process lifecycle management (start/stop/restart)
- [ ] Health monitoring and heartbeat checking
- [ ] Automatic restart on crashes
- [ ] Domain registration and discovery
- [ ] Environment variable management for domains
- [ ] Log aggregation from domain processes

## Technical Requirements

### Process Spawner
```go
type ProcessManager struct {
    processes map[string]*DomainProcess
    config    *parser.AppConfig
}

type DomainProcess struct {
    Name    string
    Path    string
    Cmd     *exec.Cmd
    Status  ProcessStatus
    LastHealthCheck time.Time
}

func (pm *ProcessManager) StartDomain(domainName string) error
func (pm *ProcessManager) StopDomain(domainName string) error
func (pm *ProcessManager) RestartDomain(domainName string) error
func (pm *ProcessManager) HealthCheck(domainName string) error
```

### Domain Discovery
Scan application directory for domains and their entry points:
```
domains/
├── users/
│   ├── fulcrum.yml        # Domain config
│   ├── domain.js          # Entry point (Node.js)
│   └── package.json       # Optional dependencies
└── orders/
    ├── fulcrum.yml
    └── domain.ts          # Entry point (TypeScript)
```

### Process Startup Sequence
1. Parse application config to find domains
2. For each domain directory:
   - Detect entry point (domain.js/ts)
   - Check for package.json dependencies
   - Set environment variables (GRPC_SERVER, DOMAIN_NAME)
   - Spawn Node.js/TypeScript process
   - Wait for gRPC connection registration
3. Monitor all processes for health

### Health Monitoring
- Periodic heartbeat messages via gRPC
- Process status monitoring (CPU, memory)
- Automatic restart on failure
- Graceful shutdown handling

### Environment Variables for Domains
```bash
FULCRUM_GRPC_SERVER=localhost:50051
FULCRUM_DOMAIN_NAME=users
FULCRUM_APP_PATH=/path/to/app
FULCRUM_LOG_LEVEL=info
```

## Integration Points
- Integrate with CLI commands (`fulcrum dev`, `fulcrum serve`)
- Connect to config parsing for domain discovery
- Integrate with gRPC server for health checks
- Add logging and monitoring capabilities

## Implementation Strategy
1. **Phase 1**: Basic process spawning
   - Simple exec.Command for Node.js processes
   - Basic start/stop functionality
   - Integration with dev command
2. **Phase 2**: Health monitoring
   - Heartbeat system via gRPC
   - Process status tracking
   - Automatic restart logic
3. **Phase 3**: Advanced features
   - Log aggregation
   - Performance monitoring
   - Graceful shutdown coordination

## CLI Integration
```bash
fulcrum dev --watch          # Start with file watching
fulcrum serve                # Production mode
fulcrum domains list         # Show domain status
fulcrum domains restart users  # Restart specific domain
```

## Dependencies
- Go's `os/exec` package for process management
- File system watcher for development mode
- Process monitoring libraries

## Success Criteria
- [ ] Domains automatically start when running `fulcrum dev`
- [ ] Process crashes are automatically detected and restarted
- [ ] Health monitoring shows domain status
- [ ] Graceful shutdown of all processes
- [ ] Proper error handling and logging
- [ ] Integration with hot reloading system

## Estimated Effort
**Medium** (1 week) - Process management is well-understood but needs careful implementation.

## Related Features
- Links to Hot Reloading (file watching triggers process restart)
- Links to CLI Commands (process control commands)
- Links to Logging System (process log aggregation)

## Notes
This is critical for developer experience - currently domains must be started manually in separate terminals, which is not sustainable.
