# gRPC Communication System ✅

## Status: COMPLETE

## Description
Bidirectional gRPC communication system enabling Go runtime to communicate with TypeScript domain processes using Protocol Buffers.

## Implementation Details
- **Location**: `lib/lang/adapters/`
- **Protocol**: gRPC with bidirectional streaming
- **Message Format**: Protocol Buffers
- **Port**: 50051 (gRPC server)

## Features Implemented
- [x] Protocol Buffer definitions (`framework.proto`)
- [x] Generated Go gRPC server code
- [x] Bidirectional streaming communication
- [x] Request/response correlation with UUIDs
- [x] Domain stream management and registration
- [x] Timeout handling (30 seconds)
- [x] Connection management and reconnection
- [x] Pending request tracking
- [x] Automatic cleanup of expired requests

## Files
- `lib/lang/adapters/framework.proto` - Protocol definitions
- `lib/lang/adapters/framework.pb.go` - Generated protobuf code
- `lib/lang/adapters/framework_grpc.pb.go` - Generated gRPC code  
- `lib/lang/adapters/grpc.go` - Server implementation
- `lib/lang/adapters/start.go` - Server startup logic

## Message Types
```protobuf
service FrameworkService {
    rpc DomainCommunication(stream DomainMessage) returns (stream RuntimeMessage);
    rpc SendMessage(DomainMessage) returns (RuntimeMessage);
}

message DomainMessage {
    string domain = 1;
    string type = 2;
    string payload = 3;
    string request_id = 4;
}

message RuntimeMessage {
    string type = 1;
    string payload = 2;
    string request_id = 3;
    bool success = 4;
    string error = 5;
}
```

## Communication Flow
1. TypeScript domains connect via `DomainCommunication` stream
2. HTTP requests trigger `SendMessage` to domains
3. Domains respond with correlated `request_id`
4. Framework tracks pending requests with timeouts

## Notes
- Robust error handling and reconnection logic
- Well-designed message correlation system
- Ready for production use with proper timeout management
