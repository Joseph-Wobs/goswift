// quikdocs/backend/main.go
package main

import (
	"embed" // For embedding static files
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"go-swift/goswift" // Assuming 'go-swift' is the module name
	"github.com/google/uuid" // For generating unique document IDs and public share IDs
)

//go:embed static/*
var embeddedFiles embed.FS // Embed the vanilla JS frontend from the 'static' directory

// --- In-Memory Data Stores (for simplicity) ---

// User represents a simple user structure for in-memory storage.
type User struct {
	ID             string
	Username       string
	HashedPassword string
}

// Document represents a QuikDocs document.
type Document struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	OwnerID   string `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Versions  []DocumentVersion `json:"versions"` // Simple version history
	ShareID   string `json:"share_id,omitempty"` // Public shareable ID
}

// DocumentVersion represents a snapshot of a document's content at a point in time.
type DocumentVersion struct {
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
}

// In-memory storage for users and documents.
var (
	inMemoryUsers = struct {
		sync.RWMutex
		data map[string]User // map[username]User
	}{
		data: make(map[string]User),
	}

	inMemoryDocuments = struct {
		sync.RWMutex
		data map[string]Document // map[documentID]Document
		// Map to quickly find document ID by share ID
		shareIDToDocID map[string]string // map[shareID]documentID
	}{
		data:           make(map[string]Document),
		shareIDToDocID: make(map[string]string),
	}
)

// --- Request Body Structs ---

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type DocumentCreateRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type DocumentUpdateRequest struct {
	Content string `json:"content"`
}

// --- Main Application ---

func main() {
	app := goswift.New()

	// Initialize SSE Manager
	sseManager := goswift.NewSSEManager(app.Logger)
	app.DI.Bind(sseManager) // Bind SSEManager to DI container

	// Global Middleware
	app.Use(goswift.RequestIDMiddleware())
	app.Use(goswift.LoggerMiddleware())
	app.Use(goswift.RecoveryMiddleware())
	app.Use(goswift.MetricsMiddleware(app.MetricsMan))
	app.Use(goswift.CORSMiddleware("*")) // Allow all origins for simplicity in development/production

	// --- Serve Frontend Static Files ---
	// This will serve the vanilla JS frontend from the 'static' directory.
	app.StaticFS("/", embeddedFiles)
	app.Logger.Info("Serving vanilla JS frontend from /")


	// --- Authentication Routes ---
	app.POST("/api/signup", func(c *goswift.Context) error {
		var req AuthRequest
		if err := c.BindJSON(&req); err != nil {
			return goswift.NewHTTPError(http.StatusBadRequest, "Invalid request payload")
		}

		if req.Username == "" || req.Password == "" {
			return goswift.NewHTTPError(http.StatusBadRequest, "Username and password are required")
		}

		inMemoryUsers.RLock()
		_, exists := inMemoryUsers.data[req.Username]
		inMemoryUsers.RUnlock()

		if exists {
			return goswift.NewHTTPError(http.StatusConflict, "Username already exists")
		}

		hashedPassword, err := goswift.HashPassword(req.Password)
		if err != nil {
			app.Logger.Error("Failed to hash password: %v", err)
			return goswift.NewHTTPError(http.StatusInternalServerError, "Failed to process registration")
		}

		userID := uuid.New().String()
		inMemoryUsers.Lock()
		inMemoryUsers.data[req.Username] = User{ID: userID, Username: req.Username, HashedPassword: hashedPassword}
		inMemoryUsers.Unlock()

		app.Logger.Info("User registered: %s (ID: %s)", req.Username, userID)
		return c.JSON(http.StatusCreated, map[string]string{"message": "User registered successfully"})
	}).Handler()

	app.POST("/api/login", func(c *goswift.Context) error {
		var req AuthRequest
		if err := c.BindJSON(&req); err != nil {
			return goswift.NewHTTPError(http.StatusBadRequest, "Invalid request payload")
		}

		inMemoryUsers.RLock()
		user, exists := inMemoryUsers.data[req.Username]
		inMemoryUsers.RUnlock()

		if !exists || !goswift.CheckPasswordHash(req.Password, user.HashedPassword) {
			return goswift.NewHTTPError(http.StatusUnauthorized, "Invalid username or password")
		}

		token, err := goswift.GenerateJWT(user.ID)
		if err != nil {
			app.Logger.Error("Failed to generate JWT for user %s: %v", user.ID, err)
			return goswift.NewHTTPError(http.StatusInternalServerError, "Failed to generate authentication token")
		}

		app.Logger.Info("User logged in: %s (ID: %s)", req.Username, user.ID)
		return c.JSON(http.StatusOK, map[string]string{"token": token, "user_id": user.ID, "username": user.Username})
	}).Handler()

	// --- Protected API Routes (Document CRUD) ---
	apiGroup := app.Group("/api")
	apiGroup.Use(goswift.JWTAuthMiddleware()) // Apply JWT authentication to all API routes

	// List user documents
	apiGroup.GET("/docs", func(c *goswift.Context) error {
		userID, _ := c.Get("userID") // JWTAuthMiddleware guarantees userID is present
		currentUserID := userID.(string)

		userDocs := []Document{}
		inMemoryDocuments.RLock()
		for _, doc := range inMemoryDocuments.data {
			if doc.OwnerID == currentUserID {
				// Exclude versions and shareID from list view for brevity
				doc.Versions = nil
				doc.ShareID = ""
				userDocs = append(userDocs, doc)
			}
		}
		inMemoryDocuments.RUnlock()

		return c.JSON(http.StatusOK, userDocs)
	}).Handler()

	// Create document
	apiGroup.POST("/docs", func(c *goswift.Context) error {
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		var req DocumentCreateRequest
		if err := c.BindJSON(&req); err != nil {
			return goswift.NewHTTPError(http.StatusBadRequest, "Invalid request payload")
		}

		if req.Title == "" {
			return goswift.NewHTTPError(http.StatusBadRequest, "Document title is required")
		}

		now := goswift.Now()
		newDoc := Document{
			ID:        uuid.New().String(),
			Title:     req.Title,
			Content:   req.Content,
			OwnerID:   currentUserID,
			CreatedAt: now,
			UpdatedAt: now,
			Versions:  []DocumentVersion{{Timestamp: now, Content: req.Content}},
		}

		inMemoryDocuments.Lock()
		inMemoryDocuments.data[newDoc.ID] = newDoc
		inMemoryDocuments.Unlock()

		app.Logger.Info("User %s created document: %s (ID: %s)", currentUserID, newDoc.Title, newDoc.ID)
		return c.JSON(http.StatusCreated, newDoc)
	}).Handler()

	// Read document
	apiGroup.GET("/docs/:id", func(c *goswift.Context) error {
		docID := c.Param("id")
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		inMemoryDocuments.RLock()
		doc, ok := inMemoryDocuments.data[docID]
		inMemoryDocuments.RUnlock()

		if !ok {
			return goswift.NewHTTPError(http.StatusNotFound, "Document not found")
		}
		if doc.OwnerID != currentUserID {
			return goswift.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		return c.JSON(http.StatusOK, doc)
	}).Handler()

	// Update document
	apiGroup.PUT("/docs/:id", func(c *goswift.Context) error {
		docID := c.Param("id")
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		var req DocumentUpdateRequest
		if err := c.BindJSON(&req); err != nil {
			return goswift.NewHTTPError(http.StatusBadRequest, "Invalid request payload")
		}

		inMemoryDocuments.Lock()
		doc, ok := inMemoryDocuments.data[docID]
		if !ok {
			inMemoryDocuments.Unlock()
			return goswift.NewHTTPError(http.StatusNotFound, "Document not found")
		}
		if doc.OwnerID != currentUserID {
			inMemoryDocuments.Unlock()
			return goswift.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		// Update content and add a new version
		doc.Content = req.Content
		doc.UpdatedAt = goswift.Now()
		doc.Versions = append(doc.Versions, DocumentVersion{Timestamp: doc.UpdatedAt, Content: req.Content})
		inMemoryDocuments.data[doc.ID] = doc // Update the map entry
		inMemoryDocuments.Unlock()

		// Broadcast update to all subscribed clients for this document
		sseManager.Broadcast(docID, req.Content)
		app.Logger.Info("User %s updated document %s (ID: %s). Broadcasted update.", currentUserID, doc.Title, doc.ID)

		return c.JSON(http.StatusOK, doc)
	}).Handler()

	// Delete document
	apiGroup.DELETE("/docs/:id", func(c *goswift.Context) error {
		docID := c.Param("id")
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		inMemoryDocuments.Lock()
		doc, ok := inMemoryDocuments.data[docID]
		if !ok {
			inMemoryDocuments.Unlock()
			return goswift.NewHTTPError(http.StatusNotFound, "Document not found")
		}
		if doc.OwnerID != currentUserID {
			inMemoryDocuments.Unlock()
			return goswift.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		delete(inMemoryDocuments.data, docID)
		// Also remove from shareID map if it exists
		if doc.ShareID != "" {
			delete(inMemoryDocuments.shareIDToDocID, doc.ShareID)
		}
		inMemoryDocuments.Unlock()

		app.Logger.Info("User %s deleted document: %s (ID: %s)", currentUserID, doc.Title, doc.ID)
		return c.NoContent(http.StatusNoContent)
	}).Handler()

	// Get document version history
	apiGroup.GET("/docs/:id/history", func(c *goswift.Context) error {
		docID := c.Param("id")
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		inMemoryDocuments.RLock()
		doc, ok := inMemoryDocuments.data[docID]
		inMemoryDocuments.RUnlock()

		if !ok {
			return goswift.NewHTTPError(http.StatusNotFound, "Document not found")
		}
		if doc.OwnerID != currentUserID {
			return goswift.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		return c.JSON(http.StatusOK, doc.Versions)
	}).Handler()

	// --- Real-time Sync (SSE) ---
	apiGroup.GET("/docs/:id/subscribe", func(c *goswift.Context) error {
		docID := c.Param("id")
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		inMemoryDocuments.RLock()
		doc, ok := inMemoryDocuments.data[docID]
		inMemoryDocuments.RUnlock()

		if !ok {
			return goswift.NewHTTPError(http.StatusNotFound, "Document not found")
		}
		if doc.OwnerID != currentUserID {
			return goswift.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		// Add the client to the SSE manager
		sseManager.AddClient(docID, currentUserID, c)

		// The SSEManager takes over the response writing, so we return nil.
		return nil
	}).Handler()

	// --- Shareable Public Link ---
	apiGroup.POST("/docs/:id/share", func(c *goswift.Context) error {
		docID := c.Param("id")
		userID, _ := c.Get("userID")
		currentUserID := userID.(string)

		inMemoryDocuments.Lock()
		doc, ok := inMemoryDocuments.data[docID]
		if !ok {
			inMemoryDocuments.Unlock()
			return goswift.NewHTTPError(http.StatusNotFound, "Document not found")
		}
		if doc.OwnerID != currentUserID {
			inMemoryDocuments.Unlock()
			return goswift.NewHTTPError(http.StatusForbidden, "Access denied")
		}

		if doc.ShareID == "" {
			shareID := uuid.New().String() // Generate a new shareable ID
			doc.ShareID = shareID
			inMemoryDocuments.data[doc.ID] = doc // Update the document in map
			inMemoryDocuments.shareIDToDocID[shareID] = docID // Map share ID to doc ID
		}
		inMemoryDocuments.Unlock()

		shareLink := fmt.Sprintf("/share/%s", doc.ShareID)
		return c.JSON(http.StatusOK, map[string]string{"share_link": shareLink})
	}).Handler()

	// Public view for shared documents (no auth required)
	app.GET("/share/:shareID", func(c *goswift.Context) error {
		shareID := c.Param("shareID")

		inMemoryDocuments.RLock()
		docID, ok := inMemoryDocuments.shareIDToDocID[shareID]
		if !ok {
			inMemoryDocuments.RUnlock()
			return goswift.NewHTTPError(http.StatusNotFound, "Shared document not found")
		}
		doc, ok := inMemoryDocuments.data[docID]
		inMemoryDocuments.RUnlock()

		if !ok { // Should not happen if shareIDToDocID is consistent
			return goswift.NewHTTPError(http.StatusInternalServerError, "Shared document not found (internal error)")
		}

		// Render a simple HTML page for public viewing
		htmlContent := fmt.Sprintf(`
			<!DOCTYPE html>
			<html lang="en">
			<head>
				<meta charset="UTF-8">
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
				<title>%s - QuikDocs Shared</title>
				<link href="https://cdn.jsdelivr.net/npm/tailwindcss@2.2.19/dist/tailwind.min.css" rel="stylesheet">
				<style>
					body { font-family: 'Inter', sans-serif; }
				</style>
			</head>
			<body class="bg-gray-100 p-4">
				<div class="max-w-3xl mx-auto bg-white p-6 rounded-lg shadow-md">
					<h1 class="text-3xl font-bold text-gray-800 mb-4">%s</h1>
					<p class="text-sm text-gray-500 mb-6">Shared by owner. Last updated: %s</p>
					<div class="prose max-w-none border border-gray-200 p-4 rounded-md bg-gray-50 overflow-auto" style="min-height: 200px;">
						<pre class="whitespace-pre-wrap font-mono text-gray-700">%s</pre>
					</div>
					<div class="mt-6 text-center">
						<a href="/" class="inline-block bg-blue-500 hover:bg-blue-600 text-white font-bold py-2 px-4 rounded-lg transition duration-300">Go to QuikDocs Home</a>
					</div>
				</div>
			</body>
			</html>
		`, doc.Title, doc.Title, doc.UpdatedAt.Format("Jan 2, 2006 15:04"), doc.Content)

		return c.HTML(http.StatusOK, htmlContent)
	}).Handler()


	// --- Start the server ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("QuikDocs GoSwift backend starting on :%s", port)
	if err := app.Run(":" + port); err != nil {
		app.Logger.Error("Server failed to start: %v", err)
		log.Fatalf("Server failed to start: %v", err)
	}
}
