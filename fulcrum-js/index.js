import grpc from '@grpc/grpc-js';
import protoLoader from '@grpc/proto-loader';
import { v4 as uuidv4 } from 'uuid';
import path from 'path';

class DomainClient {
    constructor(domainName, serverAddress = 'localhost:50051') {
        this.domainName = domainName;
        this.serverAddress = serverAddress;
        this.client = null;
        this.stream = null;
        this.messageHandlers = new Map();
        this.pendingRequests = new Map();
    }

    async connect() {
        try {
            const protoPath = new URL('framework.proto', import.meta.url).pathname;
            // Load the proto file
            const packageDefinition = protoLoader.loadSync(
                protoPath,
                {
                    keepCase: true,
                    longs: String,
                    enums: String,
                    defaults: true,
                    oneofs: true
                }
            );

            const proto = grpc.loadPackageDefinition(packageDefinition).framework;
            
            // Create client
            this.client = new proto.FrameworkService(
                this.serverAddress,
                grpc.credentials.createInsecure()
            );

            // Start bidirectional streaming
            this.stream = this.client.DomainCommunication();
            
            // Handle incoming messages
            this.stream.on('data', (response) => {
                this.handleResponse(response);
            });

            this.stream.on('error', (error) => {
                console.error('Stream error:', error);
                setTimeout(() => this.reconnect(), 5000);
            });

            this.stream.on('end', () => {
                console.log('Stream ended');
                setTimeout(() => this.reconnect(), 5000);
            });

            console.log(`Domain "${this.domainName}" connected to ${this.serverAddress}`);
            
            // Register domain
            await this.sendMessage('domain_register', {
                domain: this.domainName,
                capabilities: ['db_operations', 'email_operations']
            });

        } catch (error) {
            console.error('Failed to connect:', error);
            setTimeout(() => this.reconnect(), 5000);
        }
    }

    async reconnect() {
        console.log('Attempting to reconnect...');
        await this.connect();
    }

    // Modified to accept optional requestId parameter
    async sendMessage(type, payload, waitForResponse = false, requestId = null) {
        const messageId = requestId || uuidv4();
        
        const message = {
            domain: this.domainName,
            type: type,
            payload: JSON.stringify(payload),
            request_id: messageId
        };

        if (waitForResponse) {
            return new Promise((resolve, reject) => {
                this.pendingRequests.set(messageId, { resolve, reject });
                this.stream.write(message);
                
                // Timeout after 30 seconds
                setTimeout(() => {
                    if (this.pendingRequests.has(messageId)) {
                        this.pendingRequests.delete(messageId);
                        reject(new Error('Request timeout'));
                    }
                }, 30000);
            });
        } else {
            this.stream.write(message);
            return messageId;
        }
    }

    handleResponse(response) {
        console.log(`Received response: ${response.type} (${response.request_id})`);
        
        // Handle pending requests
        if (response.request_id && this.pendingRequests.has(response.request_id)) {
            const { resolve, reject } = this.pendingRequests.get(response.request_id);
            this.pendingRequests.delete(response.request_id);
            
            if (response.success) {
                resolve(response);
            } else {
                reject(new Error(response.error || 'Unknown error'));
            }
            return;
        }

        // Handle message types
        const handler = this.messageHandlers.get(response.type);
        if (handler) {
            try {
                handler(response);
            } catch (error) {
                console.error(`Error handling ${response.type}:`, error);
            }
        } else {
            console.log(`No handler for message type: ${response.type}`);
        }
    }

    // Register message handlers
    on(messageType, handler) {
        this.messageHandlers.set(messageType, handler);
    }

    // Helper methods for common operations - now accept requestId
    async createRecord(table, data, requestId = null) {
        return await this.sendMessage('db_create', {
            table: table,
            data: data
        }, true, requestId);
    }

    async updateRecord(table, id, data, requestId = null) {
        return await this.sendMessage('db_update', {
            table: table,
            id: id,
            data: data
        }, true, requestId);
    }

    async findRecords(table, query = {}, requestId = null) {
        return await this.sendMessage('db_find', {
            table: table,
            query: query
        }, true, requestId);
    }

    async sendEmail(to, subject, body, template = null, requestId = null) {
        return await this.sendMessage('email_send', {
            to: to,
            subject: subject,
            body: body,
            template: template
        }, true, requestId);
    }

    // Fluent API builder - now accepts requestId
    db(table, requestId = null) {
        return new DatabaseBuilder(this, table, requestId);
    }

    email(requestId = null) {
        return new EmailBuilder(this, requestId);
    }

    disconnect() {
        if (this.stream) {
            this.stream.end();
        }
    }
}

// Fluent API for database operations
class DatabaseBuilder {
    constructor(domain, table, requestId = null) {
        this.domain = domain;
        this.table = table;
        this.requestId = requestId;
        this.operations = [];
    }

    create(data) {
        this.operations.push({
            type: 'create',
            data: data
        });
        return this;
    }

    update(id, data) {
        this.operations.push({
            type: 'update',
            id: id,
            data: data
        });
        return this;
    }

