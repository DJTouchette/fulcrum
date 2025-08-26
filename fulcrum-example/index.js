const { FulcrumJS } = require('@fulcrum/js');

async function startApp() {
  console.log('Starting Fulcrum Example Application...');
  
  // Create handler service with auto-discovered domains
  const handlerService = new FulcrumJS({
    port: 50052,
    handlersPath: './domains', // Will scan domains for handler.js files
    hotReload: true,
    verbose: true
  });
  
  try {
    // Initialize and start the handler service
    await handlerService.initialize();
    await handlerService.start();
    
    // Setup graceful shutdown
    handlerService.setupGracefulShutdown();
    
    console.log('Handler service started successfully!');
    console.log('The Go framework can now connect to this service on port 50052');
    console.log('Press Ctrl+C to stop');
    
    // Keep the process alive
    await new Promise(() => {});
    
  } catch (error) {
    console.error('Failed to start handler service:', error);
    process.exit(1);
  }
}

// Check if we're running directly
if (require.main === module) {
  startApp();
}

module.exports = { startApp };
