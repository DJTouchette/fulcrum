package lang_adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"fulcrum/lib/database"
	"fulcrum/lib/database/interfaces"
	parser "fulcrum/lib/parser"
	"fulcrum/lib/views"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// CreateRouteDispatcher creates the main HTTP route multiplexer
func CreateRouteDispatcher(appConfig *parser.AppConfig, frameworkServer *FrameworkServer) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check handler
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("🏥 Health check: %s %s", r.Method, r.URL.Path)
		fmt.Fprintf(w, "Status: OK\nTime: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	})

	// Group routes by method and pattern, but only register HTML routes
	// SQL routes are used internally for data fetching
	routeGroups := make(map[string]RouteGroup)

	for _, domain := range appConfig.Domains {
		for _, route := range domain.Logic.HTTP.Routes {
			key := fmt.Sprintf("%s %s", route.Method, route.Link)

			group := routeGroups[key]
			group.Domain = domain.Name
			group.Method = route.Method
			group.Pattern = route.Link

			if route.Format == "html" {
				group.HTMLRoute = &route
			} else if route.Format == "sql" {
				group.SQLRoute = &route
			}

			routeGroups[key] = group
		}
	}

	// Sort routes by specificity (more specific routes first)
	// This ensures /users/[user_id] is registered before /users
	type routeInfo struct {
		key         string
		group       RouteGroup
		specificity int
	}

	var sortedRoutes []routeInfo
	for key, group := range routeGroups {
		if group.HTMLRoute == nil {
			log.Printf("⚠️ Skipping route %s - no HTML template found", key)
			continue
		}

		// Calculate specificity: more path segments and fewer parameters = higher specificity
		specificity := calculateRouteSpecificity(group.Pattern)
		sortedRoutes = append(sortedRoutes, routeInfo{
			key:         key,
			group:       group,
			specificity: specificity,
		})
	}

	// Sort by specificity (higher specificity first)
	sort.Slice(sortedRoutes, func(i, j int) bool {
		return sortedRoutes[i].specificity > sortedRoutes[j].specificity
	})

	// Register routes in order of specificity
	for _, routeInfo := range sortedRoutes {
		group := routeInfo.group

		// Convert [param] syntax to Go's {param} syntax for ServeMux
		goPattern := convertToGoServeMuxPattern(group.Pattern)

		log.Printf("📝 Registering: %s %s -> %s (domain: %s, html: %s, sql: %s)",
			group.Method, group.Pattern, goPattern, group.Domain,
			group.HTMLRoute.View,
			func() string {
				if group.SQLRoute != nil {
					return group.SQLRoute.View
				}
				return "none"
			}())

		// Capture variables in closure
		capturedGroup := group

		// Create handler function for this pattern
		handlerFunc := func(w http.ResponseWriter, r *http.Request) {
			log.Printf("🔍 Request: %s %s", r.Method, r.URL.Path)

			// Check method
			if r.Method != capturedGroup.Method {
				log.Printf("❌ Method mismatch: got %s, expected %s", r.Method, capturedGroup.Method)
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			// Handle the request
			handleHTMLRouteWithSQL(w, r, capturedGroup, appConfig, frameworkServer)
		}

		// Register the handler with Go's pattern syntax
		mux.HandleFunc(fmt.Sprintf("%s %s", group.Method, goPattern), handlerFunc)
	}

	// Catch-all for debugging unmatched routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("🚫 Unmatched request: %s %s", r.Method, r.URL.Path)

		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "No route found for %s %s\n\n", r.Method, r.URL.Path)
		fmt.Fprintf(w, "Available routes:\n")

		for _, routeInfo := range sortedRoutes {
			group := routeInfo.group
			goPattern := convertToGoServeMuxPattern(group.Pattern)
			fmt.Fprintf(w, "  %s %s -> %s (html: %s, sql: %s)\n",
				group.Method, goPattern, group.Pattern,
				group.HTMLRoute.View,
				func() string {
					if group.SQLRoute != nil {
						return group.SQLRoute.View
					}
					return "none"
				}())
		}
	})

	return mux
}

