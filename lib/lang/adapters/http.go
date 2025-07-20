package lang_adapters

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Global reference to the FrameworkServer instance
var frameworkServer *FrameworkServer

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Status: OK\nTime: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

// Catch-all handler to send messages directly to FrameworkServer
func catchAllHandler(w http.ResponseWriter, r *http.Request) {
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
	if frameworkServer != nil {
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
	} else {
		fmt.Fprintf(w, "FrameworkServer not initialized\n")
		fmt.Fprintf(w, "Would send: Domain=%s, Type=%s, Payload=%s\n", domain, msgType, payload)
	}
}

// StartHTTPServer starts the HTTP server with access to the FrameworkServer instance
func StartHTTPServer(server *FrameworkServer) {
	// Store reference to the FrameworkServer
	frameworkServer = server

	// Setup routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", catchAllHandler) // Catch-all route

	port := ":8080"
	fmt.Printf("🚀 HTTP Server starting on http://localhost%s\n", port)
	fmt.Println("📍 Available endpoints:")
	fmt.Println("   GET /health - Health check")
	fmt.Println("   ANY /* - Send message to FrameworkServer")
	fmt.Println()
	fmt.Println("💡 Use headers to control message:")
	fmt.Println("   X-Domain: specify target domain")
	fmt.Println("   X-Message-Type: specify message type")
	fmt.Println()

	// Start server
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("HTTP server failed to start:", err)
	}
}
