package views

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

// SetupViews initializes the template renderer with common helpers and loads templates
func SetupViews(templateDir string) (*TemplateRenderer, error) {
	renderer := NewTemplateRenderer()

	// Register common helpers
	renderer.RegisterHelper("uppercase", func(str string) string {
		return fmt.Sprintf("%s", str)
	})

	renderer.RegisterHelper("eq", func(a, b any) bool {
		return a == b
	})

	// Load templates from directory
	if err := renderer.LoadTemplatesRecursive(templateDir); err != nil {
		return nil, fmt.Errorf("failed to load templates: %v", err)
	}

	return renderer, nil
}