// calculateRouteSpecificity calculates how specific a route is
// Higher numbers = more specific routes that should be registered first
func calculateRouteSpecificity(pattern string) int {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	specificity := len(parts) * 10 // Base score for number of segments

	for _, part := range parts {
		if strings.HasPrefix(part, ":") || (strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]")) {
			// Parameter segment - less specific
			specificity -= 5
		} else if part != "" {
			// Literal segment - more specific
			specificity += 3
		}
	}

	return specificity
}

// convertToGoServeMuxPattern converts our [param] syntax to Go 1.22+ ServeMux {param} syntax
func convertToGoServeMuxPattern(pattern string) string {
	// Convert [param] to {param}
	result := pattern

	// Use regex to find [param] patterns and convert them
	re := regexp.MustCompile(`\[([^\]]+)\]`)
	result = re.ReplaceAllString(result, "{$1}")

	// Convert :param to {param} (in case we have both syntaxes)
	re2 := regexp.MustCompile(`:([^/]+)`)
	result = re2.ReplaceAllString(result, "{$1}")

	return result
}

// RouteGroup represents a route with its HTML and SQL components
type RouteGroup struct {
	Domain    string
	Method    string
	Pattern   string
	HTMLRoute *parser.Route // The .html.hbs file for rendering
	SQLRoute  *parser.Route // The .sql.hbs file for data fetching
}

