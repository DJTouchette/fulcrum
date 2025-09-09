package parser

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	views "fulcrum/lib/views"

	"gopkg.in/yaml.v2"
)

// AppConfig represents the complete application configuration
type AppConfig struct {
	Domains []DomainConfig `yaml:"domains"`
	DB      DBConfig       `yaml:"db"`
	Path    string         `yaml:"path"`
	Root    string         `yaml:"root"`
	Views   *views.TemplateRenderer
}

// DBConfig holds database configuration
type DBConfig struct {
	Driver          string `yaml:"driver"` // postgres, mysql, sqlite
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Database        string `yaml:"database"`
	Username        string `yaml:"username"`
	Password        string `yaml:"password"`
	SSLMode         string `yaml:"ssl_mode"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime_minutes"`
	// SQLite specific
	FilePath string `yaml:"file_path"`
}

// DomainConfig represents a single domain configuration
type DomainConfig struct {
	Models   []ModelDefinition `yaml:"models"`
	Logic    LogicConfig       `yaml:"logic"`
	Name     string            `yaml:"name"`
	Path     string            `yaml:"path"`
	ViewPath string            `yaml:"viewpath"`
}

// ModelDefinition defines data models for a domain
type ModelDefinition map[string]Model

// Model defines the structure of a data model
type Model map[string]Field

// Field defines a single field in a model
type Field struct {
	Type        string       `yaml:"type"`
	Validations []Validation `yaml:"validations"`
}

// Validation defines validation rules for fields
type Validation map[string]any

// LogicConfig defines the business logic configuration
type LogicConfig struct {
	HTTP HTTPConfig `yaml:"http"`
}

// HTTPConfig defines HTTP routing configuration
type HTTPConfig struct {
	Restful bool    `yaml:"restful"`
	Routes  []Route `yaml:"routes"`
}

// RedirectRule represents a redirect configuration
type RedirectRule struct {
	To     string `yaml:"to"`     // Target URL pattern
	Status int    `yaml:"status"` // HTTP status code (default: 303)
	When   string `yaml:"when"`   // Condition: "success", "error", "always"
}

// Route defines a single HTTP route
type Route struct {
	Method       string       `yaml:"method"`        // HTTP method: GET, POST, etc.
	Link         string       `yaml:"link"`          // URL pattern: /users/:id
	View         string       `yaml:"view"`          // Template filename: get.html.hbs
	Path         string       `yaml:"path"`          // Unique route identifier
	ViewPath     string       `yaml:"viewpath"`      // Full path to template file
	Format       string       `yaml:"format"`        // Response format: html, json, sql
	Redirect     RedirectRule `yaml:"redirect"`      // Redirect configuration
	TemplateName string       `yaml:"template_name"` // Preloaded template name
}

// GetAppConfig parses the application configuration from the file system
func GetAppConfig(root string) (AppConfig, error) {
	// Load main config
	mainConfigPath := filepath.Join(root, DomainConfigFileName)
	mainConfigFile, err := os.ReadFile(mainConfigPath)
	if err != nil {
		return AppConfig{}, fmt.Errorf("failed to read main config file: %w", err)
	}

	var appConfig AppConfig
	if err := yaml.Unmarshal(mainConfigFile, &appConfig); err != nil {
		return AppConfig{}, fmt.Errorf("failed to parse main config file: %w", err)
	}

	// Discover and parse domains
	domains, err := discoverDomains(root)
	if err != nil {
		return AppConfig{}, fmt.Errorf("failed to discover domains: %w", err)
	}

	appConfig.Domains = domains
	appConfig.Path = root

	// Discover redirect rules
	if err := appConfig.DiscoverRedirects(); err != nil {
		fmt.Printf("Warning: failed to discover redirects: %v\n", err)
	}

	// Note: Template preloading will happen later after the renderer is initialized

	return appConfig, nil
}

