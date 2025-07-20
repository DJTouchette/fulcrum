# TypeScript Domain Framework ✅

## Status: COMPLETE

## Description
Comprehensive TypeScript framework for building domain logic with automatic handler discovery, fluent APIs, and seamless gRPC integration.

## Implementation Details
- **Location**: `fulcrum-js/index.js`
- **Language**: TypeScript/JavaScript (ES modules)
- **Architecture**: Class-based domain system with auto-discovery
- **Communication**: gRPC client with reconnection logic

## Features Implemented
- [x] `DomainBase` class for easy domain creation
- [x] Automatic handler method discovery
- [x] Convention-based message type mapping
- [x] Fluent API builders for database and email operations
- [x] gRPC client with bidirectional streaming
- [x] Automatic reconnection on connection loss
- [x] Request correlation and timeout handling
- [x] Error handling with automatic error responses
- [x] Lifecycle hooks (`onStart`)

## Core Classes

### DomainBase
Base class that provides the domain framework:
```javascript
class UserDomain extends DomainBase {
    constructor() {
        super('users');
    }
    
    // Auto-mapped to 'user_create_request'
    async userCreateHandler(userData, context) {
        // Business logic here
        return result;
    }
}
```

### DomainClient
Low-level gRPC client with advanced features:
- Connection management and reconnection
- Message correlation with UUIDs
- Pending request tracking
- Stream management

### Fluent API Builders
- `DatabaseBuilder` - Chainable database operations
- `EmailBuilder` - Chainable email composition

## Auto-Handler Discovery
Methods ending with "Handler" are automatically mapped to gRPC message types:
- `userCreateHandler` → `user_create_request`
- `userIndexHandler` → `user_index_request`
- `userShowHandler` → `user_show_request`

## Fluent API Usage
```javascript
// Database operations
await this.db('users', requestId)
    .create({name: 'John', email: 'john@example.com'})
    .execute();

// Email operations  
await this.email(requestId)
    .to('user@example.com')
    .subject('Welcome!')
    .template('welcome')
    .send();
```

## Connection Management
- Automatic connection to gRPC server
- Retry logic with exponential backoff
- Stream error handling and recovery
- Graceful shutdown support

## Files
- `fulcrum-js/index.js` - Complete framework implementation
- `fulcrum-js/framework.proto` - Protocol buffer definitions

## Example Usage
```javascript
import { DomainBase } from 'fulcrum';

class UserDomain extends DomainBase {
    constructor() {
        super('users');
    }
    
    async userCreateHandler(userData, context) {
        const user = await this.createRecord('users', userData, context.requestId);
        await this.email(context.requestId)
            .to(userData.email)
            .subject('Welcome!')
            .send();
        return user;
    }
}

const domain = new UserDomain();
await domain.start();
```

## Notes
- Excellent developer experience with automatic conventions
- Robust error handling and reconnection logic
- Production-ready with proper timeout and correlation handling
- Well-designed fluent APIs that feel natural in TypeScript
