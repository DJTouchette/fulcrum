package lang_adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Message struct {
	ID      string
	Domain  string
	Type    string
	Payload string
}

type MessageBus interface {
	SendToDomain(domain, msgType string, payload any) error
	GetPendingMessages(domain string) ([]Message, error)
	MarkProcessed(messageID string) error
}

// PendingRequest tracks requests waiting for responses
type PendingRequest struct {
	RequestID string
	Response  chan *RuntimeMessage
	Timeout   time.Time
}

type FrameworkServer struct {
	UnimplementedFrameworkServiceServer
	messageBus      MessageBus
	domainStreams   map[string]FrameworkService_DomainCommunicationServer
	pendingRequests map[string]*PendingRequest
	streamMutex     sync.RWMutex
	requestMutex    sync.RWMutex
}

func (s *FrameworkServer) DomainCommunication(stream FrameworkService_DomainCommunicationServer) error {
	log.Println("Domain connected to bidirectional stream")

	var domainName string

	for {
		// Receive message from domain
		domainMsg, err := stream.Recv()
		if err == io.EOF {
			log.Printf("Domain %s disconnected", domainName)
			s.removeDomainStream(domainName)
			return nil
		}
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			s.removeDomainStream(domainName)
			return err
		}

		// Store domain name and stream on first message
		if domainName == "" {
			domainName = domainMsg.Domain
			s.addDomainStream(domainName, stream)
			log.Printf("Domain %s registered", domainName)
		}

		log.Printf("Received from domain %s: %s", domainMsg.Domain, domainMsg.Type)

		// Handle responses from domains
		if s.isResponseMessage(domainMsg.Type) {
			s.handleDomainResponse(domainMsg)
		} else {
			// Handle requests from domains (if any)
			response := s.processMessage(domainMsg)
			if err := stream.Send(response); err != nil {
				log.Printf("Error sending response: %v", err)
				return err
			}
		}
	}
}

func (s *FrameworkServer) SendMessage(ctx context.Context, req *DomainMessage) (*RuntimeMessage, error) {
	log.Printf("Received HTTP request: %s for domain: %s", req.Type, req.Domain)

	// Map HTTP route to correct domain message type
	var targetDomain string
	var messageType string

	switch req.Type {
	case "create_user_request":
		targetDomain = "users"              // Your JS domain name
		messageType = "user_create_request" // What your JS domain listens for
	default:
		targetDomain = req.Domain
		messageType = req.Type
	}

	// Check if we have a connected domain for this request
	stream := s.getDomainStream(targetDomain)
	if stream == nil {
		log.Printf("No domain stream found for: %s", targetDomain)
		return &RuntimeMessage{
			Type:      "error",
			RequestId: req.RequestId,
			Success:   false,
			Error:     fmt.Sprintf("Domain %s not connected", targetDomain),
		}, nil
	}

	// Create a pending request to wait for the response
	pendingReq := &PendingRequest{
		RequestID: req.RequestId,
		Response:  make(chan *RuntimeMessage, 1),
		Timeout:   time.Now().Add(30 * time.Second),
	}

	s.addPendingRequest(req.RequestId, pendingReq)
	defer s.removePendingRequest(req.RequestId)

	if err := stream.Send(&RuntimeMessage{
		Type:      messageType,
		Payload:   req.Payload,
		RequestId: req.RequestId,
		Success:   true,
	}); err != nil {
		log.Printf("Error sending to domain: %v", err)
		return &RuntimeMessage{
			Type:      "error",
			RequestId: req.RequestId,
			Success:   false,
			Error:     "Failed to send to domain",
		}, nil
	}

	log.Printf("Sent %s to domain %s, waiting for response...", messageType, targetDomain)

	// Wait for response with timeout
	select {
	case response := <-pendingReq.Response:
		log.Printf("Received response for request %s: success=%t", req.RequestId, response.Success)
		return response, nil
	case <-time.After(30 * time.Second):
		log.Printf("Timeout waiting for response to request %s", req.RequestId)
		return &RuntimeMessage{
			Type:      "error",
			RequestId: req.RequestId,
			Success:   false,
			Error:     "Request timeout",
		}, nil
	case <-ctx.Done():
		log.Printf("Context cancelled for request %s", req.RequestId)
		return &RuntimeMessage{
			Type:      "error",
			RequestId: req.RequestId,
			Success:   false,
			Error:     "Request cancelled",
		}, nil
	}
}

// Helper methods for managing domain streams
func (s *FrameworkServer) addDomainStream(domain string, stream FrameworkService_DomainCommunicationServer) {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()
	if s.domainStreams == nil {
		s.domainStreams = make(map[string]FrameworkService_DomainCommunicationServer)
	}
	s.domainStreams[domain] = stream
}

func (s *FrameworkServer) removeDomainStream(domain string) {
	s.streamMutex.Lock()
	defer s.streamMutex.Unlock()
	delete(s.domainStreams, domain)
}

