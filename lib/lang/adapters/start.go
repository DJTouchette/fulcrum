package lang_adapters

import (
	"context"
	"encoding/json"
	"fmt"
	parser "fulcrum/lib/parser"
	"fulcrum/lib/views"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// CreateFrameworkServer creates and initializes a new FrameworkServer instance
func CreateFrameworkServer() *FrameworkServer {
	// Create and configure the framework server
	frameworkServer := &FrameworkServer{
		// Initialize your dependencies here if needed
		// messageBus: yourMessageBusInstance,
	}

	log.Println("FrameworkServer created and initialized")
	return frameworkServer
}

// RouteHandler holds route information and the associated domain
type RouteHandler struct {
	Method string
	Path   string
	Link   string
	Domain string
}

// CreateRouteDispatcher creates HTTP handlers from the parsed AppConfig
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
				// Create the route path
				routePath := fmt.Sprintf("/%s", domain.Name)

				// Create handler for this specific route
				handler := createRouteHandler(route, domain.Name, frameworkServer, appConfig)

				log.Printf("Registering route: %s %s -> domain: %s, link: %s",
					route.Method, routePath, domain.Name, route.Link)

				// Register the handler
				mux.HandleFunc(routePath, handler)
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
					fmt.Fprintf(w, "  %s /%s (domain: %s)\n", route.Method, domain.Name, domain.Name)
				}
			}
		}
	})

	return mux
}

// createRouteHandler creates a handler function for a specific route
func createRouteHandler(route parser.Route, domainName string, frameworkServer *FrameworkServer, appConfig *parser.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if method matches
		if r.Method != route.Method {
			http.Error(w, fmt.Sprintf("Method %s not allowed for this route", r.Method), http.StatusMethodNotAllowed)
			return
		}

		// Extract request body for POST/PUT requests
		var payload string
		if r.Method == "POST" || r.Method == "PUT" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}
			payload = string(body)
		} else {
			// For GET/DELETE, include query parameters
			payload = fmt.Sprintf(`{"query": "%s"}`, r.URL.RawQuery)
		}

		// Create domain message
		domainMsg := &DomainMessage{
			Domain:    domainName,
			Type:      route.Link, // Use the link as the message type
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
	frameworkServer := CreateFrameworkServer()

	// Start gRPC server in goroutine
	go StartGRPCServer(frameworkServer)

	// Start HTTP server with config-based routing (blocks)
	server := StartHTTPServerWithConfig(appConfig, frameworkServer)
	renderer, err := views.SetupViews(appConfig.Path)
	if err != nil {
		log.Fatalf("Failed to setup views: %v", err)
	}

	appConfig.Views = renderer

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}
