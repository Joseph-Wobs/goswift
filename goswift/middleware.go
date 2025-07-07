// quikdocs/backend/goswift/middleware.go
package goswift

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httputil" // For HTTP Proxy
	"net/url"           // For HTTP Proxy
	"runtime/debug"
	"strings"
	"time"
)

// MiddlewareFunc defines the signature for GoSwift middleware.
// It takes the next HandlerFunc in the chain and returns a new HandlerFunc.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// applyMiddleware chains multiple middleware functions together with the final handler.
func applyMiddleware(handler HandlerFunc, middleware ...MiddlewareFunc) HandlerFunc {
	// Apply middleware in reverse order
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}

// LoggerMiddleware is a simple middleware that logs incoming requests.
func LoggerMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			start := time.Now()
			err := next(c) // Call the next handler in the chain
			duration := time.Since(start)

			statusCode := c.Status() // Get actual status code from custom writer

			// Get Request ID and Trace ID if set by RequestIDMiddleware
			requestID, _ := c.Get("requestID")
			traceID := c.TraceID() // Use the new TraceID() helper

			logPrefix := ""
			if traceID != "" {
				logPrefix = fmt.Sprintf("[%s]", traceID)
			} else if requestIDStr, ok := requestID.(string); ok && requestIDStr != "" {
				logPrefix = fmt.Sprintf("[%s]", requestIDStr)
			}

			c.engine.Logger.Info("%s %s %s %s - %d %s",
				logPrefix, c.Request.Method, c.Request.URL.Path, c.Request.RemoteAddr, statusCode, duration)
			return err
		}
	}
}

// RecoveryMiddleware is a middleware that recovers from panics and logs them.
func RecoveryMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					// Log the panic
					c.engine.Logger.Error("Panic recovered: %v\n%s", r, debug.Stack())
					// Return a 500 Internal Server Error
					err = NewHTTPError(http.StatusInternalServerError, "Internal Server Error")
				}
			}()
			return next(c)
		}
	}
}

// AuthMiddleware checks for a valid session and sets the authenticated user ID in the context.
// If no valid session is found, it redirects to the login page.
func AuthMiddleware(sessionManager *SessionManager, redirectPath string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			sessionID, err := sessionManager.GetSessionIDFromRequest(c.Request)
			if err != nil {
				c.engine.Logger.Error("AuthMiddleware: Error getting session ID from request: %v", err)
				sessionManager.ClearSessionCookie(c.Writer) // Clear potentially bad cookie
				c.Redirect(http.StatusFound, redirectPath)
				return nil // Response handled by redirect
			}

			session := sessionManager.GetSession(sessionID)
			if session == nil {
				// No valid session, redirect to login
				sessionManager.ClearSessionCookie(c.Writer) // Ensure old/expired cookie is cleared
				c.Redirect(http.StatusFound, redirectPath)
				return nil // Response handled by redirect
			}

			// Session is valid, store UserID in context for handler access
			c.Set("userID", session.UserID)
			return next(c) // Continue to the next handler
		}
	}
}

// JWTAuthMiddleware validates a JWT from the Authorization header and sets the UserID in context.
func JWTAuthMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			authHeader := c.Request.Header.Get("Authorization")
			if authHeader == "" {
				return NewHTTPError(http.StatusUnauthorized, "Authorization header required")
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return NewHTTPError(http.StatusUnauthorized, "Authorization header must be in 'Bearer <token>' format")
			}

			tokenString := parts[1]
			claims, err := ValidateJWT(tokenString)
			if err != nil {
				c.engine.Logger.Warning("JWT validation failed: %v", err)
				return NewHTTPError(http.StatusUnauthorized, "Invalid or expired token")
			}

			// Token is valid, set UserID in context
			c.Set("userID", claims.UserID)
			return next(c)
		}
	}
}


// TimeoutMiddleware sets a request timeout. If the handler exceeds the timeout, a 504 is returned.
func TimeoutMiddleware(timeout time.Duration) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
			defer cancel() // Ensure the context is cancelled to release resources

			// Create a new request with the timeout context
			c.Request = c.Request.WithContext(ctx)

			// Channel to signal when the handler has completed
			done := make(chan error, 1)

			go func() {
				// Execute the next handler in a goroutine
				done <- next(c)
			}()

			select {
			case err := <-done:
				// Handler completed within the timeout
				return err
			case <-ctx.Done():
				// Timeout occurred
				if ctx.Err() == context.DeadlineExceeded {
					c.engine.Logger.Warning("Request to %s timed out after %s", c.Request.URL.Path, timeout)
					return NewHTTPError(http.StatusGatewayTimeout, fmt.Sprintf("Request timed out after %s", timeout))
				}
				// Other context errors (e.g., context cancelled manually)
				return NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Request context error: %v", ctx.Err()))
			}
		}
	}
}

