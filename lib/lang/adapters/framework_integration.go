package lang_adapters

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	reflect "reflect"

	parser "fulcrum/lib/parser"

	"google.golang.org/protobuf/types/known/structpb"
)

// Add ProcessManager to your existing FrameworkServer
func (fs *FrameworkServer) InitializeProcessManager(appRoot string, verbose bool) error {
	fs.processManager = NewProcessManager(appRoot, verbose)

	// Auto-detect handler configuration
	config := fs.processManager.AutoDetectHandlerConfig()

	log.Printf("Initializing handler service with config: %+v", config)

	// Check if we should start the handler service
	if fs.shouldStartHandlerService(config.HandlersPath) {
		if err := fs.processManager.StartHandlerService(config); err != nil {
			log.Printf("Warning: Failed to start handler service: %v", err)
			log.Printf("Handlers will not be available")
			return nil // Don't fail completely, just warn
		}

		log.Printf("Handler service initialized successfully")
	} else {
		log.Printf("No handlers found, skipping handler service startup")
	}

	return nil
}

// shouldStartHandlerService checks if we should start the handler service
func (fs *FrameworkServer) shouldStartHandlerService(handlersPath string) bool {
	// Check if handlers directory exists and has handler.js files
	if _, err := os.Stat(handlersPath); os.IsNotExist(err) {
		return false
	}

	// Walk the directory to see if there are any handler.js files
	hasHandlers := false
	filepath.Walk(handlersPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.Name() == "handler.js" {
			hasHandlers = true
			return filepath.SkipDir // Found one, no need to continue
		}

		return nil
	})

	return hasHandlers
}

// Check for redirect in processed data
func checkForRedirect(data interface{}) *struct {
	URL    string
	Status int
} {
	if dataMap, ok := data.(map[string]interface{}); ok {
		if redirectInfo, hasRedirect := dataMap["_redirect"]; hasRedirect {
			if redirect, ok := redirectInfo.(map[string]interface{}); ok {
				if url, hasURL := redirect["url"].(string); hasURL {
					status := 303 // Default redirect status
					if redirectStatus, hasStatus := redirect["status"]; hasStatus {
						switch s := redirectStatus.(type) {
						case int:
							status = s
						case float64:
							status = int(s)
						case string:
							// Try to parse string to int
							if parsed := parseStatusCode(s); parsed > 0 {
								status = parsed
							}
						}
					}
					return &struct {
						URL    string
						Status int
					}{url, status}
				}
			}
		}
	}
	return nil
}

// Parse status code from string
func parseStatusCode(s string) int {
	switch s {
	case "301":
		return 301
	case "302":
		return 302
	case "303":
		return 303
	case "307":
		return 307
	case "308":
		return 308
	default:
		return 0
	}
}

// Render error page with helpful information
func renderErrorPage(w http.ResponseWriter, err error, route *parser.Route, data interface{}) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)

	fmt.Fprintf(w, `
		<html>
		<head>
			<title>Template Error</title>
			<style>
				body { font-family: Arial, sans-serif; margin: 40px; }
				.error { color: #d32f2f; }
				.info { background: #f5f5f5; padding: 15px; border-radius: 4px; }
				pre { background: #f0f0f0; padding: 10px; overflow: auto; }
			</style>
		</head>
		<body>
			<h1 class="error">Template Rendering Failed</h1>
			<div class="info">
				<p><strong>Error:</strong> %s</p>
				<p><strong>Template:</strong> %s</p>
				<p><strong>Template Path:</strong> %s</p>
				<p><strong>Data Type:</strong> %T</p>
			</div>
			<h3>Template Data:</h3>
			<pre>%+v</pre>
		</body>
		</html>
	`, err.Error(), route.View, route.ViewPath, data, data)
}

// convertToProtobufStruct converts any Go value to a protobuf Struct with comprehensive logging
func convertToProtobufStruct(data any) (*structpb.Struct, error) {
	// Handle nil case early
	if data == nil {
		println("Converting nil data to empty protobuf struct")
		return &structpb.Struct{Fields: make(map[string]*structpb.Value)}, nil
	}

	// Log input data details
	dataType := reflect.TypeOf(data)
	dataValue := reflect.ValueOf(data)
	println(fmt.Sprintf("Converting data to protobuf struct - type: %s, kind: %s, isNil: %t",
		dataType.String(),
		dataValue.Kind().String(),
		!dataValue.IsValid() || (dataValue.Kind() == reflect.Ptr && dataValue.IsNil()),
	))

	// Log the actual data for debugging
	println(fmt.Sprintf("Input data content: %+v", data))

	// Convert to protobuf-compatible structure
	converted, err := normalizeForProtobuf(data)
	if err != nil {
		println(fmt.Sprintf("Failed to normalize data for protobuf: %v", err))
		return nil, fmt.Errorf("failed to normalize data: %w", err)
	}

	println(fmt.Sprintf("Normalized data type: %T", converted))

	// Create protobuf struct
	pbStruct, err := structpb.NewStruct(converted)
	if err != nil {
		println(fmt.Sprintf("Failed to create protobuf struct: %v, data: %+v", err, converted))
		return nil, fmt.Errorf("failed to create protobuf struct: %w", err)
	}

	println(fmt.Sprintf("Successfully converted to protobuf struct - field_count: %d, fields: %v",
		len(pbStruct.Fields),
		getFieldNames(pbStruct),
	))

	return pbStruct, nil
}

