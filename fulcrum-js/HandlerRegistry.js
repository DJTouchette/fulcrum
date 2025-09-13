const fs = require('fs');
const path = require('path');

class HandlerRegistry {
  constructor(options = {}) {
    this.handlersPath = options.handlersPath || './handlers';
    this.basePath = options.basePath || process.cwd();
    this.handlers = new Map();
    this.fileWatchers = new Map();
    this.hotReload = options.hotReload !== false; // Default to true
    this.domainStream = null;
    this.pendingRequests = new Map();
    this.requestCounter = 0;
    
    this.loadAllHandlers();
    
    if (this.hotReload) {
      this.setupHotReloading();
    }
  }

  setDomainStream(stream) {
    this.domainStream = stream;

    this.domainStream.on('data', (message) => {
      const promise = this.pendingRequests.get(message.request_id);
      if (promise) {
        if (message.success) {
          promise.resolve(JSON.parse(message.payload));
        } else {
          promise.reject(new Error(message.error));
        }
        this.pendingRequests.delete(message.request_id);
      }
    });
  }

  sendFrameworkMessage(type, payload, req) {
    return new Promise((resolve, reject) => {
      const requestId = `${req._path}-${this.requestCounter++}`;
      this.pendingRequests.set(requestId, { resolve, reject });

      this.domainStream.write({
        domain: 'fulcrum-js',
        type: type,
        request_id: requestId,
        payload: JSON.stringify(payload)
      });
    });
  }
  
  // Walk the directory tree and load all handler.js files
  loadAllHandlers() {
    console.log(`Loading handlers from: ${this.handlersPath}`);
    
    if (!fs.existsSync(this.handlersPath)) {
      console.warn(`Handlers directory not found: ${this.handlersPath}`);
      return;
    }
    
    this.walkDirectory(this.handlersPath);
    console.log(`Loaded ${this.handlers.size} handlers`);
  }
  
  walkDirectory(dir) {
    const items = fs.readdirSync(dir);
    
    for (const item of items) {
      const fullPath = path.join(dir, item);
      const stat = fs.statSync(fullPath);
      
      if (stat.isDirectory()) {
        this.walkDirectory(fullPath);
      } else if (item === 'handler.js') {
        this.loadHandler(fullPath);
      }
    }
  }
  
  loadHandler(filePath) {
    try {
      // Clear require cache for hot reloading
      delete require.cache[path.resolve(filePath)];
      
      const handler = require(path.resolve(filePath));
      const handlerId = this.generateHandlerId(filePath);
      
      this.handlers.set(handlerId, {
        path: filePath,
        handler: handler,
        lastModified: fs.statSync(filePath).mtime
      });
      
      console.log(`Loaded handler: ${handlerId} -> ${filePath}`);
    } catch (error) {
      console.error(`Failed to load handler ${filePath}:`, error.message);
    }
  }
  
  // Generate handler ID from file path using convention
  generateHandlerId(filePath) {
    // Convert: ./handlers/users/edit/handler.js -> users.edit
    // Convert: ./handlers/orders/[order_id]/items/handler.js -> orders.[order_id].items
    
    const relativePath = path.relative(this.handlersPath, filePath);
    const parts = path.dirname(relativePath).split(path.sep);
    
    // Filter out empty parts and convert [param] to {param}
    const cleanParts = parts
      .filter(part => part && part !== '.')
      .map(part => {
        // Convert [param] to {param} for consistency
        if (part.startsWith('[') && part.endsWith(']')) {
          return '{' + part.slice(1, -1) + '}';
        }
        return part;
      });
    
    return cleanParts.join('.');
  }

  // In your HandlerRegistry.js route method, add debug logging:
async route(domain, action, params = {}, context) {
    console.log(`🔍 Looking for handler: ${domain}.${action}`);
    console.log(`🔍 Available handlers:`, Array.from(this.handlers.keys()));
    
    // Try exact match first
    const exactId = `${domain}.${action}`;
    console.log(`🔍 Trying exact match: ${exactId}`);
    
    if (this.handlers.has(exactId)) {
        console.log(`✅ Found exact match: ${exactId}`);
        return await this.executeHandler(exactId, context);
    }
    
    console.log(`❌ No exact match, trying parameterized...`);
    
    // Try with parameter substitution
    const parameterizedId = this.findParameterizedHandler(domain, action, params);
    console.log(`🔍 Parameterized result: ${parameterizedId}`);
    
    if (parameterizedId) {
        return await this.executeHandler(parameterizedId, context);
    }
    
    console.log(`❌ No handler found for: ${domain}.${action}`);
    throw new Error(`Handler not found for: ${domain}.${action}`);
}
  
  // Find handler that matches with parameters
  findParameterizedHandler(domain, action, params) {
    const targetPattern = `${domain}.${action}`;
    
    for (const [handlerId] of this.handlers) {
      if (this.matchesPattern(handlerId, targetPattern, params)) {
        return handlerId;
      }
    }
    
    return null;
  }
  