// handleHTMLRouteWithSQL handles a route by optionally executing SQL and rendering HTML
func handleHTMLRouteWithSQL(w http.ResponseWriter, r *http.Request, group RouteGroup, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) {
	log.Printf("🎯 Processing route: %s %s", group.Method, group.Pattern)

	// Extract request data (path params, query params, etc.)
	requestData := extractRequestData(r, *group.HTMLRoute)
	log.Printf("📊 Request data: %+v", requestData)

	var templateData any = requestData

	// If there's a SQL route, execute it to get data
	if group.SQLRoute != nil {
		log.Printf("🗄️ Executing SQL template: %s", group.SQLRoute.View)

		sqlData, err := executeSQL(group.SQLRoute, requestData, appConfig, frameworkServer)
		if err != nil {
			log.Printf("❌ SQL execution failed: %v", err)
			// Continue with just request data, don't fail the whole request
		} else {
			// Use SQL data directly as the main template data
			// This allows templates to use {{#each this}} for the data array
			templateData = sqlData
			log.Printf("📦 SQL data set as template data: %+v", sqlData)
		}
	}

	// Skip domain logic processing for now to keep data structure simple
	// The SQL data should be the primary template data
	log.Printf("🎨 Rendering HTML template: %s", group.HTMLRoute.View)

	html, err := loadAndRenderTemplate(group.HTMLRoute.ViewPath, templateData, appConfig.Views)
	if err != nil {
		log.Printf("❌ Template render failed: %v", err)

		// Return a helpful error page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<html>
			<head><title>Template Error</title></head>
			<body>
				<h1>Template Rendering Failed</h1>
				<p><strong>Error:</strong> %s</p>
				<p><strong>Template:</strong> %s</p>
				<p><strong>Template Path:</strong> %s</p>
				<p><strong>Data Type:</strong> %T</p>
				<p><strong>Data Preview:</strong> <pre>%+v</pre></p>
			</body>
			</html>
		`, err.Error(), group.HTMLRoute.View, group.HTMLRoute.ViewPath, templateData, templateData)
		return
	}

	log.Printf("✅ Template rendered successfully (length: %d)", len(html))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// executeSQL renders the SQL template and executes it against the database
func executeSQL(sqlRoute *parser.Route, requestData map[string]any, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) (any, error) {
	// Load and render the SQL template to generate the actual SQL query
	sqlQuery, err := loadAndRenderSQLTemplate(sqlRoute.ViewPath, requestData, appConfig.Views)
	if err != nil {
		return nil, fmt.Errorf("failed to render SQL template: %w", err)
	}

	log.Printf("🔍 Generated SQL query: %s", sqlQuery)

	// Execute the SQL query using the database executor
	if frameworkServer != nil && frameworkServer.dbExecutor != nil {
		// Use the real database executor
		ctx := context.Background()
		resultJSON, err := frameworkServer.dbExecutor.ExecuteSQL(ctx, sqlQuery, requestData, nil)
		if err != nil {
			log.Printf("❌ Database execution failed: %v", err)
			return nil, fmt.Errorf("database execution failed: %w", err)
		}

		// Parse the JSON response
		var dbResponse struct {
			Success bool             `json:"success"`
			Data    []map[string]any `json:"data"`
			Error   string           `json:"error"`
			Count   int              `json:"count"`
		}

		if err := json.Unmarshal(resultJSON, &dbResponse); err != nil {
			log.Printf("❌ Failed to parse database response: %v", err)
			return nil, fmt.Errorf("failed to parse database response: %w", err)
		}

		if !dbResponse.Success {
			log.Printf("❌ Database query failed: %s", dbResponse.Error)
			return nil, fmt.Errorf("database query failed: %s", dbResponse.Error)
		}

		log.Printf("✅ Database query successful: %d records", dbResponse.Count)

		// Return the data array directly as the main template data
		// This allows templates to use {{#each this}} directly
		return dbResponse.Data, nil
	}

	// Fallback to mock data if no database executor
	log.Printf("⚠️ No database executor available, using mock data")
	mockData := []map[string]any{
		{"id": 1, "name": "John Doe", "email": "john@example.com", "age": 30},
		{"id": 2, "name": "Jane Smith", "email": "jane@example.com", "age": 28},
		{"id": 3, "name": "Bob Johnson", "email": "bob@example.com", "age": 35},
	}

	return mockData, nil
}

// loadAndRenderSQLTemplate loads a SQL template file and renders it to generate SQL
func loadAndRenderSQLTemplate(templatePath string, data any, renderer *views.TemplateRenderer) (string, error) {
	// Create a temporary template name based on the file path
	tempName := fmt.Sprintf("sql_%d", time.Now().UnixNano())

	// Load the template
	if err := renderer.LoadTemplate(tempName, templatePath); err != nil {
		return "", fmt.Errorf("failed to load SQL template: %w", err)
	}

	// Render the SQL template (no layout for SQL)
	sql, err := renderer.Render(tempName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render SQL template: %w", err)
	}

	return sql, nil
}

// mergeMaps merges two maps, with the second map taking precedence
func mergeMaps(map1, map2 map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy first map
	for k, v := range map1 {
		result[k] = v
	}

	// Copy second map (overwriting any conflicts)
	for k, v := range map2 {
		result[k] = v
	}

	return result
}

// handleSingleRoute handles a single route request
func handleSingleRoute(w http.ResponseWriter, r *http.Request, route parser.Route, domainName string, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) {
	log.Printf("✅ Processing route: %s %s (format: %s, template: %s)",
		route.Method, route.Link, route.Format, route.View)

	// Extract request data
	requestData := extractRequestData(r, route)
	log.Printf("📊 Request data: %+v", requestData)

	switch route.Format {
	case "html":
		handleHTMLRoute(w, r, route, requestData, appConfig, frameworkServer)
	case "json":
		handleJSONRoute(w, r, route, requestData, appConfig, frameworkServer)
	case "sql":
		handleSQLRoute(w, r, route, requestData, appConfig)
	default:
		log.Printf("❌ Unsupported format: %s", route.Format)
		http.Error(w, fmt.Sprintf("Unsupported format: %s", route.Format), http.StatusBadRequest)
	}
}

// Helper function to check if a path matches a pattern (simple implementation)
func matchesPattern(path, pattern string) bool {
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")

	if len(pathParts) != len(patternParts) {
		return false
	}

	for i, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, ":") {
			// This is a parameter, it matches any value
			continue
		}
		if pathParts[i] != patternPart {
			return false
		}
	}

	return true
}

// Helper function to get allowed methods for a pattern
func getAllowedMethods(routeGroups map[string][]parser.Route, pattern string) []string {
	var methods []string
	for key := range routeGroups {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 2 && parts[1] == pattern {
			methods = append(methods, parts[0])
		}
	}
	return methods
}

// determineFormat determines the desired response format from request
func determineFormat(r *http.Request, routes []parser.Route) string {
	// Check query parameter first (?format=json)
	if format := r.URL.Query().Get("format"); format != "" {
		// Validate that this format exists in the routes
		for _, route := range routes {
			if route.Format == format {
				return format
			}
		}
	}

	// Check Accept header
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		// Look for json format in available routes
		for _, route := range routes {
			if route.Format == "json" {
				return "json"
			}
		}
	}
	if strings.Contains(accept, "text/html") {
		// Look for html format in available routes
		for _, route := range routes {
			if route.Format == "html" {
				return "html"
			}
		}
	}

	// Default to html if available, otherwise first format
	for _, route := range routes {
		if route.Format == "html" {
			return "html"
		}
	}

	// If no HTML format, return the first available format
	if len(routes) > 0 {
		return routes[0].Format
	}

	return "html"
}

// Helper function to get formats string for logging
func getFormatsString(routes []parser.Route) string {
	formats := make([]string, len(routes))
	for i, route := range routes {
		formats[i] = route.Format
	}
	return strings.Join(formats, ", ")
}

// createMultiFormatHandler creates a handler that can serve different formats based on request
func createMultiFormatHandler(routes []parser.Route, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine desired format from Accept header or query parameter
		desiredFormat := determineFormat(r, routes)

		// Find the route for the desired format
		var selectedRoute *parser.Route
		for _, route := range routes {
			if route.Format == desiredFormat {
				selectedRoute = &route
				break
			}
		}

		// Fallback to first route if format not found
		if selectedRoute == nil && len(routes) > 0 {
			selectedRoute = &routes[0]
		}

		if selectedRoute == nil {
			http.Error(w, "No handler found for this route", http.StatusNotFound)
			return
		}

		log.Printf("🔍 Handling %s %s with format: %s (template: %s)",
			r.Method, r.URL.Path, selectedRoute.Format, selectedRoute.View)

		// Handle the request based on format
		handleRouteByFormat(w, r, *selectedRoute, appConfig, frameworkServer)
	}
}

// handleRouteByFormat handles the request based on the route format
func handleRouteByFormat(w http.ResponseWriter, r *http.Request, route parser.Route, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) {
	// Extract path parameters and request data
	requestData := extractRequestData(r, route)

	switch route.Format {
	case "html":
		handleHTMLRoute(w, r, route, requestData, appConfig, frameworkServer)
	case "json":
		handleJSONRoute(w, r, route, requestData, appConfig, frameworkServer)
	case "sql":
		handleSQLRoute(w, r, route, requestData, appConfig)
	default:
		http.Error(w, fmt.Sprintf("Unsupported format: %s", route.Format), http.StatusBadRequest)
	}
}

// handleHTMLRoute handles HTML template rendering
func handleHTMLRoute(w http.ResponseWriter, r *http.Request, route parser.Route, requestData map[string]any, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) {
	log.Printf("🎨 Rendering HTML template: %s", route.View)

	// For HTML routes, we might want to:
	// 1. Process business logic via domain communication
	// 2. Render template with the result

	// First, try to get data from domain logic if there's a domain process
	var templateData map[string]any = requestData

	// If this route has domain logic, call it
	if frameworkServer != nil {
		domainData, err := callDomainLogic(r, route, requestData, frameworkServer)
		if err == nil && domainData != nil {
			templateData = domainData
			log.Printf("📦 Domain data received: %+v", templateData)
		} else if err != nil {
			log.Printf("⚠️ Domain logic error: %v", err)
		}
	}

	// The route files like get.html.hbs are not loaded as templates
	// We need to load them directly from the file system
	log.Printf("🔍 Loading template from file: %s", route.ViewPath)

	// Check if the template file exists
	if _, err := os.Stat(route.ViewPath); os.IsNotExist(err) {
		log.Printf("❌ Template file not found: %s", route.ViewPath)
		http.Error(w, fmt.Sprintf("Template file not found: %s", route.ViewPath), http.StatusInternalServerError)
		return
	}

	// Load and render the template directly
	html, err := loadAndRenderTemplate(route.ViewPath, templateData, appConfig.Views)
	if err != nil {
		log.Printf("❌ Template render failed: %v", err)

		// Return a helpful error page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<html>
			<head><title>Template Error</title></head>
			<body>
				<h1>Template Rendering Failed</h1>
				<p><strong>Error:</strong> %s</p>
				<p><strong>Template:</strong> %s</p>
				<p><strong>Template Path:</strong> %s</p>
				<p><strong>Data:</strong> <pre>%+v</pre></p>
			</body>
			</html>
		`, err.Error(), route.View, route.ViewPath, templateData)
		return
	}

	log.Printf("✅ Template rendered successfully (length: %d)", len(html))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// loadAndRenderTemplate loads a template file and renders it intelligently