// normalizeForProtobuf converts data to a map[string]interface{} structure that protobuf can handle
func normalizeForProtobuf(data any) (map[string]interface{}, error) {
	switch v := data.(type) {
	case map[string]any:
		println("Converting map[string]any to map[string]interface{}")
		result := make(map[string]interface{}, len(v))
		for k, val := range v {
			result[k] = val
		}
		return result, nil

	case []map[string]any:
		println(fmt.Sprintf("Converting []map[string]any to wrapped structure, slice_length: %d", len(v)))
		items := make([]interface{}, len(v))
		for i, item := range v {
			converted := make(map[string]interface{}, len(item))
			for k, val := range item {
				converted[k] = val
			}
			items[i] = converted
		}
		return map[string]any{"data": items}, nil

	case []any:
		println(fmt.Sprintf("Converting []any to wrapped structure, slice_length: %d", len(v)))
		items := make([]any, len(v))
		copy(items, v)
		return map[string]any{"data": items}, nil

	default:
		// Handle structs by converting to map via reflection
		if reflect.TypeOf(v).Kind() == reflect.Struct {
			println("Converting struct to map via reflection")
			structMap, err := structToMap(v)
			if err != nil {
				return nil, fmt.Errorf("failed to convert struct to map: %w", err)
			}
			return structMap, nil
		}

		println(fmt.Sprintf("Wrapping primitive value, value_type: %T", v))
		return map[string]any{"value": v}, nil
	}
}

// structToMap converts a struct to map[string]interface{} using reflection
func structToMap(s any) (map[string]any, error) {
	result := make(map[string]any)
	v := reflect.ValueOf(s)
	t := reflect.TypeOf(s)

	// Handle pointer to struct
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return result, nil
		}
		v = v.Elem()
		t = t.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", v.Kind())
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		// Use json tag if available, otherwise use field name
		fieldName := fieldType.Name
		if jsonTag := fieldType.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
			if commaIndex := len(jsonTag); commaIndex > 0 {
				if commaPos := findComma(jsonTag); commaPos != -1 {
					fieldName = jsonTag[:commaPos]
				} else {
					fieldName = jsonTag
				}
			}
		}

		result[fieldName] = field.Interface()
	}

	return result, nil
}

// Helper function to find comma in string
func findComma(s string) int {
	for i, r := range s {
		if r == ',' {
			return i
		}
	}
	return -1
}

// convertFromProtobufStruct converts a protobuf Struct back to Go data with logging
func convertFromProtobufStruct(pbStruct *structpb.Struct) any {
	if pbStruct == nil {
		println("Converting nil protobuf struct to nil")
		return nil
	}

	println(fmt.Sprintf("Converting protobuf struct to Go data - field_count: %d, fields: %v",
		len(pbStruct.Fields),
		getFieldNames(pbStruct),
	))

	result := pbStruct.AsMap()
	println(fmt.Sprintf("Converted protobuf struct, result_type: %T", result))

	return result
}

// getFieldNames extracts field names from protobuf struct for logging
func getFieldNames(pbStruct *structpb.Struct) []string {
	if pbStruct == nil || pbStruct.Fields == nil {
		return nil
	}

	names := make([]string, 0, len(pbStruct.Fields))
	for name := range pbStruct.Fields {
		names = append(names, name)
	}
	return names
}

// Update your existing FrameworkServer struct to include ProcessManager
// Add this field to your existing FrameworkServer:
// processManager *ProcessManager

// Create HTTP server with process manager integration
func StartHTTPServerWithProcessManager(appConfig *parser.AppConfig, frameworkServer *FrameworkServer) *http.Server {
	// Use your existing CreateRouteDispatcher but replace the handler function
	mux := CreateRouteDispatcherWithProcessManager(appConfig, frameworkServer)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Your existing logging code...

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}

// Modified version of your CreateRouteDispatcher
func CreateRouteDispatcherWithProcessManager(appConfig *parser.AppConfig, frameworkServer *FrameworkServer) *http.ServeMux {
	// Use your existing CreateRouteDispatcher code but replace:
	// handleHTMLRouteWithSQL with handleHTMLRouteWithProcessManager
	//
	// This is a conceptual function - you'd modify your existing CreateRouteDispatcher
	// to use handleHTMLRouteWithProcessManager instead of handleHTMLRouteWithSQL

	return CreateRouteDispatcher(appConfig, frameworkServer) // Your existing function
}
