package lang_adapters

import (
	"bufio"
	"context"
	"fmt"
	"fulcrum/handler"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ProcessManager manages Node.js handler processes for the framework
type ProcessManager struct {
	processes     map[string]*ManagedProcess
	mutex         sync.RWMutex
	handlerClient handler.HandlerServiceClient
	handlerConn   *grpc.ClientConn
	isInitialized bool
	appRoot       string
	verbose       bool
}

// ManagedProcess represents a managed Node.js process
type ManagedProcess struct {
	Name      string
	Command   *exec.Cmd
	Port      int
	LogPrefix string
	isRunning bool
	stopChan  chan struct{}
	mutex     sync.RWMutex
}

// NewProcessManager creates a new process manager
func NewProcessManager(appRoot string, verbose bool) *ProcessManager {
	return &ProcessManager{
		processes:     make(map[string]*ManagedProcess),
		appRoot:       appRoot,
		verbose:       verbose,
		isInitialized: false,
	}
}

// StartHandlerService starts the FulcrumJS handler service for the application
func (pm *ProcessManager) StartHandlerService(config HandlerConfig) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if handler service is already running
	if _, exists := pm.processes["handlers"]; exists {
		return fmt.Errorf("handler service is already running")
	}

	log.Printf("Starting FulcrumJS handler service...")

	// Determine the command to run
	var cmd *exec.Cmd

	// Check if fulcrum-js CLI is available globally
	if pm.isFulcrumJSAvailable() {
		cmd = pm.createCLICommand(config)
	} else {
		// Fall back to running the example app's Node.js entry point
		cmd = pm.createAppCommand(config)
	}

	if cmd == nil {
		return fmt.Errorf("could not determine how to start handler service")
	}

	// Create managed process
	process := &ManagedProcess{
		Name:      "handlers",
		Command:   cmd,
		Port:      config.Port,
		LogPrefix: "[FulcrumJS]",
		stopChan:  make(chan struct{}),
	}

	// Set up logging
	if err := pm.setupProcessLogging(process); err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	// Start the process
	if err := process.Command.Start(); err != nil {
		return fmt.Errorf("failed to start handler service: %w", err)
	}

	process.isRunning = true
	pm.processes["handlers"] = process

	// Wait for the service to be ready
	if err := pm.waitForHandlerService(config.Port, 30*time.Second); err != nil {
		pm.stopProcess("handlers")
		return fmt.Errorf("handler service failed to start: %w", err)
	}

	// Connect gRPC client
	if err := pm.connectHandlerClient(config.Port); err != nil {
		pm.stopProcess("handlers")
		return fmt.Errorf("failed to connect to handler service: %w", err)
	}

	pm.isInitialized = true
	log.Printf("Handler service started successfully on port %d", config.Port)

	return nil
}

// isFulcrumJSAvailable checks if fulcrum-js CLI is available
func (pm *ProcessManager) isFulcrumJSAvailable() bool {
	_, err := exec.LookPath("fulcrum-js")
	return err == nil
}

// createCLICommand creates a command using the fulcrum-js CLI
func (pm *ProcessManager) createCLICommand(config HandlerConfig) *exec.Cmd {
	args := []string{"dev", "--port", fmt.Sprintf("%d", config.Port)}

	if config.HandlersPath != "" {
		args = append(args, "--handlers", config.HandlersPath)
	}

	if pm.verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command("fulcrum-js", args...)
	cmd.Dir = pm.appRoot

	return cmd
}

// createAppCommand creates a command using the example app's Node.js file
func (pm *ProcessManager) createAppCommand(config HandlerConfig) *exec.Cmd {
	// Look for package.json in the app root
	packageJsonPath := filepath.Join(pm.appRoot, "package.json")
	if _, err := os.Stat(packageJsonPath); os.IsNotExist(err) {
		return nil
	}

	// Check for index.js
	indexPath := filepath.Join(pm.appRoot, "index.js")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("node", "index.js")
	cmd.Dir = pm.appRoot

	// Set environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("HANDLER_PORT=%d", config.Port),
		fmt.Sprintf("HANDLERS_PATH=%s", config.HandlersPath),
	)

	if pm.verbose {
		cmd.Env = append(cmd.Env, "VERBOSE=true")
	}

	return cmd
}

// setupProcessLogging sets up stdout/stderr logging for a process
func (pm *ProcessManager) setupProcessLogging(process *ManagedProcess) error {
	stdout, err := process.Command.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := process.Command.StderrPipe()
	if err != nil {
		return err
	}

	// Log stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if pm.verbose {
				log.Printf("%s %s", process.LogPrefix, line)
			}
		}
	}()

	// Log stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("%s ERROR: %s", process.LogPrefix, line)
		}
	}()

	return nil
}

