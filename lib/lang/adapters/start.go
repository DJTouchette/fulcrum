package lang_adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"fulcrum/lib/database"
	"fulcrum/lib/database/interfaces"
	parser "fulcrum/lib/parser"
	"fulcrum/lib/views"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// RouteHandler holds route information and the associated domain
type RouteHandler struct {
	Method string
	Path   string
	Link   string
	Domain string
}

func CreateRouteDispatcher(appConfig *parser.AppConfig, frameworkServer *FrameworkServer) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check handler
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Status: OK\nTime: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	})

	// Create handlers for each route in each domain
	for _, domain := range appConfig.Domains {
		if domain.Logic.HTTP.Restful {
			for _, route := range domain.Logic.HTTP.Routes {
				// Build the full route path
				routePath := fmt.Sprintf("/%s%s", domain.Name, route.Path)

				// Create handler for this specific route
				handler := createRouteHandler(route, domain.Name, frameworkServer, appConfig)

				log.Printf("Registering route: %s %s -> domain: %s, link: %s",
					route.Method, routePath, domain.Name, route.Link)

				pattern := fmt.Sprintf("%s %s", route.Method, routePath)
				mux.HandleFunc(pattern, handler)
			}
		}
	}

	// Catch-all for unmatched routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "No route found for %s %s\n", r.Method, r.URL.Path)
		fmt.Fprintf(w, "Available routes:\n")
		for _, domain := range appConfig.Domains {
			if domain.Logic.HTTP.Restful {
				for _, route := range domain.Logic.HTTP.Routes {
					routePath := fmt.Sprintf("/%s%s", domain.Name, route.Path)
					fmt.Fprintf(w, "  %s %s (domain: %s)\n", route.Method, routePath, domain.Name)
				}
			}
		}
	})

	return mux
}