func (s *FrameworkServer) getDomainStream(domain string) FrameworkService_DomainCommunicationServer {
	s.streamMutex.RLock()
	defer s.streamMutex.RUnlock()
	return s.domainStreams[domain]
}

// Helper methods for managing pending requests
func (s *FrameworkServer) addPendingRequest(requestID string, req *PendingRequest) {
	s.requestMutex.Lock()
	defer s.requestMutex.Unlock()
	if s.pendingRequests == nil {
		s.pendingRequests = make(map[string]*PendingRequest)
	}
	s.pendingRequests[requestID] = req
}

func (s *FrameworkServer) removePendingRequest(requestID string) {
	s.requestMutex.Lock()
	defer s.requestMutex.Unlock()
	delete(s.pendingRequests, requestID)
}

func (s *FrameworkServer) getPendingRequest(requestID string) *PendingRequest {
	s.requestMutex.RLock()
	defer s.requestMutex.RUnlock()
	return s.pendingRequests[requestID]
}

// Check if a message type is a response (ends with "_response")
func (s *FrameworkServer) isResponseMessage(msgType string) bool {
	return len(msgType) > 9 && msgType[len(msgType)-9:] == "_response"
}

// Handle responses from domains
func (s *FrameworkServer) handleDomainResponse(msg *DomainMessage) {
	// Find the pending request
	pendingReq := s.getPendingRequest(msg.RequestId)
	if pendingReq == nil {
		log.Printf("No pending request found for response: %s", msg.RequestId)
		return
	}

	// Parse the response payload to determine success
	var responseData map[string]interface{}
	success := true
	errorMsg := ""

	if err := json.Unmarshal([]byte(msg.Payload), &responseData); err == nil {
		if successVal, ok := responseData["success"].(bool); ok {
			success = successVal
		}
		if errVal, ok := responseData["error"].(string); ok {
			errorMsg = errVal
		}
	}

	// Create runtime message
	response := &RuntimeMessage{
		Type:      msg.Type,
		Payload:   msg.Payload,
		RequestId: msg.RequestId,
		Success:   success,
		Error:     errorMsg,
	}

	// Send response to waiting goroutine
	select {
	case pendingReq.Response <- response:
		log.Printf("Response sent for request %s", msg.RequestId)
	default:
		log.Printf("No one waiting for response %s", msg.RequestId)
	}
}

func (s *FrameworkServer) processMessage(msg *DomainMessage) *RuntimeMessage {
	// Handle framework-level messages (db, email, etc.)
	switch msg.Type {
	case "domain_register":
		// Handle domain registration
		log.Printf("Domain %s registered successfully", msg.Domain)
		return &RuntimeMessage{
			Type:      "register_success",
			Payload:   `{"status": "registered"}`,
			RequestId: msg.RequestId,
			Success:   true,
		}
	case "db_create":
		// Simulate database creation
		log.Printf("Creating database record for domain %s", msg.Domain)
		return &RuntimeMessage{
			Type:      "db_result",
			Payload:   `{"id": 123, "status": "created"}`,
			RequestId: msg.RequestId,
			Success:   true,
		}
	case "db_find":
		// Simulate database find
		log.Printf("Finding database records for domain %s", msg.Domain)
		return &RuntimeMessage{
			Type:      "db_result",
			Payload:   `{"records": [{"id": 123, "name": "Test User", "email": "test@example.com"}]}`,
			RequestId: msg.RequestId,
			Success:   true,
		}
	case "email_send":
		// Simulate email sending
		log.Printf("Sending email for domain %s", msg.Domain)
		return &RuntimeMessage{
			Type:      "email_result",
			Payload:   `{"status": "sent"}`,
			RequestId: msg.RequestId,
			Success:   true,
		}
	default:
		log.Printf("Unknown framework message type: %s", msg.Type)
		return &RuntimeMessage{
			Type:      "error",
			RequestId: msg.RequestId,
			Success:   false,
			Error:     fmt.Sprintf("Unknown message type: %s", msg.Type),
		}
	}
}

// Cleanup routine to remove expired pending requests
func (s *FrameworkServer) startCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.requestMutex.Lock()
			now := time.Now()
			for requestID, req := range s.pendingRequests {
				if now.After(req.Timeout) {
					log.Printf("Cleaning up expired request: %s", requestID)
					close(req.Response)
					delete(s.pendingRequests, requestID)
				}
			}
			s.requestMutex.Unlock()
		}
	}()
}

func Listen() *FrameworkServer {
	// Create listener
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create gRPC server
	server := grpc.NewServer()
	reflection.Register(server)

	// Create framework server
	frameworkServer := &FrameworkServer{
		domainStreams:   make(map[string]FrameworkService_DomainCommunicationServer),
		pendingRequests: make(map[string]*PendingRequest),
	}

	// Start cleanup routine
	frameworkServer.startCleanupRoutine()

	RegisterFrameworkServiceServer(server, frameworkServer)

	log.Println("gRPC server starting on :50051")

	// Start server
	if err := server.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	return frameworkServer
}