// PreloadRouteTemplates loads all route templates at startup
func (ac *AppConfig) PreloadRouteTemplates() error {
	if ac.Views == nil {
		return fmt.Errorf("template renderer not initialized")
	}

	log.Printf("🔄 Pre-loading route templates...")

	for domainIndex, domain := range ac.Domains {
		for routeIndex, route := range domain.Logic.HTTP.Routes {
			// Create a predictable template name based on the route path
			// Use a hash of the file path to ensure uniqueness and consistency
			pathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(route.ViewPath)))
			templateName := fmt.Sprintf("route_%s", pathHash[:16]) // Use first 16 chars of hash

			// Load the template with the predictable name
			if err := ac.Views.LoadTemplate(templateName, route.ViewPath); err != nil {
				log.Printf("⚠️ Failed to preload template %s (%s): %v", templateName, route.ViewPath, err)
				// Don't fail completely, just log the warning
				continue
			}

			// Store the template name back on the route for easy lookup
			ac.Domains[domainIndex].Logic.HTTP.Routes[routeIndex].TemplateName = templateName

			log.Printf("✅ Preloaded template: %s -> %s", templateName, route.ViewPath)
		}
	}

	log.Printf("🏁 Route template preloading completed")
	return nil
}

// DiscoverRedirects scans for redirect.yaml files and applies them to routes
func (ac *AppConfig) DiscoverRedirects() error {
	log.Printf("🔍 Starting redirect discovery...")

	for domainIndex, domain := range ac.Domains {
		log.Printf("🔍 Checking domain: %s", domain.Name)

		for routeIndex, route := range domain.Logic.HTTP.Routes {
			log.Printf("🔍 Checking route: %s %s", route.Method, route.Link)
			log.Printf("🔍 Route ViewPath: %s", route.ViewPath)

			// Check for redirect.yaml file in the same directory as the template
			templateDir := filepath.Dir(route.ViewPath)
			redirectPath := filepath.Join(templateDir, "redirect.yaml")

			log.Printf("🔍 Looking for redirect file at: %s", redirectPath)

			if stat, err := os.Stat(redirectPath); err == nil {
				log.Printf("✅ Found redirect file! Size: %d bytes", stat.Size())

				// Load redirect configuration
				redirectData, err := os.ReadFile(redirectPath)
				if err != nil {
					log.Printf("❌ Could not read redirect file %s: %v", redirectPath, err)
					continue
				}

				log.Printf("📄 Redirect file contents: %s", string(redirectData))

				var redirectRule RedirectRule
				if err := yaml.Unmarshal(redirectData, &redirectRule); err != nil {
					log.Printf("❌ Could not parse redirect file %s: %v", redirectPath, err)
					continue
				}

				log.Printf("✅ Parsed redirect rule: %+v", redirectRule)

				// Apply redirect rule to the route
				ac.Domains[domainIndex].Logic.HTTP.Routes[routeIndex].Redirect = redirectRule
				log.Printf("📍 Applied redirect rule for %s %s: %+v", route.Method, route.Link, redirectRule)
			} else {
				log.Printf("🚫 No redirect file found at %s: %v", redirectPath, err)
			}
		}
	}

	log.Printf("🏁 Redirect discovery completed")
	return nil
}

// discoverDomains scans the domains directory and builds domain configurations
func discoverDomains(root string) ([]DomainConfig, error) {
	domainsDir := filepath.Join(root, "domains")

	// Check if domains directory exists
	if _, err := os.Stat(domainsDir); os.IsNotExist(err) {
		return []DomainConfig{}, nil // No domains directory, return empty
	}

	var domains []DomainConfig

	// Walk through each domain directory
	entries, err := os.ReadDir(domainsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read domains directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		domainPath := filepath.Join(domainsDir, entry.Name())
		domain, err := parseDomain(root, domainPath, entry.Name())
		if err != nil {
			fmt.Printf("Warning: failed to parse domain %s: %v\n", entry.Name(), err)
			continue
		}

		domains = append(domains, domain)
	}

	return domains, nil
}

// parseDomain creates a DomainConfig from a domain directory
func parseDomain(root, domainPath, domainName string) (DomainConfig, error) {
	domain := DomainConfig{
		Name:     domainName,
		Path:     domainPath,
		ViewPath: filepath.Join("domains", domainName, "views"),
	}

	// Load domain-specific config if it exists
	configPath := filepath.Join(domainPath, DomainConfigFileName)
	if _, err := os.Stat(configPath); err == nil {
		configFile, err := os.ReadFile(configPath)
		if err != nil {
			return domain, fmt.Errorf("failed to read domain config: %w", err)
		}

		if err := yaml.Unmarshal(configFile, &domain); err != nil {
			return domain, fmt.Errorf("failed to parse domain config: %w", err)
		}
	}

	// Discover routes from file system
	routes, err := discoverRoutes(root, domainPath, domainName)
	if err != nil {
		return domain, fmt.Errorf("failed to discover routes: %w", err)
	}

	domain.Logic.HTTP.Routes = routes
	domain.Logic.HTTP.Restful = true // Enable RESTful routing by default

	return domain, nil
}

