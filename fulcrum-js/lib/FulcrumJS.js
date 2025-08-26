const path = require('path');
const HandlerRegistry = require('../HandlerRegistry');
const HandlerService = require('../HandlerService');

class FulcrumJS {
  constructor(options = {}) {
    this.options = {
      port: options.port || 50052,
      handlersPath: options.handlersPath || this.discoverHandlersPath(),
      protoPath: options.protoPath || path.join(__dirname, '..', 'proto', 'handler.proto'),
      hotReload: options.hotReload !== false,
      verbose: options.verbose || false,
      ...options
    };
    
    this.registry = null;
    this.service = null;
    this.isRunning = false;
    
    if (this.options.verbose) {
      console.log('FulcrumJS initialized with options:', this.options);
    }
  }
  
  // Auto-discover handlers path relative to the application
  discoverHandlersPath() {
    const cwd = process.cwd();
    
    // Common handler locations to check
    const possiblePaths = [
      path.join(cwd, 'handlers'),
      path.join(cwd, 'domains'),
      path.join(cwd, 'app', 'handlers'),
      path.join(cwd, 'src', 'handlers')
    ];
    
    const fs = require('fs');
    
    for (const possiblePath of possiblePaths) {
      if (fs.existsSync(possiblePath)) {
        if (this.options.verbose) {
          console.log(`Auto-discovered handlers path: ${possiblePath}`);
        }
        return possiblePath;
      }
    }
    
    // Default fallback
    return path.join(cwd, 'handlers');
  }
  
  // Initialize the framework components
  async initialize() {
    try {
      // Create handler registry
      this.registry = new HandlerRegistry({
        handlersPath: this.options.handlersPath,
        hotReload: this.options.hotReload,
        verbose: this.options.verbose
      });
      
      // Create gRPC service
      this.service = new HandlerService({
        port: this.options.port,
        protoPath: this.options.protoPath,
        registry: this.registry,
        verbose: this.options.verbose
      });
      
      if (this.options.verbose) {
        console.log('FulcrumJS components initialized successfully');
      }
      
      return true;
    } catch (error) {
      console.error('Failed to initialize FulcrumJS:', error);
      throw error;
    }
  }
  
  // Start the handler service
  async start() {
    if (this.isRunning) {
      console.warn('FulcrumJS is already running');
      return;
    }
    
    if (!this.service) {
      await this.initialize();
    }
    
    try {
      await this.service.start();
      this.isRunning = true;
      
      if (this.options.verbose) {
        this.logServiceInfo();
      }
      
      return true;
    } catch (error) {
      console.error('Failed to start FulcrumJS service:', error);
      throw error;
    }
  }
  
  // Stop the handler service
  async stop() {
    if (!this.isRunning) {
      return;
    }
    
    try {
      if (this.service) {
        await this.service.stop();
      }
      
      if (this.registry) {
        this.registry.destroy();
      }
      
      this.isRunning = false;
      
      if (this.options.verbose) {
        console.log('FulcrumJS stopped successfully');
      }
      
      return true;
    } catch (error) {
      console.error('Error stopping FulcrumJS:', error);
      throw error;
    }
  }
  
  // Get service information
  getInfo() {
    if (!this.registry || !this.service) {
      return {
        status: 'not_initialized',
        options: this.options
      };
    }
    
    return {
      status: this.isRunning ? 'running' : 'stopped',
      options: this.options,
      handlers: {
        loaded: this.registry.listHandlers(),
        count: this.registry.listHandlers().length,
        info: this.registry.getHandlerInfo()
      },
      service: {
        port: this.options.port,
        address: `localhost:${this.options.port}`
      }
    };
  }
  
  // Log service information
  logServiceInfo() {
    const info = this.getInfo();
    
    console.log('\n=== FulcrumJS Service Information ===');
    console.log(`Status: ${info.status}`);
    console.log(`Port: ${info.service.port}`);
    console.log(`Handlers Path: ${info.options.handlersPath}`);
    console.log(`Loaded Handlers: ${info.handlers.count}`);
    
    if (info.handlers.count > 0) {
      console.log('Available handlers:');
      info.handlers.loaded.forEach(handler => {
        console.log(`  - ${handler}`);
      });
    } else {
      console.log('No handlers found. Create handler.js files in your domains.');
    }
    console.log('=====================================\n');
  }
  
  // Check if a handler exists
  hasHandler(domain, action, params = {}) {
    if (!this.registry) {
      return false;
    }
    
    return this.registry.hasHandler(domain, action, params);
  }
  
  // Process a request directly (for testing)
  async processRequest(requestData) {
    if (!this.registry) {
      throw new Error('FulcrumJS not initialized');
    }
    
    return this.registry.processRequest(requestData);
  }
  
  // Health check
  async healthCheck() {
    if (!this.service) {
      return { healthy: false, error: 'Service not initialized' };
    }
    
    try {
      // Implement health check logic
      return {
        healthy: this.isRunning,
        version: '1.0.0',
        service_name: 'fulcrum-js',
        handlers_loaded: this.registry ? this.registry.listHandlers().length : 0
      };
    } catch (error) {
      return { healthy: false, error: error.message };
    }
  }
  
  // Graceful shutdown handler
  setupGracefulShutdown() {
    const shutdown = async (signal) => {
      console.log(`\nReceived ${signal}, shutting down gracefully...`);
      
      try {
        await this.stop();
        process.exit(0);
      } catch (error) {
        console.error('Error during shutdown:', error);
        process.exit(1);
      }
    };
    
    process.on('SIGINT', () => shutdown('SIGINT'));
    process.on('SIGTERM', () => shutdown('SIGTERM'));
    
    // Handle uncaught exceptions
    process.on('uncaughtException', async (error) => {
      console.error('Uncaught exception:', error);
      await this.stop();
      process.exit(1);
    });
    
    process.on('unhandledRejection', async (reason, promise) => {
      console.error('Unhandled rejection at:', promise, 'reason:', reason);
      await this.stop();
      process.exit(1);
    });
  }
}

module.exports = FulcrumJS;
