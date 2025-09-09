package framework

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"fulcrum/lib/auth"
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

	lang_adapters "fulcrum/lib/lang/adapters"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// HTMXRequest contains HTMX-specific request information
type HTMXRequest struct {
	IsHTMX         bool
	Trigger        string
	TriggerName    string
	Target         string
	CurrentURL     string
	Prompt         string
	Request        bool
	Boosted        bool
	HistoryRestore bool
}

// parseHTMXHeaders extracts HTMX-specific headers from the request
func parseHTMXHeaders(r *http.Request) HTMXRequest {
	return HTMXRequest{
		IsHTMX:         r.Header.Get("HX-Request") == "true",
		Trigger:        r.Header.Get("HX-Trigger"),
		TriggerName:    r.Header.Get("HX-Trigger-Name"),
		Target:         r.Header.Get("HX-Target"),
		CurrentURL:     r.Header.Get("HX-Current-URL"),
		Prompt:         r.Header.Get("HX-Prompt"),
		Request:        r.Header.Get("HX-Request") == "true",
		Boosted:        r.Header.Get("HX-Boosted") == "true",
		HistoryRestore: r.Header.Get("HX-History-Restore-Request") == "true",
	}
}

// setHTMXResponseHeaders sets HTMX-specific response headers
func setHTMXResponseHeaders(w http.ResponseWriter, options map[string]string) {
	for key, value := range options {
		switch key {
		case "trigger":
			w.Header().Set("HX-Trigger", value)
		case "trigger-after-settle":
			w.Header().Set("HX-Trigger-After-Settle", value)
		case "trigger-after-swap":
			w.Header().Set("HX-Trigger-After-Swap", value)
		case "redirect":
			w.Header().Set("HX-Redirect", value)
		case "refresh":
			w.Header().Set("HX-Refresh", value)
		case "location":
			w.Header().Set("HX-Location", value)
		case "push-url":
			w.Header().Set("HX-Push-Url", value)
		case "replace-url":
			w.Header().Set("HX-Replace-Url", value)
		case "reswap":
			w.Header().Set("HX-Reswap", value)
		case "retarget":
			w.Header().Set("HX-Retarget", value)
		case "reselect":
			w.Header().Set("HX-Reselect", value)
		}
	}
}

// extractHTMXHeaders extracts HTMX response headers from template data
func extractHTMXHeaders(data any) map[string]string {
	headers := make(map[string]string)

	// Check if data contains HTMX response instructions
	if dataMap, ok := data.(map[string]any); ok {
		if htmxData, exists := dataMap["htmx_response"]; exists {
			if htmxMap, ok := htmxData.(map[string]any); ok {
				for key, value := range htmxMap {
					if strValue, ok := value.(string); ok {
						headers[key] = strValue
					}
				}
			}
		}

		// Check for common response patterns
		if redirect, exists := dataMap["redirect_to"]; exists {
			if redirectStr, ok := redirect.(string); ok {
				headers["redirect"] = redirectStr
			}
		}

		if trigger, exists := dataMap["htmx_trigger"]; exists {
			if triggerStr, ok := trigger.(string); ok {
				headers["trigger"] = triggerStr
			}
		}
	}

	return headers
}