// BasicAuth authenticates requests using HTTP Basic Authentication.
// It takes a username and password and returns a HandlerFunc if authentication succeeds,
// otherwise it returns a 401 Unauthorized response.
func BasicAuth(expectedUsername, expectedPassword string, realm string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			user, pass, ok := c.Request.BasicAuth()
			if !ok || user != expectedUsername || pass != expectedPassword {
				c.Writer.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, realm))
				return NewHTTPError(http.StatusUnauthorized, "Unauthorized")
			}
			return next(c)
		}
	}
}

// CORSMiddleware provides Cross-Origin Resource Sharing (CORS) support.
// allowedOrigins can be "*" for any origin, or a comma-separated list of specific origins.
func CORSMiddleware(allowedOrigins string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			origin := c.Request.Header.Get("Origin")
			if origin == "" {
				return next(c) // Not a CORS request
			}

			// Set allowed origin
			if allowedOrigins == "*" || strings.Contains(allowedOrigins, origin) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			} else {
				// If origin is not allowed, do not set ACAO header, which will result in CORS error
				return next(c)
			}

			// Handle preflight OPTIONS requests
			if c.Request.Method == http.MethodOptions {
				c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID") // Add X-Request-ID
				c.Writer.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight for 24 hours
				c.Writer.WriteHeader(http.StatusNoContent)
				return nil // Preflight handled
			}

			return next(c)
		}
	}
}

// RequestIDMiddleware generates a unique request ID and attaches it to the request context and response header.
// It also checks for X-Trace-ID and uses it as the trace ID if present.
func RequestIDMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			requestID := c.Request.Header.Get("X-Request-ID")
			traceID := c.Request.Header.Get("X-Trace-ID") // Check for existing trace ID

			if requestID == "" {
				// Generate a new unique ID if not provided by client
				b := make([]byte, 16) // 16 bytes for a reasonably unique ID
				if _, err := rand.Read(b); err != nil {
					c.engine.Logger.Error("Failed to generate request ID: %v", err)
					requestID = fmt.Sprintf("gen-err-%d", time.Now().UnixNano()) // Fallback
				} else {
					requestID = base64.URLEncoding.EncodeToString(b)
				}
			}

			if traceID == "" {
				traceID = requestID // If no trace ID, use request ID as trace ID
			}

			// Set the Request ID in the context for handler access
			c.Set("requestID", requestID)
			// Set the Trace ID in the context for handler access
			c.Set("traceID", traceID)

			// Set the Request ID and Trace ID in the response headers
			c.Writer.Header().Set("X-Request-ID", requestID)
			c.Writer.Header().Set("X-Trace-ID", traceID)

			return next(c)
		}
	}
}

// MetricsMiddleware records request count and response time for each route.
func MetricsMiddleware(metricsMan *MetricsManager) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			start := time.Now()
			err := next(c) // Execute the next handler

			// Only record metrics if the request was successfully handled by a route
			// and not, for example, a 404 handled by the error handler before a route match.
			// A more robust solution might involve capturing the matched route pattern earlier.
			if c.Status() != http.StatusNotFound { // Simple check to avoid recording 404s as route metrics
				duration := time.Since(start)
				routePath := c.Request.URL.Path // Using full path for simplicity, could be matched pattern
				metricsMan.RecordRequest(routePath, duration)
			}
			return err
		}
	}
}

// Proxy creates a middleware that proxies requests to a target URL.
func Proxy(targetURL string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc { // next is ignored as proxy is a terminal handler
		return func(c *Context) error {
			remote, err := url.Parse(targetURL)
			if err != nil {
				c.engine.Logger.Error("Proxy middleware: invalid target URL '%s': %v", targetURL, err)
				return NewHTTPError(http.StatusInternalServerError, "Bad proxy configuration")
			}

			proxy := httputil.NewSingleHostReverseProxy(remote)

			// Modify the request before sending it to the target
			proxy.Director = func(req *http.Request) {
				req.Header.Add("X-Forwarded-For", req.RemoteAddr)
				req.Header.Add("X-Origin-Host", req.Host)
				req.URL.Scheme = remote.Scheme
				req.URL.Host = remote.Host
				req.Host = remote.Host // Important for target server's Host header
			}

			// Handle potential proxy errors
			proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
				c.engine.Logger.Error("Proxy error for %s %s: %v", req.Method, req.URL.Path, err)
				c.Writer.WriteHeader(http.StatusBadGateway)
				c.Writer.Write([]byte("Bad Gateway"))
			}

			proxy.ServeHTTP(c.Writer, c.Request)
			return nil // Proxy handles the response
		}
	}
}
