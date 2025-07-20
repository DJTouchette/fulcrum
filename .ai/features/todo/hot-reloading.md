# Hot Reloading 🚧

## Status: TODO - MEDIUM PRIORITY

## Description
File watcher system that automatically reloads configuration changes, restarts domain processes, and reloads templates during development.

## Current State
- No file watching implemented
- Manual restarts required for all changes
- Template system loads templates at startup only
- Dev command mentioned but not implemented

## Required Implementation
- [ ] File system watcher for config files (fulcrum.yml)
- [ ] TypeScript/JavaScript file watching for domain code
- [ ] Template file watching and reloading
- [ ] Selective process restart (only affected domains)
- [ ] Browser refresh notification (optional)
- [ ] Configuration validation on reload

## Technical Requirements

### File Watcher System
```go
type HotReloader struct {
    watcher     *fsnotify.Watcher
    appConfig   *parser.AppConfig
    processManager *ProcessManager
    templateRenderer *views.TemplateRenderer
}

func (hr *HotReloader) Start() error
func (hr *HotReloader) Stop() error
func (hr *HotReloader) handleFileChange(event fsnotify.Event)
```

### Watch Patterns
- `**/fulcrum.yml` - Configuration files (app and domain level)
- `domains/**/*.js` - Domain JavaScript files  
- `domains/**/*.ts` - Domain TypeScript files
- `shared/templates/**/*.hbs` - Template files
- `domains/*/templates/**/*.hbs` - Domain-specific templates

### Reload Strategies

#### Config File Changes
1. Parse and validate new configuration
2. Compare with current config to identify changes
3. Restart affected domains only
4. Reload HTTP routes if changed
5. Update database schema if models changed

#### Domain Code Changes  
1. Identify which domain changed
2. Gracefully stop domain process
3. Restart domain process with new code
4. Wait for gRPC reconnection

#### Template Changes
1. Reload affected templates into memory
2. Clear template cache for changed files
3. No process restart needed

### Development Server Integration
```bash
fulcrum dev --watch          # Enable hot reloading
fulcrum dev --watch=false    # Disable hot reloading
fulcrum dev --poll           # Use polling instead of events (for some filesystems)
```

## Implementation Strategy
1. **Phase 1**: Basic file watching
   - Implement fsnotify integration
   - Watch config and domain files
   - Simple restart-all approach
2. **Phase 2**: Selective reloading
   - Identify changed domains
   - Restart only affected processes  
   - Template-only reloading
3. **Phase 3**: Advanced features
   - Browser refresh notifications
   - Real-time config validation
   - Performance optimizations

## Integration Points
- Process Management system for domain restarts
- Config Parser for validation and comparison
- Template System for template reloading
- CLI command flags for enabling/disabling

## File Change Detection
```go
func (hr *HotReloader) handleFileChange(event fsnotify.Event) {
    switch {
    case strings.HasSuffix(event.Name, "fulcrum.yml"):
        hr.reloadConfig(event.Name)
    case strings.HasSuffix(event.Name, ".js") || strings.HasSuffix(event.Name, ".ts"):
        hr.restartDomain(hr.getDomainFromPath(event.Name))
    case strings.HasSuffix(event.Name, ".hbs"):
        hr.reloadTemplate(event.Name)
    }
}
```

## Dependencies
- `github.com/fsnotify/fsnotify` - File system notifications
- Integration with Process Management
- Integration with Template System

## Success Criteria
- [ ] Config changes automatically reload without full restart
- [ ] Domain code changes restart only affected domain
- [ ] Template changes reload without any restart
- [ ] Debouncing prevents excessive reloads during rapid edits
- [ ] Error handling for invalid configs/code
- [ ] Performance impact is minimal during development

## Estimated Effort
**Small-Medium** (3-5 days) - File watching is well-understood, integration is the main complexity.

## Related Features
- Requires Process Management (for domain restarts)
- Integrates with Template System (for template reloading)
- Links to CLI Commands (watch flags)

## Notes
This is crucial for developer experience but depends on Process Management being implemented first. Should be prioritized after core functionality is working.