// extractBodyContent extracts content from within <body> tags
func extractBodyContent(html string) string {
	// Simple regex to extract body content
	re := regexp.MustCompile(`(?s)<body[^>]*>(.*?)</body>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return html // Return as-is if no body tags found
}

// wrapInLayout wraps content in the main layout
func wrapInLayout(content string, data any, renderer *views.TemplateRenderer) (string, error) {
	layoutData := map[string]any{
		"body": content,
	}

	if dataMap, ok := data.(map[string]any); ok {
		for key, value := range dataMap {
			if key != "body" {
				layoutData[key] = value
			}
		}
	}

	html, err := renderer.Render("layouts/main", layoutData)
	if err != nil {
		log.Printf("⚠️ Layout render failed, returning content directly: %v", err)
		return content, nil
	}

	return html, nil
}

// CreateRouteDispatcher creates the main HTTP route multiplexer with HTMX support
func CreateRouteDispatcher(appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) *http.ServeMux {
	mux := http.NewServeMux()

	// Track registered routes to avoid conflicts
	registeredRoutes := make(map[string]bool)

	// Health check handler
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("🏥 Health check: %s %s", r.Method, r.URL.Path)
		fmt.Fprintf(w, "Status: OK\nTime: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	})

	// HTMX static assets handler
	mux.HandleFunc("GET /htmx.min.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year cache
		// Serve HTMX from CDN or embedded version
		http.Redirect(w, r, "https://unpkg.com/htmx.org@1.9.10/dist/htmx.min.js", http.StatusMovedPermanently)
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

	rootGroup := sortedRoutes[0].group

	// Register routes in order of specificity
	for _, routeInfo := range sortedRoutes {
		group := routeInfo.group
		if group.Pattern == appConfig.Root {
			rootGroup = group
			rootGroup.Pattern = "/"
		}

		// Convert [param] syntax to Go's {param} syntax for ServeMux
		goPattern := convertToGoServeMuxPattern(group.Pattern)
		routeKey := fmt.Sprintf("%s %s", group.Method, goPattern)

		// Check if this route is already registered
		if registeredRoutes[routeKey] {
			log.Printf("⏭️ Skipping duplicate route: %s (already registered)", routeKey)
			continue
		}

		log.Printf("📝 Registering: %s %s -> %s (domain: %s, html: %s, sql: %s)",
			group.Method, group.Pattern, goPattern, group.Domain,
			group.HTMLRoute.View,
			func() string {
				if group.SQLRoute != nil {
					return group.SQLRoute.View
				}
				return "none"
			}())

		// Mark this route as registered
		registeredRoutes[routeKey] = true

		// Capture variables in closure
		capturedGroup := group

		// Create handler function for this pattern with HTMX support
		handlerFunc := func(w http.ResponseWriter, r *http.Request) {
			// Skip authentication check for auth domain routes - they handle auth themselves
			if capturedGroup.Domain != "auth" && !auth.IsAuthenticated(r) {
				log.Printf("🔍 Request: %s %s has been redirected to login", r.Method, r.URL.Path)
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}

			log.Printf("🔍 Request: %s %s", r.Method, r.URL.Path)

			// Parse HTMX headers
			htmxReq := parseHTMXHeaders(r)
			if htmxReq.IsHTMX {
				log.Printf("🔄 HTMX Request detected: trigger=%s, target=%s", htmxReq.Trigger, htmxReq.Target)
			}

			// Check method
			if r.Method != capturedGroup.Method {
				log.Printf("❌ Method mismatch: got %s, expected %s", r.Method, capturedGroup.Method)
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			// Determine the desired format from query params or Accept header
			requestedFormat := determineRequestedFormat(r)
			log.Printf("🎯 Requested format: %s", requestedFormat)

			// Handle based on the requested format
			if requestedFormat == "json" {
				// Extract request data for JSON handling
				requestData := extractRequestData(r, *capturedGroup.HTMLRoute)
				handleJSONRoute(w, r, *capturedGroup.HTMLRoute, requestData, appConfig, frameworkServer)
			} else {
				// Handle HTML/HTMX requests
				handleHTMLRouteWithProcessManager(w, r, capturedGroup, appConfig, frameworkServer)
			}
		}

		// Register the handler with Go's pattern syntax
		mux.HandleFunc(fmt.Sprintf("%s %s", group.Method, goPattern), handlerFunc)
	}

	// Catch-all for debugging unmatched routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if appConfig.Root != "" {
			fmt.Println("")
			handleHTMLRouteWithProcessManager(w, r, rootGroup, appConfig, frameworkServer)
			return
		}

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

func extractActionFromRoute(pattern, method string) string {
	// For /users/:user_id/edit, we want "user_id.edit" not just "edit"
	parts := strings.Split(strings.Trim(pattern, "/"), "/")

	if len(parts) <= 1 {
		return "index"
	}

	// Skip the domain (first part), build action from remaining parts
	actionParts := []string{}
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if strings.HasPrefix(part, ":") {
			// Convert :user_id to {user_id}
			paramName := strings.TrimPrefix(part, ":")
			actionParts = append(actionParts, "{"+paramName+"}")
		} else if part != "" {
			actionParts = append(actionParts, part)
		}
	}

	if len(actionParts) > 0 {
		return strings.Join(actionParts, ".")
	}

	// Fallback to method-based action
	switch method {
	case "GET":
		return "show"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "index"
	}
}

func handleHTMLRouteWithProcessManager(w http.ResponseWriter, r *http.Request, group RouteGroup, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) {
	log.Printf("Processing route: %s %s", group.Method, group.Pattern)

	// Parse HTMX headers
	htmxReq := parseHTMXHeaders(r)

	requestData := extractRequestData(r, *group.HTMLRoute)

	// Add HTMX context to request data
	requestData["htmx"] = map[string]any{
		"is_htmx":     htmxReq.IsHTMX,
		"trigger":     htmxReq.Trigger,
		"target":      htmxReq.Target,
		"current_url": htmxReq.CurrentURL,
		"boosted":     htmxReq.Boosted,
	}

	var templateData any = requestData

	// Step 1: Execute SQL if exists
	if group.SQLRoute != nil {
		log.Printf("Executing SQL template: %s", group.SQLRoute.View)
		sqlData, err := executeSQL(group.SQLRoute, requestData, appConfig, frameworkServer)
		if err != nil {
			log.Printf("SQL execution failed: %v", err)
		} else {
			templateData = sqlData
			log.Printf("SQL data retrieved successfully")
		}
	}

	// Step 2: Execute JavaScript handler if available
	if frameworkServer.ProcessManager != nil && frameworkServer.ProcessManager.IsHandlerServiceRunning() {
		domain := group.Domain
		action := extractActionFromRoute(group.Pattern, group.Method)
		log.Printf("Executing handler: %s.%s", domain, action)

		processedData, err := frameworkServer.ProcessManager.ExecuteHandler(domain, action, templateData, requestData)

		if err != nil {
			log.Printf("Handler execution failed: %v", err)
		} else {
			templateData = processedData
			log.Printf("Handler processing completed successfully")
		}
	} else {
		log.Printf("Handler service not available, skipping handler execution")
	}

	// Step 3: Determine template path with HTMX override support
	templatePath := group.HTMLRoute.ViewPath

	// Check for HTMX-specific template override
	if htmxReq.IsHTMX {
		htmxTemplatePath := strings.Replace(templatePath, ".html.hbs", ".htmx.hbs", 1)
		if _, err := os.Stat(htmxTemplatePath); err == nil {
			templatePath = htmxTemplatePath
			log.Printf("🎯 Using HTMX-specific template: %s", templatePath)
		} else {
			log.Printf("🎯 Using regular template for HTMX (no layout): %s", templatePath)
		}
	}

	// Step 4: Wrap final data in vm key before rendering
	viewModel := map[string]any{
		"vm": map[string]any{
			group.Domain: templateData,
			"domain":     group.Domain,
			"group":      group,
			"htmx":       htmxReq,
		},
	}

	// Step 5: Render template with HTMX-aware logic
	html, err := loadAndRenderHTMXTemplate(templatePath, viewModel, appConfig.Views, htmxReq.IsHTMX)
	if err != nil {
		log.Printf("Template render failed: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Step 6: Handle HTMX response headers
	htmxHeaders := extractHTMXHeaders(templateData)
	setHTMXResponseHeaders(w, htmxHeaders)

	// Step 7: Handle redirects for successful form submissions (non-HTMX only)
	if (r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH") && !htmxReq.IsHTMX {
		if dataArray, ok := templateData.([]map[string]any); ok && len(dataArray) > 0 {
			if id, exists := dataArray[0]["id"]; exists {
				redirectURL := buildShowURL(group.Pattern, id)
				log.Printf("🔀 Redirecting to: %s", redirectURL)
				http.Redirect(w, r, redirectURL, http.StatusSeeOther)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// loadAndRenderHTMXTemplate renders templates with HTMX-specific logic
func loadAndRenderHTMXTemplate(templatePath string, data any, renderer *views.TemplateRenderer, isHTMXRequest bool) (string, error) {
	pathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(templatePath)))
	templateName := fmt.Sprintf("route_%s", pathHash[:16])

	content, err := renderer.Render(templateName, data)
	if err != nil {
		// Fallback: load template dynamically
		log.Printf("⚠️ Template %s not preloaded, loading dynamically: %s", templateName, templatePath)

		tempName := fmt.Sprintf("temp_%d", time.Now().UnixNano())
		if loadErr := renderer.LoadTemplate(tempName, templatePath); loadErr != nil {
			return "", fmt.Errorf("failed to load template: %w", loadErr)
		}

		content, err = renderer.Render(tempName, data)
		if err != nil {
			return "", fmt.Errorf("failed to render template: %w", err)
		}
	}

	contentTrimmed := strings.TrimSpace(content)
	isCompleteDocument := strings.HasPrefix(strings.ToLower(contentTrimmed), "<!doctype html") ||
		strings.HasPrefix(strings.ToLower(contentTrimmed), "<html")

	if isHTMXRequest {
		// For HTMX requests, always return content without layout
		if isCompleteDocument {
			log.Printf("⚠️ HTMX request received full document, extracting body content")
			return extractBodyContent(content), nil
		} else {
			log.Printf("📦 Returning HTMX fragment (no layout)")
			return content, nil
		}
	} else if isCompleteDocument {
		// Return full document for regular requests
		log.Printf("📄 Returning complete document")
		return content, nil
	} else {
		// Wrap in layout for regular requests
		log.Printf("📄 Wrapping content in layout")
		return wrapInLayout(content, data, renderer)
	}
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

// buildShowURL constructs the show URL based on the create pattern
func buildShowURL(createPattern string, id any) string {
	// Convert /users/create to /users/:user_id pattern
	if strings.Contains(createPattern, "/create") {
		basePattern := strings.Replace(createPattern, "/create", "", 1)
		return fmt.Sprintf("%s/%v", basePattern, id)
	}

	// Fallback for other patterns
	return fmt.Sprintf("/users/%v", id)
}

// executeSQL renders the SQL template and executes it against the database
func executeSQL(sqlRoute *parser.Route, requestData map[string]any, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) (any, error) {
	// Load and render the SQL template to generate the actual SQL query
	sqlQuery, err := loadAndRenderSQLTemplate(sqlRoute.ViewPath, requestData, appConfig.Views)
	if err != nil {
		return nil, fmt.Errorf("failed to render SQL template: %w", err)
	}

	log.Printf("🔍 Generated SQL query: %s", sqlQuery)

	// Execute the SQL query using the database executor
	if frameworkServer != nil && frameworkServer.DbExecutor != nil {
		// Use the real database executor
		ctx := context.Background()
		resultJSON, err := frameworkServer.DbExecutor.ExecuteSQL(ctx, sqlQuery, requestData, nil)
		if err != nil {
			log.Printf("❌ Database execution failed: %v", err)
			return nil, fmt.Errorf("database execution failed: %w", err)
		}

		log.Printf("🔍 Raw database response: %s", string(resultJSON))

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
		log.Printf("📦 Database response data: %+v", dbResponse.Data)

		// For INSERT/UPDATE/DELETE with RETURNING, the data should be in dbResponse.Data
		// Return the data array directly as the main template data
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
	// Create the expected template name based on path hash
	pathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(templatePath)))
	templateName := fmt.Sprintf("route_%s", pathHash[:16])

	// Try to render with the preloaded template name
	sql, err := renderer.Render(templateName, data)
	if err != nil {
		// Fallback: load the template dynamically for development
		log.Printf("⚠️ SQL template %s not preloaded, loading dynamically: %s", templateName, templatePath)

		// Create a temporary name and load it
		tempName := fmt.Sprintf("sql_temp_%d", time.Now().UnixNano())

		if loadErr := renderer.LoadTemplate(tempName, templatePath); loadErr != nil {
			return "", fmt.Errorf("failed to load SQL template: %w", loadErr)
		}

		sql, err = renderer.Render(tempName, data)
		if err != nil {
			return "", fmt.Errorf("failed to render SQL template: %w", err)
		}

		// Note: We can't delete the temp template, but this should only happen in development
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
func handleSingleRoute(w http.ResponseWriter, r *http.Request, route parser.Route, domainName string, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) {
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

// determineRequestedFormat determines the desired response format from request
func determineRequestedFormat(r *http.Request) string {
	// Check query parameter first (?format=json)
	if format := r.URL.Query().Get("format"); format != "" {
		log.Printf("🔍 Format from query param: %s", format)
		return format
	}

	// Check Accept header
	accept := r.Header.Get("Accept")
	log.Printf("🔍 Accept header: %s", accept)

	if strings.Contains(accept, "application/json") {
		return "json"
	}
	if strings.Contains(accept, "text/html") {
		return "html"
	}

	// Default to html
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
func createMultiFormatHandler(routes []parser.Route, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine desired format from Accept header or query parameter
		desiredFormat := determineRequestedFormat(r)

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
func handleRouteByFormat(w http.ResponseWriter, r *http.Request, route parser.Route, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) {
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
func handleHTMLRoute(w http.ResponseWriter, r *http.Request, route parser.Route, requestData map[string]any, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) {
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
	// Create the expected template name based on path hash
	pathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(templatePath)))
	templateName := fmt.Sprintf("route_%s", pathHash[:16])

	// Try to render with the preloaded template name
	content, err := renderer.Render(templateName, data)
	if err != nil {
		// Fallback: load the template dynamically for development
		log.Printf("⚠️ Template %s not preloaded, loading dynamically: %s", templateName, templatePath)

		// Create a temporary name and load it
		tempName := fmt.Sprintf("temp_%d", time.Now().UnixNano())

		if loadErr := renderer.LoadTemplate(tempName, templatePath); loadErr != nil {
			return "", fmt.Errorf("failed to load template: %w", loadErr)
		}

		content, err = renderer.Render(tempName, data)
		if err != nil {
			return "", fmt.Errorf("failed to render template: %w", err)
		}

		// Note: We can't delete the temp template since DeleteTemplate doesn't exist
		// But this should only happen in development when templates aren't preloaded
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
func handleJSONRoute(w http.ResponseWriter, r *http.Request, route parser.Route, requestData map[string]any, appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) {
	log.Printf("🔗 Processing JSON route: %s", route.View)

	var responseData any

	// Look for a corresponding SQL route with the same pattern and method
	var sqlRoute *parser.Route
	for _, domain := range appConfig.Domains {
		for _, domainRoute := range domain.Logic.HTTP.Routes {
			if domainRoute.Method == route.Method &&
				domainRoute.Link == route.Link &&
				domainRoute.Format == "sql" {
				sqlRoute = &domainRoute
				break
			}
		}
		if sqlRoute != nil {
			break
		}
	}

	// If we found a SQL route, execute it to get data
	if sqlRoute != nil {
		log.Printf("🗄️ Found SQL route for JSON: %s", sqlRoute.View)

		sqlData, err := executeSQL(sqlRoute, requestData, appConfig, frameworkServer)
		if err != nil {
			log.Printf("❌ SQL execution failed for JSON route: %v", err)
			responseData = map[string]any{
				"success": false,
				"error":   fmt.Sprintf("Database error: %v", err),
			}
		} else {
			log.Printf("✅ SQL data retrieved for JSON: %+v", sqlData)
			// Return the SQL data directly, or wrap it in a success response
			if dataArray, ok := sqlData.([]map[string]any); ok {
				responseData = map[string]any{
					"success": true,
					"data":    dataArray,
					"count":   len(dataArray),
				}
			} else {
				responseData = map[string]any{
					"success": true,
					"data":    sqlData,
				}
			}
		}
	} else {
		// No SQL route found, fall back to domain logic or request data
		log.Printf("⚠️ No SQL route found for JSON route, using fallback")

		if frameworkServer != nil {
			domainData, err := callDomainLogic(r, route, requestData, frameworkServer)
			if err != nil {
				responseData = map[string]any{
					"success": false,
					"error":   err.Error(),
				}
			} else if domainData != nil {
				responseData = domainData
			} else {
				responseData = map[string]any{
					"success": true,
					"data":    requestData,
				}
			}
		} else {
			responseData = map[string]any{
				"success": true,
				"data":    requestData,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responseData); err != nil {
		log.Printf("❌ Failed to encode JSON response: %v", err)
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ JSON response sent successfully")
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
func callDomainLogic(r *http.Request, route parser.Route, requestData map[string]any, frameworkServer *lang_adapters.FrameworkServer) (map[string]any, error) {
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

// extractRequestData extracts all relevant data from the HTTP request with HTMX support
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

	// Add HTMX-specific data
	htmxReq := parseHTMXHeaders(r)
	data["_htmx"] = htmxReq
	data["_is_htmx"] = htmxReq.IsHTMX

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
func StartHTTPServerWithConfig(appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) *http.Server {
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
	fmt.Printf("   GET /health -> Health check\n")
	fmt.Printf("   GET /htmx.min.js -> HTMX library\n")
	fmt.Println()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}

// StartGRPCServerWithShutdown starts gRPC server and returns server instance for shutdown control
func StartGRPCServerWithShutdown(frameworkServer *lang_adapters.FrameworkServer) *grpc.Server {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on port 50051: %v", err)
	}

	server := grpc.NewServer()
	reflection.Register(server)
	lang_adapters.RegisterFrameworkServiceServer(server, frameworkServer)

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
	frameworkServer := &lang_adapters.FrameworkServer{
		Db:              db,
		DbExecutor:      database.NewDatabaseExecutor(db),
		DomainStreams:   make(map[string]lang_adapters.FrameworkService_DomainCommunicationServer),
		PendingRequests: make(map[string]*lang_adapters.PendingRequest),
	}
	frameworkServer.StartCleanupRoutine()

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

	log.Println("Pre-loading route templates...")
	if err := appConfig.PreloadRouteTemplates(); err != nil {
		log.Printf("Warning: failed to preload route templates: %v", err)
	} else {
		log.Println("✅ Route templates preloaded successfully")
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

// StartHTTPServerWithProcessManager starts HTTP server with HTMX and process manager support
func StartHTTPServerWithProcessManager(appConfig *parser.AppConfig, frameworkServer *lang_adapters.FrameworkServer) *http.Server {
	mux := CreateRouteDispatcher(appConfig, frameworkServer)
	auth.AddLoginRoute(mux, frameworkServer)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Printf("🚀 HTTP Server with HTMX support starting on http://localhost%s\n", server.Addr)
	fmt.Println("📍 Registered routes:")

	// Log routes with HTMX support indication
	routeGroups := make(map[string][]string)
	for _, domain := range appConfig.Domains {
		for _, route := range domain.Logic.HTTP.Routes {
			key := fmt.Sprintf("%s %s", route.Method, route.Link)
			routeGroups[key] = append(routeGroups[key], route.Format)
		}
	}

	for pattern, formats := range routeGroups {
		fmt.Printf("   %s (formats: %s, HTMX: ✓)\n", pattern, strings.Join(formats, ", "))
	}
	fmt.Println("   GET /health -> Health check")
	fmt.Println("   GET /htmx.min.js -> HTMX library")
	fmt.Println()
	fmt.Println("🔄 HTMX Features Enabled:")
	fmt.Println("   - Automatic fragment detection")
	fmt.Println("   - HTMX-specific templates (.htmx.hbs)")
	fmt.Println("   - Regular templates without layout for HTMX")
	fmt.Println("   - Response header management")
	fmt.Println("   - Context injection for handlers")
	fmt.Println()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}

// Add this function to framework_integration.go
func StartBothServersWithProcessManager(appConfig *parser.AppConfig) {
	// Database setup (your existing code)
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

	// Framework Server Setup with Process Manager
	frameworkServer := &lang_adapters.FrameworkServer{
		Db:              db,
		DbExecutor:      database.NewDatabaseExecutor(db),
		DomainStreams:   make(map[string]lang_adapters.FrameworkService_DomainCommunicationServer),
		PendingRequests: make(map[string]*lang_adapters.PendingRequest),
	}
	frameworkServer.StartCleanupRoutine()

	// Initialize Process Manager for JavaScript handlers
	if err := frameworkServer.InitializeProcessManager(appConfig.Path, true); err != nil {
		log.Printf("Warning: Failed to initialize process manager: %v", err)
	}

	// Template setup (your existing code)
	renderer, err := views.SetupViewsFromConfig(appConfig)
	if err != nil {
		log.Fatalf("Failed to setup views: %v", err)
	}
	appConfig.Views = renderer

	// Validate and preload templates
	if err := appConfig.ValidateRoutes(); err != nil {
		log.Printf("Warning: Route validation issues found: %v", err)
	}

	if err := appConfig.PreloadRouteTemplates(); err != nil {
		log.Printf("Warning: failed to preload route templates: %v", err)
	}

	// Start servers with process manager integration
	grpcServer := StartGRPCServerWithShutdown(frameworkServer)
	httpServer := StartHTTPServerWithProcessManager(appConfig, frameworkServer)

	// Graceful shutdown
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

	// Stop process manager
	if frameworkServer.ProcessManager != nil {
		if err := frameworkServer.ProcessManager.StopAll(); err != nil {
			log.Printf("Process manager shutdown error: %v", err)
		}
	}

	log.Println("Servers gracefully stopped.")
}

// Legacy functions for backward compatibility

// StartGRPCServer starts the gRPC server with the given FrameworkServer (legacy)
func StartGRPCServer(frameworkServer *lang_adapters.FrameworkServer) {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on port 50051: %v", err)
	}

	// Create gRPC server
	server := grpc.NewServer()
	reflection.Register(server)

	// Register the framework service
	lang_adapters.RegisterFrameworkServiceServer(server, frameworkServer)

	log.Println("gRPC server starting on :50051")

	// Start serving (this blocks)
	if err := server.Serve(listener); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}

// StartHTTPServerWithShutdown starts HTTP server and returns server instance for shutdown control (legacy)
func StartHTTPServerWithShutdown(frameworkServer *lang_adapters.FrameworkServer) *http.Server {
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

		domainMsg := &lang_adapters.DomainMessage{
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
