// quikdocs/backend/goswift/sse.go
package goswift

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEClient represents a single SSE client connection.
type SSEClient struct {
	ID   string
	Chan chan string // Channel to send messages to this client
}

// SSEManager manages multiple SSE connections and broadcasts messages.
type SSEManager struct {
	mu      sync.RWMutex
	clients map[string]map[string]SSEClient // map[documentID]map[clientID]SSEClient
	Logger  *Logger
}

// NewSSEManager creates and initializes a new SSEManager.
func NewSSEManager(logger *Logger) *SSEManager {
	return &SSEManager{
		clients: make(map[string]map[string]SSEClient),
		Logger:  logger,
	}
}

// AddClient adds a new SSE client for a specific document.
func (sm *SSEManager) AddClient(docID, clientID string, c *Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.clients[docID]; !ok {
		sm.clients[docID] = make(map[string]SSEClient)
	}

	clientChan := make(chan string, 5) // Buffered channel for messages
	sm.clients[docID][clientID] = SSEClient{ID: clientID, Chan: clientChan}

	sm.Logger.Info("SSE: Client %s connected for document %s", clientID, docID)

	// Set necessary headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*") // Important for CORS with SSE

	// Flush the headers immediately
	flusher, ok := c.Writer.ResponseWriter.(http.Flusher)
	if !ok {
		sm.Logger.Error("SSE: Streaming not supported by client writer.")
		return
	}

	// Keep connection alive by sending comments or heartbeats
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Send a heartbeat every 30 seconds
		defer ticker.Stop()
		for {
			select {
			case msg := <-clientChan:
				// Write the message to the client
				fmt.Fprintf(c.Writer, "data: %s\n\n", msg)
				flusher.Flush()
			case <-ticker.C:
				// Send a heartbeat comment
				fmt.Fprintf(c.Writer, ": heartbeat\n\n")
				flusher.Flush()
			case <-c.Request.Context().Done():
				// Client disconnected
				sm.RemoveClient(docID, clientID)
				sm.Logger.Info("SSE: Client %s disconnected from document %s", clientID, docID)
				return
			}
		}
	}()
}

// RemoveClient removes an SSE client connection.
func (sm *SSEManager) RemoveClient(docID, clientID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if docClients, ok := sm.clients[docID]; ok {
		if client, ok := docClients[clientID]; ok {
			close(client.Chan) // Close the client's channel
			delete(docClients, clientID)
			if len(docClients) == 0 {
				delete(sm.clients, docID) // Clean up document entry if no more clients
			}
		}
	}
}

// Broadcast sends a message to all clients subscribed to a specific document.
func (sm *SSEManager) Broadcast(docID string, message string) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if docClients, ok := sm.clients[docID]; ok {
		for _, client := range docClients {
			select {
			case client.Chan <- message:
				// Message sent successfully
			default:
				// Client channel is blocked, maybe it's slow or disconnected.
				// Log and consider removing the client if this happens often.
				sm.Logger.Warning("SSE: Failed to send message to client %s for document %s (channel full)", client.ID, docID)
			}
		}
	}
}