    find(query = {}) {
        this.operations.push({
            type: 'find',
            query: query
        });
        return this;
    }

    async execute() {
        const results = [];
        for (const op of this.operations) {
            let result;
            switch (op.type) {
                case 'create':
                    result = await this.domain.createRecord(this.table, op.data, this.requestId);
                    break;
                case 'update':
                    result = await this.domain.updateRecord(this.table, op.id, op.data, this.requestId);
                    break;
                case 'find':
                    result = await this.domain.findRecords(this.table, op.query, this.requestId);
                    break;
            }
            results.push(result);
        }
        return results.length === 1 ? results[0] : results;
    }
}

// Fluent API for email operations
class EmailBuilder {
    constructor(domain, requestId = null) {
        this.domain = domain;
        this.requestId = requestId;
        this.emailData = {};
    }

    to(email) {
        this.emailData.to = email;
        return this;
    }

    subject(subject) {
        this.emailData.subject = subject;
        return this;
    }

    body(body) {
        this.emailData.body = body;
        return this;
    }

    template(templateName) {
        this.emailData.template = templateName;
        return this;
    }

    // Method to set request ID for chaining
    withRequestId(requestId) {
        this.requestId = requestId;
        return this;
    }

    async send() {
        return await this.domain.sendEmail(
            this.emailData.to,
            this.emailData.subject,
            this.emailData.body,
            this.emailData.template,
            this.requestId
        );
    }
}

export class DomainBase {
    constructor(domainName, serverAddress = 'localhost:50051') {
        this.client = new DomainClient(domainName, serverAddress);
        this.setupAutoHandlers();
    }

    /**
     * Automatically discovers handler methods and sets up gRPC handlers
     * Convention: methods ending with "Handler" become gRPC message handlers
     * Method name "userCreateHandler" -> handles "user_create_request" messages
     */
    setupAutoHandlers() {
        const methods = Object.getOwnPropertyNames(Object.getPrototypeOf(this))
            .filter(name => name.endsWith('Handler') && typeof this[name] === 'function');

        methods.forEach(methodName => {
            // Convert camelCase to snake_case and add _request suffix
            // userCreateHandler -> user_create_request
            const messageName = this.methodNameToMessageType(methodName);
            
            console.log(`Setting up handler: ${methodName} -> ${messageName}`);
            
            this.client.on(messageName, async (message) => {
                await this.handleMessage(methodName, message);
            });
        });
    }

    /**
     * Convert method name to message type
     * userCreateHandler -> user_create_request
     * userIndexHandler -> user_index_request
     */
    methodNameToMessageType(methodName) {
        // Remove 'Handler' suffix
        const baseName = methodName.replace(/Handler$/, '');
        
        // Convert camelCase to snake_case
        const snakeCase = baseName.replace(/([A-Z])/g, '_$1').toLowerCase();
        
        // Add _request suffix
        return `${snakeCase}_request`;
    }

    /**
     * Handle incoming messages by calling the appropriate handler method
     */
    async handleMessage(methodName, message) {
        try {
            console.log(`Handling ${methodName} with message:`, message.type);
            
            // Parse payload if it's JSON
            let payload;
            try {
                payload = JSON.parse(message.payload);
            } catch {
                payload = message.payload;
            }

            // Call the handler method with payload and full message context
            const result = await this[methodName](payload, {
                requestId: message.request_id,
                domain: message.domain,
                type: message.type,
                client: this.client
            });

            // Auto-generate response message type
            const responseType = message.type.replace('_request', '_response');
            
            // Send response
            await this.client.sendMessage(responseType, result, false, message.request_id);

        } catch (error) {
            console.error(`Error in ${methodName}:`, error);
            
            const responseType = message.type.replace('_request', '_response');
            await this.client.sendMessage(responseType, {
                success: false,
                error: error.message
            }, false, message.request_id);
        }
    }

    /**
     * Start the domain service
     */
    async start() {
        console.log(`Starting ${this.client.domainName} domain...`);
        await this.client.connect();
        console.log(`${this.client.domainName} domain started and connected to framework`);
        
        // Call onStart hook if it exists
        if (typeof this.onStart === 'function') {
            await this.onStart();
        }
    }

    /**
     * Stop the domain service
     */
    stop() {
        this.client.disconnect();
    }

    // Convenient access to client methods
    get db() {
        return (table, requestId) => this.client.db(table, requestId);
    }

    get email() {
        return (requestId) => this.client.email(requestId);
    }

    async createRecord(table, data, requestId) {
        return await this.client.createRecord(table, data, requestId);
    }

    async updateRecord(table, id, data, requestId) {
        return await this.client.updateRecord(table, id, data, requestId);
    }

    async findRecords(table, query = {}, requestId) {
        return await this.client.findRecords(table, query, requestId);
    }

    async sendEmail(to, subject, body, template = null, requestId) {
        return await this.client.sendEmail(to, subject, body, template, requestId);
    }
}

export default { DomainClient, DatabaseBuilder, EmailBuilder, DomainBase }
