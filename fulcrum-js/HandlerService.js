const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');
const HandlerRegistry = require('./HandlerRegistry');

class HandlerService {
  constructor(options = {}) {
    this.port = options.port || 50052;
    this.protoPath = options.protoPath || path.join(__dirname, 'proto', 'handler.proto');
    this.handlersPath = options.handlersPath || './handlers';
    this.server = null;
    
    // Initialize handler registry
    this.registry = options.registry || new HandlerRegistry({
      handlersPath: this.handlersPath,
      hotReload: true
    });
    
    // Load proto definition
    this.loadProtoDefinition();
  }
  
  loadProtoDefinition() {
    const packageDefinition = protoLoader.loadSync(this.protoPath, {
      keepCase: true,
      longs: String,
      enums: String,
      defaults: true,
      oneofs: true
    });
    
    this.handlerProto = grpc.loadPackageDefinition(packageDefinition).handler;
  }
  
  // gRPC method implementations
  processData(call, callback) {
    const request = call.request;
    
    console.log(`Processing handler request: ${request.domain}.${request.action}`);
    
    try {
      // Convert protobuf Struct to JavaScript objects
      const sqlData = this.structToObject(request.sql_data);
      const requestData = this.structToObject(request.request_data);
      
      // Extract parameters from metadata or request
      const params = this.extractParams(request);
      
      console.log(`SQL data:`, sqlData);
      console.log(`Request data:`, requestData);
      console.log(`Parameters:`, params);
      
      // Process through handler registry
      const result = this.registry.processRequest({
        domain: request.domain,
        action: request.action,
        params: params,
        sql: sqlData,
        request: requestData
      });
      
      if (result.success) {
        const response = {
          success: true,
          processed_data: this.objectToStruct(result.data),
          error_message: '',
          metadata: {}
        };
        
        // Handle redirects
        if (result.data && result.data._redirect) {
          response.redirect = {
            url: result.data._redirect.url,
            status_code: result.data._redirect.status || 303
          };
          
          // Remove redirect from processed data to avoid template confusion
          const cleanData = { ...result.data };
          delete cleanData._redirect;
          response.processed_data = this.objectToStruct(cleanData);
        }
        
        console.log(`Handler completed successfully`);
        callback(null, response);
      } else {
        console.error(`Handler error: ${result.error}`);
        callback(null, {
          success: false,
          processed_data: this.objectToStruct({}),
          error_message: result.error,
          metadata: {}
        });
      }
      
    } catch (error) {
      console.error(`Processing error:`, error);
      
      callback(null, {
        success: false,
        processed_data: this.objectToStruct({}),
        error_message: error.message,
        metadata: {}
      });
    }
  }
  
  // Health check implementation
  health(call, callback) {
    const handlerCount = this.registry.listHandlers().length;
    
    callback(null, {
      healthy: true,
      version: '1.0.0',
      service_name: 'fulcrum-handler-service',
      metadata: {
        handlers_loaded: handlerCount.toString(),
        handlers_path: this.handlersPath
      }
    });
  }
  
  // Extract parameters from the request
  extractParams(request) {
    const params = {};
    
    // Get params from metadata
    if (request.metadata) {
      Object.entries(request.metadata).forEach(([key, value]) => {
        if (key.endsWith('_id') || key.startsWith('param_')) {
          params[key] = value;
        }
      });
    }
    
    // Parse route path for parameters
    if (request.route_path) {
      const pathParts = request.route_path.split('/');
      pathParts.forEach(part => {
        if (part.startsWith(':')) {
          const paramName = part.substring(1);
          // You'd need to extract the actual value from somewhere
          // This is a simplified implementation
          params[paramName] = request.metadata[paramName] || null;
        }
      });
    }
    
    return params;
  }
  
  // Convert protobuf Struct to JavaScript object
  structToObject(struct) {
    if (!struct || !struct.fields) {
      return {};
    }
    
    const result = {};
    for (const [key, value] of Object.entries(struct.fields)) {
      result[key] = this.valueToJavaScript(value);
    }
    return result;
  }
  
  // Convert protobuf Value to JavaScript value
  valueToJavaScript(value) {
    if (!value) return null;
    
    switch (value.kind) {
      case 'nullValue':
        return null;
      case 'numberValue':
        return value.numberValue;
      case 'stringValue':
        return value.stringValue;
      case 'boolValue':
        return value.boolValue;
      case 'structValue':
        return this.structToObject(value.structValue);
      case 'listValue':
        return value.listValue.values.map(v => this.valueToJavaScript(v));
      default:
        return null;
    }
  }
  
  // Convert JavaScript object to protobuf Struct
  objectToStruct(obj) {
    if (!obj || typeof obj !== 'object') {
      return { fields: {} };
    }
    
    const fields = {};
    
    for (const [key, value] of Object.entries(obj)) {
      fields[key] = this.javaScriptToValue(value);
    }
    
    return { fields };
  }
  
  // Convert JavaScript value to protobuf Value
  javaScriptToValue(value) {
    if (value === null || value === undefined) {
      return { kind: 'nullValue', nullValue: 'NULL_VALUE' };
    } else if (typeof value === 'number') {
      return { kind: 'numberValue', numberValue: value };
    } else if (typeof value === 'string') {
      return { kind: 'stringValue', stringValue: value };
    } else if (typeof value === 'boolean') {
      return { kind: 'boolValue', boolValue: value };
    } else if (Array.isArray(value)) {
      return {
        kind: 'listValue',
        listValue: {
          values: value.map(v => this.javaScriptToValue(v))
        }
      };
    } else if (typeof value === 'object') {
      return {
        kind: 'structValue',
        structValue: this.objectToStruct(value)
      };
    } else {
      return { kind: 'stringValue', stringValue: String(value) };
    }
  }
  
  // Start the gRPC server
  start() {
    this.server = new grpc.Server();
    
    // Add the handler service
    this.server.addService(this.handlerProto.HandlerService.service, {
      ProcessData: this.processData.bind(this),
      Health: this.health.bind(this)
    });
    
    const address = `0.0.0.0:${this.port}`;
    
    this.server.bindAsync(address, grpc.ServerCredentials.createInsecure(), (err, port) => {
      if (err) {
        console.error('Failed to start gRPC server:', err);
        return;
      }
      
      console.log(`Handler Service started on ${address}`);
      console.log(`gRPC server listening on port ${port}`);
      console.log(`Handlers loaded: ${this.registry.listHandlers().length}`);
      console.log(`Available handlers:`, this.registry.listHandlers());
      
      this.server.start();
    });
  }
  
  // Stop the server
  stop() {
    if (this.server) {
      this.server.tryShutdown(() => {
        console.log('Handler Service stopped');
      });
    }
    
    // Cleanup handler registry
    this.registry.destroy();
  }
  
  // Get service info for debugging
  getInfo() {
    return {
      port: this.port,
      handlersPath: this.handlersPath,
      loadedHandlers: this.registry.listHandlers(),
      handlerInfo: this.registry.getHandlerInfo()
    };
  }
}

module.exports = HandlerService;
