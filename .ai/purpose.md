# Config-Driven Full-Stack Framework - Project Summary

## Core Concept
Building a batteries-included, full-stack web framework that is:
- **Config-driven** - Define models, routes, and features via YAML configs
- **Domain/Feature-driven** - Organize code by business domains rather than MVC layers
- **Model and domain-driven** - Describe data models and domain rules for interaction
- **Multi-language** - Go for performance-critical parts, TypeScript for business logic

## Architecture

### Go CLI Runtime
- Acts as the main server and orchestrator
- Handles config parsing, HTTP routing, database connections
- Manages domain processes via gRPC
- Provides batteries-included features (auth, file upload, caching, etc.)
- Generates HTML with Tailwind CSS/DaisyUI

### TypeScript Domain Processes
- Each domain runs as a separate Node.js process
- Handles business logic, validation, and transformations
- Communicates with Go runtime via gRPC
- Uses fluent API that builds instruction objects rather than direct I/O

### Communication Pattern
- **gRPC** for Go ↔ TypeScript communication
- **Dependency Injection** pattern - TypeScript returns instructions, Go executes them
- **Fluent APIs** make TypeScript feel natural while keeping I/O in Go

## File Structure
```
my-app/
├── framework.yaml          # Root config (server, database, features)
├── domains/
│   ├── user/
│   │   ├── config.yaml     # User domain config (models, routes)
│   │   ├── user.ts         # User domain logic
│   │   └── templates/      # User-specific templates
│   └── order/
│       ├── config.yaml     # Order domain config
│       ├── order.ts        # Order domain logic
│       └── templates/
└── shared/
    ├── middleware/
    └── templates/
```

## Key Features
- **Hot reloading** in development
- **Single binary deployment** (Go CLI + domain processes)
- **Type safety** with protobuf definitions
- **Process isolation** for domains
- **Familiar developer experience** with TypeScript
- **Batteries included** (auth, file uploads, email, caching, etc.)

## Example Usage
```bash
# CLI commands
myframework serve --config framework.yaml
myframework dev --watch --port 3000
myframework build --output ./dist
```

## Technical Stack
- **Go** - CLI runtime, HTTP server, database, gRPC server
- **TypeScript/Node.js** - Domain logic, gRPC clients  
- **gRPC** - Inter-process communication
- **Protocol Buffers** - Type-safe message definitions
- **YAML** - Configuration format
- **Tailwind CSS/DaisyUI** - UI framework

This creates a developer-friendly framework where you define your app structure via configs and write business logic in TypeScript, while getting the performance and deployment benefits of Go.