// discoverRoutes scans the domain directory for route files and builds route configurations
func discoverRoutes(root, domainPath, domainName string) ([]Route, error) {
	var routes []Route

	// Walk through the domain directory looking for route files
	err := filepath.Walk(domainPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-route files
		if info.IsDir() || !isRouteFile(path) {
			return nil
		}

		route, err := parseRouteFromPath(root, domainPath, domainName, path)
		if err != nil {
			fmt.Printf("Warning: failed to parse route from %s: %v\n", path, err)
			return nil
		}

		routes = append(routes, route)
		return nil
	})

	return routes, err
}

// isRouteFile determines if a file represents a route handler
func isRouteFile(path string) bool {
	// Look for files like: get.html.hbs, post.json.hbs, etc.
	filename := filepath.Base(path)

	// Pattern: {method}.{format}.hbs or {method}.{format}.handlebars
	patterns := []string{
		`^(get|post|put|patch|delete|head|options)\.(html|json|xml|sql|text)\.(hbs|handlebars)$`,
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, strings.ToLower(filename)); matched {
			return true
		}
	}

	return false
}

// parseRouteFromPath creates a Route configuration from a file path
func parseRouteFromPath(root, domainPath, domainName, filePath string) (Route, error) {
	// Get relative path from domain root
	relPath, err := filepath.Rel(domainPath, filePath)
	if err != nil {
		return Route{}, err
	}

	// Parse the file structure to determine route
	dir := filepath.Dir(relPath)
	filename := filepath.Base(relPath)

	// Extract method and format from filename (e.g., "get.html.hbs" -> method="get", format="html")
	parts := strings.Split(filename, ".")
	if len(parts) < 3 {
		return Route{}, fmt.Errorf("invalid route file format: %s", filename)
	}

	method := strings.ToUpper(parts[0])
	format := parts[1]

	// Build the URL path with proper handling
	urlPath := buildURLPath(domainName, dir)

	// Create a unique identifier for this route that includes format
	routeID := fmt.Sprintf("%s_%s_%s", method, strings.ReplaceAll(urlPath, "/", "_"), format)

	// Create the route
	route := Route{
		Method:   method,
		Link:     urlPath,
		View:     filename,
		Path:     routeID, // Use unique ID instead of file path
		ViewPath: filePath,
		Format:   format,
	}

	return route, nil
}

// buildURLPath converts a file system path to a URL path with correct parameter handling
func buildURLPath(domainName, dir string) string {
	// Handle the root index case
	if dir == "." || dir == "" || dir == "index" {
		return "/" + domainName
	}

	parts := []string{domainName}

	// Split the directory path and process each part
	pathParts := strings.Split(strings.Trim(dir, "/"), "/")
	for _, part := range pathParts {
		if part == "" || part == "." {
			continue
		}

		// Handle index directory - don't add it to URL
		if part == "index" {
			continue
		}

		// Convert [param] to :param for URL parameters
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			param := strings.Trim(part, "[]")
			parts = append(parts, ":"+param)
		} else {
			parts = append(parts, part)
		}
	}

	return "/" + strings.Join(parts, "/")
}

// Model helper methods
func (dc *DomainConfig) GetModel(name string) (Model, bool) {
	for _, modelDef := range dc.Models {
		if model, exists := modelDef[name]; exists {
			return model, true
		}
	}
	return nil, false
}

func (m Model) GetField(fieldName string) (Field, bool) {
	field, exists := m[fieldName]
	return field, exists
}

func (f Field) GetValidation(validationType string) (any, bool) {
	for _, validation := range f.Validations {
		if val, exists := validation[validationType]; exists {
			return val, true
		}
	}
	return nil, false
}

func (f Field) IsNullable() bool {
	if nullable, exists := f.GetValidation(Nullable); exists {
		if val, ok := nullable.(bool); ok {
			return val
		}
	}
	return true
}

func (f Field) GetLengthConstraints() (min, max int, hasConstraints bool) {
	if lengthVal, exists := f.GetValidation(ValidateLength); exists {
		if lengthMap, ok := lengthVal.(map[string]any); ok {
			var minVal, maxVal int
			var hasMin, hasMax bool
			if minInterface, exists := lengthMap[ValidateLengthMin]; exists {
				if minInt, ok := minInterface.(int); ok {
					minVal = minInt
					hasMin = true
				}
			}
			if maxInterface, exists := lengthMap[ValidateLengthMax]; exists {
				if maxInt, ok := maxInterface.(int); ok {
					maxVal = maxInt
					hasMax = true
				}
			}
			if hasMin || hasMax {
				return minVal, maxVal, true
			}
		}
	}
	return 0, 0, false
}

