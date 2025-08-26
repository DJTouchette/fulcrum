const path = require('path');
const HandlerService = require('./HandlerService');

// Configuration from environment variables
const config = {
  port: parseInt(process.env.HANDLER_PORT) || 50052,
  handlersPath: process.env.HANDLERS_PATH || path.join(process.cwd(), 'handlers'),
  protoPath: process.env.PROTO_PATH || path.join(__dirname, 'proto', 'handler.proto')
};

console.log('Starting Fulcrum Handler Service with config:', config);

// Create and start the handler service
const handlerService = new HandlerService(config);

// Start the service
handlerService.start();

// Display service info
setTimeout(() => {
  const info = handlerService.getInfo();
  console.log('\n=== Service Information ===');
  console.log(`Port: ${info.port}`);
  console.log(`Handlers Path: ${info.handlersPath}`);
  console.log(`Loaded Handlers: ${info.loadedHandlers.length}`);
  
  if (info.loadedHandlers.length > 0) {
    console.log('Available handlers:');
    info.loadedHandlers.forEach(handler => {
      console.log(`  - ${handler}`);
    });
  } else {
    console.log('No handlers found. Create handler.js files in your handlers directory.');
  }
  console.log('===========================\n');
}, 1000);

// Graceful shutdown
const shutdown = () => {
  console.log('\nShutting down gracefully...');
  handlerService.stop();
  process.exit(0);
};

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

// Handle uncaught exceptions
process.on('uncaughtException', (error) => {
  console.error('Uncaught exception:', error);
  handlerService.stop();
  process.exit(1);
});

process.on('unhandledRejection', (reason, promise) => {
  console.error('Unhandled rejection at:', promise, 'reason:', reason);
  handlerService.stop();
  process.exit(1);
});