// waitForHandlerService waits for the handler service to be ready
func (pm *ProcessManager) waitForHandlerService(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to connect
		conn, err := grpc.Dial(
			fmt.Sprintf("localhost:%d", port),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(2*time.Second),
		)

		if err == nil {
			// Try a health check
			client := handler.NewHandlerServiceClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

			_, healthErr := client.Health(ctx, &handler.HealthRequest{})
			cancel()
			conn.Close()

			if healthErr == nil {
				return nil // Service is ready
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("handler service did not become ready within %v", timeout)
}

// connectHandlerClient establishes gRPC connection to handler service
func (pm *ProcessManager) connectHandlerClient(port int) error {
	address := fmt.Sprintf("localhost:%d", port)

	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to handler service: %w", err)
	}

	client := handler.NewHandlerServiceClient(conn)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Health(ctx, &handler.HealthRequest{})
	if err != nil {
		conn.Close()
		return fmt.Errorf("handler health check failed: %w", err)
	}

	pm.handlerConn = conn
	pm.handlerClient = client

	log.Printf("Connected to %s v%s", resp.ServiceName, resp.Version)
	return nil
}

// GetHandlerClient returns the gRPC client for handler service
func (pm *ProcessManager) GetHandlerClient() handler.HandlerServiceClient {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.handlerClient
}

// IsHandlerServiceRunning checks if the handler service is running
func (pm *ProcessManager) IsHandlerServiceRunning() bool {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	process, exists := pm.processes["handlers"]
	if !exists {
		return false
	}

	process.mutex.RLock()
	defer process.mutex.RUnlock()

	return process.isRunning
}

// stopProcess stops a managed process
func (pm *ProcessManager) stopProcess(name string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	process, exists := pm.processes[name]
	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	process.mutex.Lock()
	defer process.mutex.Unlock()

	if !process.isRunning {
		return nil
	}

	// Signal the process to stop
	close(process.stopChan)

	// Try graceful shutdown first
	if process.Command.Process != nil {
		if err := process.Command.Process.Signal(os.Interrupt); err != nil {
			// Force kill if graceful shutdown fails
			process.Command.Process.Kill()
		}

		// Wait for process to exit
		process.Command.Wait()
	}

	process.isRunning = false
	delete(pm.processes, name)

	return nil
}

// StopAll stops all managed processes
func (pm *ProcessManager) StopAll() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	var errors []string

	// Close gRPC connection first
	if pm.handlerConn != nil {
		pm.handlerConn.Close()
		pm.handlerConn = nil
		pm.handlerClient = nil
	}

	// Stop all processes
	for name := range pm.processes {
		if err := pm.stopProcess(name); err != nil {
			errors = append(errors, fmt.Sprintf("failed to stop %s: %v", name, err))
		}
	}

	pm.isInitialized = false

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping processes: %s", strings.Join(errors, "; "))
	}

	return nil
}

// HandlerConfig represents configuration for the handler service
type HandlerConfig struct {
	Port         int
	HandlersPath string
	Verbose      bool
	HotReload    bool
}

// AutoDetectHandlerConfig tries to detect handler configuration from the app structure
func (pm *ProcessManager) AutoDetectHandlerConfig() HandlerConfig {
	config := HandlerConfig{
		Port:      50052,
		Verbose:   pm.verbose,
		HotReload: true,
	}

	// Try to find handlers directory
	possiblePaths := []string{
		filepath.Join(pm.appRoot, "domains"),
		filepath.Join(pm.appRoot, "handlers"),
		filepath.Join(pm.appRoot, "app", "handlers"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			config.HandlersPath = path
			break
		}
	}

	if config.HandlersPath == "" {
		config.HandlersPath = filepath.Join(pm.appRoot, "domains")
	}

	return config
}

// ExecuteHandler calls the handler service to process a request
func (pm *ProcessManager) ExecuteHandler(domain, action string, sqlData, requestData interface{}) (interface{}, error) {
	if !pm.isInitialized {
		return nil, fmt.Errorf("handler service not initialized")
	}

	client := pm.GetHandlerClient()
	if client == nil {
		return nil, fmt.Errorf("handler client not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Convert data to protobuf structs
	sqlStruct, err := convertToProtobufStruct(sqlData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert SQL data: %w", err)
	}

	requestStruct, err := convertToProtobufStruct(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request data: %w", err)
	}

	// Create request
	req := &handler.HandlerRequest{
		Domain:      domain,
		Action:      action,
		SqlData:     sqlStruct,
		RequestData: requestStruct,
		Metadata: map[string]string{
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	// Call handler service
	resp, err := client.ProcessData(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("handler service call failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("handler error: %s", resp.ErrorMessage)
	}

	// Convert response back to Go data
	result := convertFromProtobufStruct(resp.ProcessedData)

	// Handle redirects
	if resp.Redirect != nil && resp.Redirect.Url != "" {
		if resultMap, ok := result.(map[string]interface{}); ok {
			resultMap["_redirect"] = map[string]interface{}{
				"url":    resp.Redirect.Url,
				"status": resp.Redirect.StatusCode,
			}
		}
	}

	return result, nil
}

// GetProcessInfo returns information about managed processes
func (pm *ProcessManager) GetProcessInfo() map[string]interface{} {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	info := map[string]interface{}{
		"initialized": pm.isInitialized,
		"app_root":    pm.appRoot,
		"processes":   make(map[string]interface{}),
	}

	for name, process := range pm.processes {
		process.mutex.RLock()
		processInfo := map[string]interface{}{
			"name":       process.Name,
			"port":       process.Port,
			"running":    process.isRunning,
			"log_prefix": process.LogPrefix,
		}
		process.mutex.RUnlock()

		info["processes"].(map[string]interface{})[name] = processInfo
	}

	return info
}