func createRouteHandler(route parser.Route, domainName string, frameworkServer *FrameworkServer, appConfig *parser.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("🔍 Handler called for: %s %s (route: %s, domain: %s)",
			r.Method, r.URL.Path, route.Path, domainName)
		// Check if method matches
		if r.Method != route.Method {
			http.Error(w, fmt.Sprintf("Method %s not allowed for this route", r.Method), http.StatusMethodNotAllowed)
			return
		}

		// If no link is specified, just render the view directly
		if route.Link == "" {
			if route.View != "" && route.ViewPath != "" {
				// Extract any data for template context (query params, path params, form data)
				templateData := extractRequestDataForTemplate(r, route.Path, domainName)

				html, err := appConfig.Views.RenderWithLayout(
					"shared/views/layouts/main",
					route.ViewPath,
					templateData,
				)
				if err != nil {
					log.Printf("Error rendering template %s: %v", route.ViewPath, err)
					http.Error(w, "Failed to render template", http.StatusInternalServerError)
					return
				}
				// Write HTML response
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(html))
				return
			} else {
				// No link and no view - just return success
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"success": true, "message": "Route processed"}`)
				return
			}
		}

		// Extract path parameters from URL
		pathParams := extractPathParameters(r.URL.Path, route.Path, domainName)

		// Extract request body for POST/PUT requests
		var payload string
		if r.Method == "POST" || r.Method == "PUT" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}

			// Parse existing payload and add path params
			var payloadData map[string]any
			if len(body) > 0 {
				if err := json.Unmarshal(body, &payloadData); err != nil {
					// If not JSON, treat as form data
					payloadData = make(map[string]any)
					if err := r.ParseForm(); err == nil {
						for k, v := range r.Form {
							if len(v) == 1 {
								payloadData[k] = v[0]
							} else {
								payloadData[k] = v
							}
						}
					}
				}
			} else {
				// Parse form data for POST requests
				if err := r.ParseForm(); err == nil {
					payloadData = make(map[string]any)
					for k, v := range r.Form {
						if len(v) == 1 {
							payloadData[k] = v[0]
						} else {
							payloadData[k] = v
						}
					}
				} else {
					payloadData = make(map[string]any)
				}
			}

			// Add path parameters to payload
			for k, v := range pathParams {
				payloadData[k] = v
			}

			payloadBytes, _ := json.Marshal(payloadData)
			payload = string(payloadBytes)
		} else {
			// For GET/DELETE, include query parameters and path parameters
			data := make(map[string]any)

			// Add query parameters
			for k, v := range r.URL.Query() {
				if len(v) == 1 {
					data[k] = v[0]
				} else {
					data[k] = v
				}
			}

			// Add path parameters
			for k, v := range pathParams {
				data[k] = v
			}

			payloadBytes, _ := json.Marshal(data)
			payload = string(payloadBytes)
		}

		// Create domain message
		domainMsg := &DomainMessage{
			Domain:    domainName,
			Type:      route.Link,
			Payload:   payload,
			RequestId: fmt.Sprintf("http-%d", time.Now().UnixNano()),
		}

		// Send to framework server
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		response, err := frameworkServer.SendMessage(ctx, domainMsg)
		if err != nil {
			log.Printf("Error processing message for route %s: %v", route.Link, err)
			http.Error(w, "Failed to process request", http.StatusInternalServerError)
			return
		}

		fmt.Printf("📤 Response received:\n")
		fmt.Printf("   Type: %T\n", response)
		fmt.Printf("   Value: %+v\n", response)
		fmt.Printf("   JSON: %s\n", func() string {
			if jsonBytes, err := json.MarshalIndent(response, "", "  "); err == nil {
				return string(jsonBytes)
			}
			return "Failed to marshal to JSON"
		}())
		fmt.Println("========================================")

		if route.View != "" && route.ViewPath != "" {
			var templateData any
			if response.Success && response.Payload != "" {
				if err := json.Unmarshal([]byte(response.Payload), &templateData); err != nil {
					templateData = map[string]any{
						"data": response.Payload,
					}
				}
			} else {
				templateData = map[string]any{
					"success": response.Success,
					"error":   response.Error,
				}
			}
			html, err := appConfig.Views.RenderWithLayout(
				"shared/views/layouts/main",
				route.ViewPath,
				templateData,
			)
			if err != nil {
				log.Printf("Error rendering template %s: %v", route.ViewPath, err)
				http.Error(w, "Failed to render template", http.StatusInternalServerError)
				return
			}
			// Write HTML response
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(html))
			return
		}

		// Default JSON response (when no view is specified)
		w.Header().Set("Content-Type", "application/json")
		if response.Success {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"success": true,
				"type": "%s",
				"data": %s,
				"request_id": "%s"
				}`, response.Type, response.Payload, response.RequestId)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{
				"success": false,
				"error": "%s",
				"request_id": "%s"
				}`, response.Error, response.RequestId)
		}
	}
}

// Helper function to extract request data for template context when no domain logic is needed
func extractRequestDataForTemplate(r *http.Request, routePath, domainName string) map[string]any {
	data := make(map[string]any)

	// Extract path parameters
	pathParams := extractPathParameters(r.URL.Path, routePath, domainName)
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
	if r.Method == "POST" || r.Method == "PUT" {
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

	return data
}

// Helper function to extract path parameters
func extractPathParameters(actualPath, routePath, domainName string) map[string]string {
	params := make(map[string]string)

	// Remove domain prefix from actual path
	domainPrefix := "/" + domainName
	if strings.HasPrefix(actualPath, domainPrefix) {
		actualPath = strings.TrimPrefix(actualPath, domainPrefix)
	}
	if actualPath == "" {
		actualPath = "/"
	}

	// Handle root path case
	if routePath == "/" {
		return params
	}

	// Split paths into segments
	actualSegments := strings.Split(strings.Trim(actualPath, "/"), "/")
	routeSegments := strings.Split(strings.Trim(routePath, "/"), "/")

	// Match segments and extract parameters
	for i, routeSegment := range routeSegments {
		if i >= len(actualSegments) {
			break
		}

		if strings.HasPrefix(routeSegment, "{") && strings.HasSuffix(routeSegment, "}") {
			// This is a parameter
			paramName := strings.Trim(routeSegment, "{}")
			params[paramName] = actualSegments[i]
		}
	}

	return params
}

// StartHTTPServerWithConfig starts HTTP server using the parsed configuration
func StartHTTPServerWithConfig(appConfig *parser.AppConfig, frameworkServer *FrameworkServer) *http.Server {
	// Create the route dispatcher
	mux := CreateRouteDispatcher(appConfig, frameworkServer)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Printf("🚀 HTTP Server starting on http://localhost%s\n", server.Addr)
	fmt.Println("📍 Registered routes:")

	// Log all registered routes
	for _, domain := range appConfig.Domains {
		if domain.Logic.HTTP.Restful {
			for _, route := range domain.Logic.HTTP.Routes {
				fmt.Printf("   %s /%s -> domain: %s, link: %s\n",
					route.Method, route.Link, domain.Name, route.Link)
			}
		}
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

// StartGRPCServer starts the gRPC server with the given FrameworkServer
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

// StartHTTPServerWithShutdown starts HTTP server and returns server instance for shutdown control
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

// Example usage functions

// StartBothServersWithConfig starts both servers using parsed configuration
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
	// Create framework server with a database executor
	frameworkServer := &FrameworkServer{
		db:              db,
		dbExecutor:      database.NewDatabaseExecutor(db),
		domainStreams:   make(map[string]FrameworkService_DomainCommunicationServer),
		pendingRequests: make(map[string]*PendingRequest),
	}
	frameworkServer.startCleanupRoutine() // Start request cleanup

	// --- Renderer Setup ---
	renderer, err := views.SetupViews(appConfig.Path)
	if err != nil {
		log.Fatalf("Failed to setup views: %v", err)
	}
	appConfig.Views = renderer

	// --- Start Servers ---
	grpcServer := StartGRPCServerWithShutdown(frameworkServer)
	httpServer := StartHTTPServerWithConfig(appConfig, frameworkServer)

	// --- Graceful Shutdown ---
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
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