func loadAndRenderTemplate(templatePath string, data any, renderer *views.TemplateRenderer) (string, error) {
	// Create a temporary template name based on the file path
	tempName := fmt.Sprintf("route_%d", time.Now().UnixNano())

	// Load the template
	if err := renderer.LoadTemplate(tempName, templatePath); err != nil {
		return "", fmt.Errorf("failed to load template: %w", err)
	}

	// First, render the template to see its content
	content, err := renderer.Render(tempName, data)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	// Check if this is a complete HTML document
	contentTrimmed := strings.TrimSpace(content)
	isCompleteDocument := strings.HasPrefix(strings.ToLower(contentTrimmed), "<!doctype html") ||
		strings.HasPrefix(strings.ToLower(contentTrimmed), "<html")

	if isCompleteDocument {
		// This is a complete document, return as-is
		log.Printf("📄 Template is complete document, rendering directly")
		return content, nil
	} else {
		// This is content that should go in a layout
		log.Printf("📄 Template is content, rendering with layout")

		// Prepare layout data
		layoutData := map[string]any{
			"body": content,
		}

		// If data is a map, merge it with layout data for layout context
		if dataMap, ok := data.(map[string]any); ok {
			for key, value := range dataMap {
				if key != "body" { // Don't override body
					layoutData[key] = value
				}
			}
		}

		// Render with layout
		html, err := renderer.Render("layouts/main", layoutData)
		if err != nil {
			// If layout fails, return the content as-is
			log.Printf("⚠️ Layout render failed, returning content directly: %v", err)
			return content, nil
		}

		return html, nil
	}
}