  // Check if a handler ID pattern matches the target with parameters
  matchesPattern(handlerId, targetPattern, params) {
    const handlerParts = handlerId.split('.');
    const targetParts = targetPattern.split('.');
    
    if (handlerParts.length !== targetParts.length) {
      return false;
    }
    
    for (let i = 0; i < handlerParts.length; i++) {
      const handlerPart = handlerParts[i];
      const targetPart = targetParts[i];
      
      // If handler part is a parameter, it matches anything
      if (handlerPart.startsWith('{') && handlerPart.endsWith('}')) {
        continue;
      }
      
      // Otherwise, must be exact match
      if (handlerPart !== targetPart) {
        return false;
      }
    }
    
    return true;
  }
  
  async executeHandler(handlerId, context) {
    const handlerInfo = this.handlers.get(handlerId);
    
    if (!handlerInfo) {
      throw new Error(`Handler not found: ${handlerId}`);
    }
    
    // Check for hot reload
    if (this.hotReload) {
      const currentMtime = fs.statSync(handlerInfo.path).mtime;
      if (currentMtime > handlerInfo.lastModified) {
        console.log(`Reloading handler: ${handlerId}`);
        this.loadHandler(handlerInfo.path);
      }
    }
    
    const handler = this.handlers.get(handlerId).handler;
    
    if (typeof handler === 'function') {
      return await handler(context);
    } else if (typeof handler.handle === 'function') {
      return await handler.handle(context);
    } else if (typeof handler.process === 'function') {
      return await handler.process(context);
    } else {
      throw new Error(`Handler ${handlerId} must export a function or object with handle/process method`);
    }
  }
  
  // Public method to process requests (called from Go via gRPC)
  async processRequest(requestData) {
    try {
      const { domain, action, params = {}, sql = null, request = {} } = requestData;
      
      // Create context object
      const context = {
        domain,
        action,
        params,
        sql,
        request,
        route: {
          domain,
          action,
          params
        },
        utils: this.createUtilities(),
        fulcrum: {
          db: {
            find: async (table, query) => await this.sendFrameworkMessage('db_find', { table, query }, request),
            create: async (table, data) => await this.sendFrameworkMessage('db_create', { table, data }, request),
            update: async (table, id, data) => await this.sendFrameworkMessage('db_update', { table, id, data }, request),
          }
        }
      };
      
      // Route to appropriate handler
      const handlerResult = await this.route(domain, action, params, context);
      
      return {
        success: true,
        data: handlerResult,
        error: null
      };
      
    } catch (error) {
      console.error('Handler execution error:', error);
      
      return {
        success: false,
        data: null,
        error: error.message
      };
    }
  }
  
  // Create utility functions available to all handlers
  createUtilities() {
    return {
      formatDate: (date) => new Date(date).toLocaleDateString(),
      formatCurrency: (amount, currency = 'USD') => {
        return new Intl.NumberFormat('en-US', {
          style: 'currency',
          currency: currency
        }).format(amount);
      },
      slugify: (text) => {
        return text
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, '-')
          .replace(/^-+|-+$/g, '');
      },
      capitalize: (text) => {
        return text.charAt(0).toUpperCase() + text.slice(1);
      },
      isEmpty: (value) => {
        return value === null || value === undefined || value === '';
      },
      groupBy: (array, key) => {
        return array.reduce((groups, item) => {
          const group = item[key];
          groups[group] = groups[group] || [];
          groups[group].push(item);
          return groups;
        }, {});
      }
    };
  }
  
  // Setup file watching for hot reloading
  setupHotReloading() {
    if (!fs.existsSync(this.handlersPath)) {
      return;
    }
    
    console.log('Setting up hot reloading...');
    
    // Watch for new handler files
    this.watchDirectory(this.handlersPath);
  }
  
  watchDirectory(dir) {
    try {
      const watcher = fs.watch(dir, { recursive: true }, (eventType, filename) => {
        if (filename && filename.endsWith('handler.js')) {
          const fullPath = path.join(dir, filename);
          
          if (eventType === 'change' && fs.existsSync(fullPath)) {
            console.log(`Handler file changed: ${filename}`);
            this.loadHandler(fullPath);
          } else if (eventType === 'rename') {
            if (fs.existsSync(fullPath)) {
              console.log(`New handler file: ${filename}`);
              this.loadHandler(fullPath);
            } else {
              // File was deleted
              const handlerId = this.generateHandlerId(fullPath);
              this.handlers.delete(handlerId);
              console.log(`Handler file removed: ${handlerId}`);
            }
          }
        }
      });
      
      this.fileWatchers.set(dir, watcher);
    } catch (error) {
      console.warn(`Could not watch directory ${dir}:`, error.message);
    }
  }
  
  // Get handler information for debugging
  getHandlerInfo() {
    const info = {};
    
    for (const [handlerId, handlerInfo] of this.handlers) {
      info[handlerId] = {
        path: handlerInfo.path,
        lastModified: handlerInfo.lastModified
      };
    }
    
    return info;
  }
  
  // List all available handlers
  listHandlers() {
    return Array.from(this.handlers.keys()).sort();
  }
  
  // Check if a specific handler exists
  hasHandler(domain, action, params = {}) {
    try {
      this.route(domain, action, params);
      return true;
    } catch {
      return false;
    }
  }
  
  // Cleanup
  destroy() {
    for (const [dir, watcher] of this.fileWatchers) {
      watcher.close();
      console.log(`Stopped watching: ${dir}`);
    }
    
    this.fileWatchers.clear();
    this.handlers.clear();
  }
}

module.exports = HandlerRegistry;
