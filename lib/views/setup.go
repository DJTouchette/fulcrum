package views

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymerick/raymond"
)

// TemplateRenderer handles Handlebars template rendering
type TemplateRenderer struct {
	templates map[string]*raymond.Template
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer() *TemplateRenderer {
	return &TemplateRenderer{
		templates: make(map[string]*raymond.Template),
	}
}

// LoadTemplate loads a Handlebars template from file
func (tr *TemplateRenderer) LoadTemplate(name, filePath string) error {
	log.Printf("LoadTemplate: Loading template '%s' from file '%s'", name, filePath)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("LoadTemplate: Template file does not exist: %s", filePath)
		return fmt.Errorf("template file does not exist: %s", filePath)
	}

	tmpl, err := raymond.ParseFile(filePath)
	if err != nil {
		log.Printf("LoadTemplate: Failed to parse template %s: %v", name, err)
		return fmt.Errorf("failed to parse template %s: %v", name, err)
	}

	tr.templates[name] = tmpl
	log.Printf("LoadTemplate: Successfully registered template '%s'", name)
	return nil
}

// LoadTemplatesFromDir loads all .hbs files from a directory (non-recursive)
func (tr *TemplateRenderer) LoadTemplatesFromDir(dir string) error {
	log.Printf("LoadTemplatesFromDir: Loading templates from directory: %s", dir)

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Printf("LoadTemplatesFromDir: Directory does not exist: %s", dir)
		return fmt.Errorf("directory does not exist: %s", dir)
	}

	pattern := filepath.Join(dir, "*.hbs")
	log.Printf("LoadTemplatesFromDir: Using pattern: %s", pattern)

	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("LoadTemplatesFromDir: Failed to glob templates: %v", err)
		return fmt.Errorf("failed to glob templates: %v", err)
	}

	log.Printf("LoadTemplatesFromDir: Found %d .hbs files", len(files))

	for _, file := range files {
		name := filepath.Base(file)
		name = name[:len(name)-len(filepath.Ext(name))] // Remove .hbs extension
		log.Printf("LoadTemplatesFromDir: Processing file %s -> template name '%s'", file, name)

		if err := tr.LoadTemplate(name, file); err != nil {
			log.Printf("LoadTemplatesFromDir: Failed to load template from %s: %v", file, err)
			return err
		}
	}

	log.Printf("LoadTemplatesFromDir: Successfully loaded %d templates from %s", len(files), dir)
	return nil
}

// LoadTemplatesRecursive loads all .hbs files from a directory recursively
func (tr *TemplateRenderer) LoadTemplatesRecursive(dir string) error {
	log.Printf("LoadTemplatesRecursive: Starting to load templates from directory: %s", dir)

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Printf("LoadTemplatesRecursive: Directory does not exist: %s", dir)
		return fmt.Errorf("directory does not exist: %s", dir)
	}

	templateCount := 0

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("LoadTemplatesRecursive: Error walking path %s: %v", path, err)
			return err
		}

		// Log every file/directory we encounter
		if info.IsDir() {
			log.Printf("LoadTemplatesRecursive: Found directory: %s", path)
		} else {
			log.Printf("LoadTemplatesRecursive: Found file: %s (ext: %s)", path, filepath.Ext(path))
		}

		if !info.IsDir() && filepath.Ext(path) == ".hbs" {
			// Use relative path from base dir as template name
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				log.Printf("LoadTemplatesRecursive: Error getting relative path for %s: %v", path, err)
				return err
			}

			// Remove .hbs extension and use path as name (e.g., "partials/header")
			name := relPath[:len(relPath)-len(filepath.Ext(relPath))]

			log.Printf("LoadTemplatesRecursive: Loading template:")
			log.Printf("  - File path: %s", path)
			log.Printf("  - Relative path: %s", relPath)
			log.Printf("  - Template name: %s", name)

			if err := tr.LoadTemplate(name, path); err != nil {
				log.Printf("LoadTemplatesRecursive: Error loading template %s from %s: %v", name, path, err)
				return err
			}

			templateCount++
		}
		return nil
	})
	if err != nil {
		log.Printf("LoadTemplatesRecursive: Walk failed: %v", err)
		return err
	}

	log.Printf("LoadTemplatesRecursive: Finished loading %d templates from %s", templateCount, dir)
	return nil
}

// Render renders a template with the given data
func (tr *TemplateRenderer) Render(name string, data any) (string, error) {
	log.Printf("Render: Attempting to render template '%s'", name)

	// Log all available templates for debugging
	log.Printf("Render: Available templates:")
	for templateName := range tr.templates {
		log.Printf("  - '%s'", templateName)
	}

	tmpl, exists := tr.templates[name]
	if !exists {
		log.Printf("Render: Template '%s' not found", name)
		return "", fmt.Errorf("template %s not found", name)
	}

	result, err := tmpl.Exec(data)
	if err != nil {
		log.Printf("Render: Failed to execute template '%s': %v", name, err)
		return "", fmt.Errorf("failed to execute template %s: %v", name, err)
	}

	log.Printf("Render: Successfully rendered template '%s' (output length: %d chars)", name, len(result))
	return result, nil
}