// handleJSONRoute handles JSON API responses
func handleJSONRoute(w http.ResponseWriter, r *http.Request, route parser.Route, requestData map[string]any, appConfig *parser.AppConfig, frameworkServer *FrameworkServer) {
	// For JSON routes, process via domain logic and return JSON
	var responseData any = map[string]any{
		"success": true,
		"data":    requestData,
	}

	if frameworkServer != nil {
		domainData, err := callDomainLogic(r, route, requestData, frameworkServer)
		if err != nil {
			responseData = map[string]any{
				"success": false,
				"error":   err.Error(),
			}
		} else if domainData != nil {
			responseData = domainData
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responseData)
}

// handleSQLRoute handles SQL template rendering (for debugging/development)
func handleSQLRoute(w http.ResponseWriter, r *http.Request, route parser.Route, requestData map[string]any, appConfig *parser.AppConfig) {
	sqlQuery, err := appConfig.Views.Render(route.View, requestData)
	if err != nil {
		http.Error(w, fmt.Sprintf("SQL template error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(sqlQuery))
}

// callDomainLogic communicates with domain process for business logic
func callDomainLogic(r *http.Request, route parser.Route, requestData map[string]any, frameworkServer *FrameworkServer) (map[string]any, error) {
	// This would communicate with the domain process
	// For now, just return the request data with some mock processing

	// Add some mock data to demonstrate the system working
	mockData := map[string]any{
		"users": []map[string]any{
			{"id": 1, "name": "John Doe", "email": "john@example.com"},
			{"id": 2, "name": "Jane Smith", "email": "jane@example.com"},
		},
		"request_data": requestData,
		"processed_at": time.Now().Format(time.RFC3339),
	}

	return mockData, nil
}

// extractRequestData extracts all relevant data from the HTTP request
func extractRequestData(r *http.Request, route parser.Route) map[string]any {
	data := make(map[string]any)

	// In Go 1.22+, path values are available via r.PathValue()
	// Extract path parameters based on the route pattern
	pathParams := extractPathParametersFromGoServeMux(r, route.Link)
	for k, v := range pathParams {
		data[k] = v
	}

	// Add query parameters
	for k, v := range r.URL.Query() {
		if len(v) == 1 {
			data[k] = v[0]
		} else {
			data[k] = v
		}
	}

	// For POST/PUT, also include form data
	if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
		if err := r.ParseForm(); err == nil {
			for k, v := range r.Form {
				if len(v) == 1 {
					data[k] = v[0]
				} else {
					data[k] = v
				}
			}
		}
	}

	// Add request metadata
	data["_method"] = r.Method
	data["_path"] = r.URL.Path
	data["_route"] = route.Link

	return data
}

// extractPathParametersFromGoServeMux extracts parameters using Go 1.22+ ServeMux
func extractPathParametersFromGoServeMux(r *http.Request, routePattern string) map[string]string {
	params := make(map[string]string)

	// Extract parameter names from the route pattern
	// Convert [param] to param names
	re := regexp.MustCompile(`\[([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(routePattern, -1)

	for _, match := range matches {
		if len(match) > 1 {
			paramName := match[1]
			// Use Go 1.22+ PathValue method
			if value := r.PathValue(paramName); value != "" {
				params[paramName] = value
			}
		}
	}

	// Also handle :param syntax
	re2 := regexp.MustCompile(`:([^/]+)`)
	matches2 := re2.FindAllStringSubmatch(routePattern, -1)

	for _, match := range matches2 {
		if len(match) > 1 {
			paramName := match[1]
			if value := r.PathValue(paramName); value != "" {
				params[paramName] = value
			}
		}
	}

	return params
}

// extractPathParameters extracts parameters from URL path (legacy version)
func extractPathParameters(actualPath, routePattern string) map[string]string {
	params := make(map[string]string)

	// Split paths into segments
	actualSegments := strings.Split(strings.Trim(actualPath, "/"), "/")
	patternSegments := strings.Split(strings.Trim(routePattern, "/"), "/")

	// Match segments and extract parameters
	for i, patternSegment := range patternSegments {
		if i >= len(actualSegments) {
			break
		}

		// Check if this segment is a parameter (starts with :)
		if strings.HasPrefix(patternSegment, ":") {
			paramName := strings.TrimPrefix(patternSegment, ":")
			params[paramName] = actualSegments[i]
		}
	}

	return params
}

// StartHTTPServerWithConfig starts HTTP server using the parsed configuration
func StartHTTPServerWithConfig(appConfig *parser.AppConfig, frameworkServer *FrameworkServer) *http.Server {
	// Create the route dispatcher with the fixed logic
	mux := CreateRouteDispatcher(appConfig, frameworkServer)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Printf("🚀 HTTP Server starting on http://localhost%s\n", server.Addr)
	fmt.Println("📍 Registered routes:")

	// Group and log routes properly
	routeGroups := make(map[string][]string)
	for _, domain := range appConfig.Domains {
		for _, route := range domain.Logic.HTTP.Routes {
			key := fmt.Sprintf("%s %s", route.Method, route.Link)
			routeGroups[key] = append(routeGroups[key], route.Format)
		}
	}

	for pattern, formats := range routeGroups {
		fmt.Printf("   %s (formats: %s)\n", pattern, strings.Join(formats, ", "))
	}
	fmt.Println("   GET /health -> Health check")
	fmt.Println()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}

// StartGRPCServerWithShutdown starts gRPC server and returns server instance for shutdown control
func StartGRPCServerWithShutdown(frameworkServer *FrameworkServer) *grpc.Server {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on port 50051: %v", err)
	}

	server := grpc.NewServer()
	reflection.Register(server)
	RegisterFrameworkServiceServer(server, frameworkServer)

	log.Println("gRPC server starting on :50051")

	// Start in goroutine
	go func() {
		if err := server.Serve(listener); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return server
}

// StartBothServersWithConfig starts the servers using the new file-system based config
func StartBothServersWithConfig(appConfig *parser.AppConfig) {
	// --- Database Setup ---
	dbConfig := interfaces.Config{
		Driver:          interfaces.DatabaseDriver(appConfig.DB.Driver),
		Host:            appConfig.DB.Host,
		Port:            appConfig.DB.Port,
		Username:        appConfig.DB.Username,
		Password:        appConfig.DB.Password,
		Database:        appConfig.DB.Database,
		SSLMode:         appConfig.DB.SSLMode,
		MaxOpenConns:    appConfig.DB.MaxOpenConns,
		MaxIdleConns:    appConfig.DB.MaxIdleConns,
		ConnMaxLifetime: time.Duration(appConfig.DB.ConnMaxLifetime) * time.Minute,
		FilePath:        appConfig.DB.FilePath,
	}

	dbManager, err := database.NewManager(dbConfig)
	if err != nil {
		log.Fatalf("Failed to create database manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := dbManager.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}
	defer dbManager.Close()

	db := dbManager.GetDatabase()

	// --- Framework Server Setup ---
	frameworkServer := &FrameworkServer{
		db:              db,
		dbExecutor:      database.NewDatabaseExecutor(db),
		domainStreams:   make(map[string]FrameworkService_DomainCommunicationServer),
		pendingRequests: make(map[string]*PendingRequest),
	}
	frameworkServer.startCleanupRoutine()

	// --- Enhanced Renderer Setup ---
	log.Println("Setting up template renderer...")

	// Log discovered domains and their template directories
	log.Printf("Discovered %d domains:", len(appConfig.Domains))
	for _, domain := range appConfig.Domains {
		log.Printf("  - Domain: %s", domain.Name)
		log.Printf("    Path: %s", domain.Path)
		log.Printf("    Routes: %d", len(domain.Logic.HTTP.Routes))
		for _, route := range domain.Logic.HTTP.Routes {
			log.Printf("      %s %s -> %s", route.Method, route.Link, route.ViewPath)
		}
	}

	// Get all template directories
	templateDirs := appConfig.GetAllTemplateDirectories()
	log.Printf("Template directories found: %v", templateDirs)

	// Setup renderer with the new system
	renderer, err := views.SetupViewsFromConfig(appConfig)
	if err != nil {
		log.Fatalf("Failed to setup views: %v", err)
	}

	appConfig.Views = renderer

	// --- Validate Routes and Templates ---
	if err := appConfig.ValidateRoutes(); err != nil {
		log.Printf("Warning: Route validation issues found: %v", err)
		// Don't fail, just warn - some templates might be loaded dynamically
	}

	// --- Start Servers ---
	log.Println("Starting gRPC server...")
	grpcServer := StartGRPCServerWithShutdown(frameworkServer)

	log.Println("Starting HTTP server...")
	httpServer := StartHTTPServerWithConfig(appConfig, frameworkServer)

	log.Println("Servers started successfully!")
	log.Printf("HTTP routes registered:")
	printRegisteredRoutes(appConfig)

	// --- Graceful Shutdown ---
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	log.Println("Application ready. Press Ctrl+C to shutdown.")
	<-c

	log.Println("Shutting down servers...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Shutdown gRPC server
	grpcServer.GracefulStop()

	log.Println("Servers gracefully stopped.")
}

// printRegisteredRoutes logs all registered routes for debugging
func printRegisteredRoutes(appConfig *parser.AppConfig) {
	for _, domain := range appConfig.Domains {
		log.Printf("Domain '%s' routes:", domain.Name)
		for _, route := range domain.Logic.HTTP.Routes {
			log.Printf("  %s %s (format: %s, template: %s)",
				route.Method, route.Link, route.Format, route.View)
		}
	}
}

// StartBothServersInDevMode starts servers with development features enabled
func StartBothServersInDevMode(appConfig *parser.AppConfig) {
	log.Println("Starting in DEVELOPMENT mode")

	// In dev mode, we might want different behaviors:
	// - Hot reloading templates
	// - More verbose logging
	// - Different error handling

	// Setup development renderer
	renderer, err := views.SetupViewsForDevelopment(appConfig)
	if err != nil {
		log.Fatalf("Failed to setup development views: %v", err)
	}
	appConfig.Views = renderer

	// Enable hot reloading if needed
	if err := setupHotReloading(appConfig); err != nil {
		log.Printf("Warning: Could not setup hot reloading: %v", err)
	}

	// Continue with normal startup but with dev features
	StartBothServersWithConfig(appConfig)
}

// setupHotReloading sets up file watching for template changes
func setupHotReloading(appConfig *parser.AppConfig) error {
	// This would implement file watching using something like fsnotify
	// For now, just log that it would be implemented
	log.Println("Hot reloading would be implemented here")

	templateDirs := appConfig.GetAllTemplateDirectories()
	for _, dir := range templateDirs {
		log.Printf("Would watch directory for changes: %s", dir)
	}

	return nil
}

// Legacy functions for backward compatibility

// StartGRPCServer starts the gRPC server with the given FrameworkServer (legacy)
func StartGRPCServer(frameworkServer *FrameworkServer) {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on port 50051: %v", err)
	}

	// Create gRPC server
	server := grpc.NewServer()
	reflection.Register(server)

	// Register the framework service
	RegisterFrameworkServiceServer(server, frameworkServer)

	log.Println("gRPC server starting on :50051")

	// Start serving (this blocks)
	if err := server.Serve(listener); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}

// StartHTTPServerWithShutdown starts HTTP server and returns server instance for shutdown control (legacy)
func StartHTTPServerWithShutdown(frameworkServer *FrameworkServer) *http.Server {
	mux := http.NewServeMux()

	// Health check handler
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Status: OK\nTime: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	})

	// Catch-all handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Extract information from the HTTP request
		domain := r.Header.Get("X-Domain")
		if domain == "" {
			domain = "default"
		}

		msgType := r.Header.Get("X-Message-Type")
		if msgType == "" {
			msgType = "http_request"
		}

		// Create payload with request info
		payload := fmt.Sprintf(`{"method": "%s", "path": "%s", "query": "%s"}`,
			r.Method, r.URL.Path, r.URL.RawQuery)

		// Send message directly to FrameworkServer instance
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		domainMsg := &DomainMessage{
			Domain:    domain,
			Type:      msgType,
			Payload:   payload,
			RequestId: fmt.Sprintf("http-%d", time.Now().UnixNano()),
		}

		response, err := frameworkServer.SendMessage(ctx, domainMsg)
		if err != nil {
			log.Printf("Error processing message: %v", err)
			fmt.Fprintf(w, "Error: Failed to process request\n")
			return
		}

		// Return response from FrameworkServer
		fmt.Fprintf(w, "Response from FrameworkServer:\n")
		fmt.Fprintf(w, "Type: %s\n", response.Type)
		fmt.Fprintf(w, "Success: %t\n", response.Success)
		fmt.Fprintf(w, "Payload: %s\n", response.Payload)
		if response.Error != "" {
			fmt.Fprintf(w, "Error: %s\n", response.Error)
		}
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Printf("🚀 HTTP Server starting on http://localhost%s\n", server.Addr)
	fmt.Println("📍 Available endpoints:")
	fmt.Println("   GET /health - Health check")
	fmt.Println("   ANY /* - Send message to FrameworkServer")
	fmt.Println()
	fmt.Println("💡 Use headers to control message:")
	fmt.Println("   X-Domain: specify target domain")
	fmt.Println("   X-Message-Type: specify message type")
	fmt.Println()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}
