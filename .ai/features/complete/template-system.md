# Template System (Handlebars) ✅

## Status: COMPLETE

## Description
Full-featured template rendering system using Handlebars templates with layout support, helper registration, and recursive template loading.

## Implementation Details
- **Location**: `lib/views/setup.go`
- **Template Engine**: Handlebars (via `github.com/aymerick/raymond`)
- **Features**: Layouts, helpers, recursive loading, HTTP integration

## Features Implemented
- [x] Handlebars template parsing and rendering
- [x] Template loading from directories (recursive)
- [x] Layout system with content injection
- [x] Custom helper registration
- [x] HTTP response integration
- [x] Template caching in memory
- [x] Comprehensive error handling and logging
- [x] File existence checking
- [x] Relative path-based template naming

## Core API

### TemplateRenderer Class
```go
type TemplateRenderer struct {
    templates map[string]*raymond.Template
}
```

### Key Methods
- `LoadTemplate(name, filePath)` - Load single template
- `LoadTemplatesFromDir(dir)` - Load from directory (non-recursive)
- `LoadTemplatesRecursive(dir)` - Load from directory tree
- `Render(name, data)` - Render template with data
- `RenderWithLayout(layout, template, data)` - Render with layout
- `RenderTo(w, name, data)` - Render directly to HTTP response

## Template Structure
```
templates/
├── layouts/
│   └── main.hbs          # Layout template
├── users/
│   ├── index.hbs         # User listing page
│   └── show.hbs          # User detail page
└── partials/
    ├── header.hbs        # Reusable header
    └── footer.hbs        # Reusable footer
```

## Layout System
Layout templates receive rendered content in `{{body}}`:
```handlebars
<!DOCTYPE html>
<html>
<head><title>My App</title></head>
<body>
    {{{body}}}  <!-- Rendered page content injected here -->
</body>
</html>
```

## Helper System
Built-in helpers provided:
- `uppercase` - Transform text to uppercase
- `eq` - Equality comparison for conditionals

Custom helpers can be registered:
```go
renderer.RegisterHelper("formatDate", func(date time.Time) string {
    return date.Format("2006-01-02")
})
```

## Integration with HTTP
Templates are automatically rendered for routes with `view` specified:
```yaml
routes:
  - method: GET
    link: user_index_request
    view: users/index  # Renders users/index.hbs with layout
```

## Template Naming Convention
Templates are named by their relative path from the template root:
- `users/index.hbs` → template name: `users/index`
- `layouts/main.hbs` → template name: `layouts/main`

## Setup Function
```go
renderer, err := views.SetupViews("/path/to/templates")
// Loads all templates and registers common helpers
```

## Files
- `lib/views/setup.go` - Complete template system implementation

## Error Handling
- File existence validation
- Template parsing error reporting
- Missing template detection
- Comprehensive logging for debugging

## Notes
- Production-ready with comprehensive error handling
- Efficient memory-based template caching
- Flexible helper system for extending functionality
- Well-integrated with HTTP response system
- Detailed logging for troubleshooting template issues
