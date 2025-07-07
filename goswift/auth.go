// go-swift/goswift/auth.go
package goswift

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt" // External dependency for password hashing
)

const (
	sessionCookieName = "goswift_session"
	sessionExpiry     = 24 * time.Hour // Sessions expire after 24 hours
)

// HashPassword generates a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash compares a plain password with its bcrypt hash.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Session represents a user session.
type Session struct {
	UserID    string
	ExpiresAt time.Time
}

// SessionManager handles session creation, storage, and retrieval.
// This is an in-memory implementation for simplicity.
// In a production app, use a persistent store (e.g., database, Redis).
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]Session // map[sessionID]Session
}

// NewSessionManager creates and initializes a new SessionManager.
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]Session),
	}
	// Start a goroutine to clean up expired sessions
	go sm.cleanupExpiredSessions()
	return sm
}

// GenerateSessionID generates a new random session ID.
func (sm *SessionManager) GenerateSessionID() (string, error) {
	b := make([]byte, 32) // 32 bytes for a strong session ID
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// CreateSession creates a new session for a user and returns its ID.
func (sm *SessionManager) CreateSession(userID string) (string, error) {
	sessionID, err := sm.GenerateSessionID()
	if err != nil {
		return "", err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[sessionID] = Session{
		UserID:    userID,
		ExpiresAt: time.Now().Add(sessionExpiry),
	}
	return sessionID, nil
}

// GetSession retrieves a session by its ID. Returns nil if not found or expired.
func (sm *SessionManager) GetSession(sessionID string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	session, ok := sm.sessions[sessionID]
	if !ok || session.ExpiresAt.Before(time.Now()) {
		return nil // Session not found or expired
	}
	return &session
}

// DeleteSession removes a session.
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// SetSessionCookie sets a session cookie in the HTTP response.
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  time.Now().Add(sessionExpiry),
		HttpOnly: true, // Prevent JavaScript access to the cookie
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the session cookie from the HTTP response.
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete the cookie
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})
}

// GetSessionIDFromRequest extracts the session ID from the request cookie.
func (sm *SessionManager) GetSessionIDFromRequest(r *http.Request) (string, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		if err == http.ErrNoCookie {
			return "", nil // No session cookie found
		}
		return "", fmt.Errorf("failed to get session cookie: %w", err)
	}
	return cookie.Value, nil
}

// cleanupExpiredSessions periodically removes expired sessions from memory.
func (sm *SessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(time.Minute) // Check every minute
	defer ticker.Stop()

	for range ticker.C {
		sm.mu.Lock()
		for sessionID, session := range sm.sessions {
			if session.ExpiresAt.Before(time.Now()) {
				delete(sm.sessions, sessionID)
			}
		}
		sm.mu.Unlock()
	}
}