// RenderTo renders a template directly to an http.ResponseWriter
func (tr *TemplateRenderer) RenderTo(w http.ResponseWriter, name string, data any) error {
	log.Printf("RenderTo: Rendering template '%s' to HTTP response", name)

	html, err := tr.Render(name, data)
	if err != nil {
		log.Printf("RenderTo: Failed to render template '%s': %v", name, err)
		return err
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
	log.Printf("RenderTo: Successfully wrote HTML response for template '%s'", name)
	return nil
}

// RenderWithLayout renders a template within a layout
func (tr *TemplateRenderer) RenderWithLayout(layoutName, templateName string, data any) (string, error) {
	log.Printf("RenderWithLayout: Rendering template '%s' with layout '%s'", templateName, layoutName)

	// First render the content template
	content, err := tr.Render(templateName, data)
	if err != nil {
		log.Printf("RenderWithLayout: Failed to render content template '%s': %v", templateName, err)
		return "", err
	}

	log.Printf("RenderWithLayout: Content template rendered successfully (length: %d chars)", len(content))

	// Create layout data with the rendered content
	layoutData := map[string]any{
		"body": content,
	}

	// If data is a map, merge it with layout data
	if dataMap, ok := data.(map[string]any); ok {
		log.Printf("RenderWithLayout: Merging %d data fields with layout data", len(dataMap))
		for key, value := range dataMap {
			layoutData[key] = value
		}
	}

	// Render the layout with the content
	result, err := tr.Render(layoutName, layoutData)
	if err != nil {
		log.Printf("RenderWithLayout: Failed to render layout '%s': %v", layoutName, err)
		return "", err
	}

	log.Printf("RenderWithLayout: Successfully rendered template '%s' with layout '%s' (final length: %d chars)", templateName, layoutName, len(result))
	return result, nil
}

// RenderWithLayoutTo renders a template with layout directly to http.ResponseWriter
func (tr *TemplateRenderer) RenderWithLayoutTo(w http.ResponseWriter, layoutName, templateName string, data any) error {
	log.Printf("RenderWithLayoutTo: Rendering template '%s' with layout '%s' to HTTP response", templateName, layoutName)

	html, err := tr.RenderWithLayout(layoutName, templateName, data)
	if err != nil {
		log.Printf("RenderWithLayoutTo: Failed to render with layout: %v", err)
		return err
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
	log.Printf("RenderWithLayoutTo: Successfully wrote HTML response with layout")
	return nil
}

// RegisterHelper registers a custom Handlebars helper
func (tr *TemplateRenderer) RegisterHelper(name string, helper any) {
	raymond.RegisterHelper(name, helper)
}

// SetupViewsFromConfig initializes the template renderer using the new config system
func SetupViewsFromConfig(appConfig interface{ GetAllTemplateDirectories() []string }) (*TemplateRenderer, error) {
	renderer := NewTemplateRenderer()

	// Register common helpers
	registerCommonHelpers(renderer)

	// Load templates from all discovered directories
	templateDirs := appConfig.GetAllTemplateDirectories()

	if len(templateDirs) == 0 {
		log.Println("Warning: No template directories found")
		return renderer, nil
	}

	for _, dir := range templateDirs {
		log.Printf("Loading templates from directory: %s", dir)
		if err := renderer.LoadTemplatesRecursive(dir); err != nil {
			log.Printf("Warning: Failed to load templates from %s: %v", dir, err)
			// Continue loading other directories even if one fails
		}
	}

	return renderer, nil
}

// SetupViewsForDevelopment sets up views with hot-reloading capabilities
func SetupViewsForDevelopment(appConfig interface{ GetAllTemplateDirectories() []string }) (*TemplateRenderer, error) {
	renderer := NewTemplateRenderer()
	registerCommonHelpers(renderer)

	// In development, we might want to reload templates on each request
	// For now, just load them once - hot reloading can be added later
	templateDirs := appConfig.GetAllTemplateDirectories()

	for _, dir := range templateDirs {
		if err := renderer.LoadTemplatesRecursive(dir); err != nil {
			log.Printf("Warning: Failed to load templates from %s: %v", dir, err)
		}
	}

	return renderer, nil
}

// registerCommonHelpers registers commonly used Handlebars helpers
func registerCommonHelpers(renderer *TemplateRenderer) {
	// String manipulation helpers
	renderer.RegisterHelper("uppercase", func(str string) string {
		return strings.ToUpper(str)
	})

	renderer.RegisterHelper("lowercase", func(str string) string {
		return strings.ToLower(str)
	})

	renderer.RegisterHelper("capitalize", func(str string) string {
		if len(str) == 0 {
			return str
		}
		return strings.ToUpper(str[:1]) + strings.ToLower(str[1:])
	})

	// Comparison helpers
	renderer.RegisterHelper("eq", func(a, b any) bool {
		return a == b
	})

	renderer.RegisterHelper("ne", func(a, b any) bool {
		return a != b
	})

	renderer.RegisterHelper("gt", func(a, b any) bool {
		switch aVal := a.(type) {
		case int:
			if bVal, ok := b.(int); ok {
				return aVal > bVal
			}
		case float64:
			if bVal, ok := b.(float64); ok {
				return aVal > bVal
			}
		}
		return false
	})

	renderer.RegisterHelper("lt", func(a, b any) bool {
		switch aVal := a.(type) {
		case int:
			if bVal, ok := b.(int); ok {
				return aVal < bVal
			}
		case float64:
			if bVal, ok := b.(float64); ok {
				return aVal < bVal
			}
		}
		return false
	})

	// Logical helpers
	renderer.RegisterHelper("and", func(a, b bool) bool {
		return a && b
	})

	renderer.RegisterHelper("or", func(a, b bool) bool {
		return a || b
	})

	renderer.RegisterHelper("not", func(a bool) bool {
		return !a
	})

	// Conditional helpers
	renderer.RegisterHelper("if_eq", func(a, b any, options *raymond.Options) string {
		if a == b {
			return options.Fn()
		}
		return options.Inverse()
	})

	// URL/Path helpers
	renderer.RegisterHelper("url", func(path string) string {
		// Basic URL helper - can be enhanced with base URL logic
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	})

	// JSON helper for client-side data
	renderer.RegisterHelper("json", func(data any) string {
		// This would need proper JSON marshaling
		return fmt.Sprintf("%+v", data)
	})
}

// LoadTemplateForRoute loads a specific template for a route if not already loaded
func (tr *TemplateRenderer) LoadTemplateForRoute(routePath, templatePath string) error {
	// Check if template is already loaded
	if _, exists := tr.templates[routePath]; exists {
		return nil
	}

	// Load the template
	return tr.LoadTemplate(routePath, templatePath)
}

// RenderRoute renders a template specifically for a route with enhanced context
func (tr *TemplateRenderer) RenderRoute(routeName string, data map[string]any, layoutName ...string) (string, error) {
	// Add route-specific context
	if data == nil {
		data = make(map[string]any)
	}

	// Add meta information
	data["_route"] = routeName
	data["_timestamp"] = fmt.Sprintf("%d", 1234567890) // You'd use actual timestamp

	// If layout is specified, use it
	if len(layoutName) > 0 && layoutName[0] != "" {
		return tr.RenderWithLayout(layoutName[0], routeName, data)
	}

	// Otherwise render directly
	return tr.Render(routeName, data)
}

// SetupViews - keep the old function for backward compatibility
func SetupViews(templateDir string) (*TemplateRenderer, error) {
	renderer := NewTemplateRenderer()
	registerCommonHelpers(renderer)

	if err := renderer.LoadTemplatesRecursive(templateDir); err != nil {
		return nil, fmt.Errorf("failed to load templates: %v", err)
	}

	return renderer, nil
}

// Enhanced template loading with better error handling and logging
func (tr *TemplateRenderer) LoadTemplatesFromDirectories(dirs []string) error {
	var loadedCount int
	var errors []error

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			log.Printf("Template directory does not exist: %s", dir)
			continue
		}

		count, err := tr.loadTemplatesFromDirWithCount(dir)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to load from %s: %w", dir, err))
			continue
		}

		loadedCount += count
		log.Printf("Loaded %d templates from %s", count, dir)
	}

	log.Printf("Total templates loaded: %d", loadedCount)

	if len(errors) > 0 {
		log.Printf("Encountered %d errors while loading templates", len(errors))
		for _, err := range errors {
			log.Printf("  - %v", err)
		}
		// Return the first error, but we've already logged all of them
		return errors[0]
	}

	return nil
}

// loadTemplatesFromDirWithCount loads templates and returns the count
func (tr *TemplateRenderer) loadTemplatesFromDirWithCount(dir string) (int, error) {
	count := 0

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && (filepath.Ext(path) == ".hbs" || filepath.Ext(path) == ".handlebars") {
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}

			// Remove extension for template name
			name := relPath
			if filepath.Ext(name) == ".hbs" {
				name = name[:len(name)-4]
			} else if filepath.Ext(name) == ".handlebars" {
				name = name[:len(name)-11]
			}

			// Convert path separators to forward slashes for consistent naming
			name = strings.ReplaceAll(name, string(filepath.Separator), "/")

			if err := tr.LoadTemplate(name, path); err != nil {
				return err
			}

			count++
		}

		return nil
	})

	return count, err
}