// Template discovery functions for the view system
func (dc *DomainConfig) GetTemplateDirectories(rootPath string) []string {
	var dirs []string

	// Domain-specific views
	domainViewsPath := filepath.Join(rootPath, "domains", dc.Name, "views")
	if _, err := os.Stat(domainViewsPath); err == nil {
		dirs = append(dirs, domainViewsPath)
	}

	return dirs
}

// GetAllTemplateDirectories returns all template directories for the app
func (ac *AppConfig) GetAllTemplateDirectories() []string {
	var allDirs []string

	// Add shared templates first (lower priority)
	sharedPath := filepath.Join(ac.Path, "shared", "views")
	if _, err := os.Stat(sharedPath); err == nil {
		allDirs = append(allDirs, sharedPath)
	}

	// Add domain-specific templates (higher priority)
	for _, domain := range ac.Domains {
		dirs := domain.GetTemplateDirectories(ac.Path)
		allDirs = append(allDirs, dirs...)
	}

	return allDirs
}

// Utility functions for backward compatibility
func FindDomainFiles(root string) ([]string, error) {
	var domainFiles []string

	domainsDir := filepath.Join(root, "domains")
	entries, err := os.ReadDir(domainsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			configPath := filepath.Join(domainsDir, entry.Name(), DomainConfigFileName)
			if _, err := os.Stat(configPath); err == nil {
				domainFiles = append(domainFiles, configPath)
			}
		}
	}

	return domainFiles, nil
}

// getDomainName extracts domain name from path
func getDomainName(domainPath string) string {
	return filepath.Base(filepath.Dir(domainPath))
}

// PrintYAML prints the configuration as YAML for debugging
func (ac *AppConfig) PrintYAML() {
	yamlData, err := yaml.Marshal(ac)
	if err != nil {
		fmt.Printf("Error marshaling YAML: %v", err)
		return
	}
	fmt.Print(string(yamlData))
}

// Validation and debugging functions
func (ac *AppConfig) ValidateRoutes() error {
	var errors []string

	for _, domain := range ac.Domains {
		for _, route := range domain.Logic.HTTP.Routes {
			// Check if template file exists
			if _, err := os.Stat(route.ViewPath); os.IsNotExist(err) {
				errors = append(errors,
					fmt.Sprintf("Missing template: %s %s -> %s",
						route.Method, route.Link, route.ViewPath))
			}

			// Validate route pattern
			if !strings.HasPrefix(route.Link, "/") {
				errors = append(errors,
					fmt.Sprintf("Invalid route pattern (must start with /): %s", route.Link))
			}

			// Validate HTTP method
			validMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
			valid := false
			for _, method := range validMethods {
				if route.Method == method {
					valid = true
					break
				}
			}
			if !valid {
				errors = append(errors,
					fmt.Sprintf("Invalid HTTP method: %s", route.Method))
			}

			// Validate format
			validFormats := []string{"html", "json", "xml", "sql", "text"}
			valid = false
			for _, format := range validFormats {
				if route.Format == format {
					valid = true
					break
				}
			}
			if !valid {
				errors = append(errors,
					fmt.Sprintf("Invalid format: %s", route.Format))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("route validation errors:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// DebugRoutes prints detailed route information for debugging
func (ac *AppConfig) DebugRoutes() {
	fmt.Println("=== Route Discovery Debug ===")

	for _, domain := range ac.Domains {
		fmt.Printf("Domain: %s\n", domain.Name)
		fmt.Printf("  Path: %s\n", domain.Path)
		fmt.Printf("  View Path: %s\n", domain.ViewPath)

		for _, route := range domain.Logic.HTTP.Routes {
			fmt.Printf("  Route:\n")
			fmt.Printf("    Method: %s\n", route.Method)
			fmt.Printf("    Link: %s\n", route.Link)
			fmt.Printf("    View: %s\n", route.View)
			fmt.Printf("    ViewPath: %s\n", route.ViewPath)
			fmt.Printf("    Format: %s\n", route.Format)
			fmt.Printf("    Path: %s\n", route.Path)
			if route.Redirect.To != "" {
				fmt.Printf("    Redirect: %+v\n", route.Redirect)
			}
			fmt.Println()
		}
	}

	fmt.Println("=============================")
}
