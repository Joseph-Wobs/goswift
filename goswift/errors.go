// go-swift/goswift/errors.go
package goswift

import (
	"fmt"
	"net/http"
)

// HTTPError is a custom error type for HTTP-related errors.
type HTTPError struct {
	StatusCode int
	Message    string
	Err        error // Original error, if any
}

// NewHTTPError creates a new HTTPError instance.
func NewHTTPError(statusCode int, message string, errs ...error) *HTTPError {
	var originalErr error
	if len(errs) > 0 {
		originalErr = errs[0]
	}
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
		Err:        originalErr,
	}
}

// Error implements the error interface for HTTPError.
func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("HTTP Error %d: %s (original error: %v)", e.StatusCode, e.Message, e.Err)
	}
	return fmt.Sprintf("HTTP Error %d: %s", e.StatusCode, e.Message)
}

// defaultErrorHandler is the default function for handling errors returned by handlers.
// It sends an appropriate HTTP response based on the error type.
func defaultErrorHandler(err error, c *Context) {
	var statusCode = http.StatusInternalServerError
	var message = "Internal Server Error"

	if httpErr, ok := err.(*HTTPError); ok {
		statusCode = httpErr.StatusCode
		message = httpErr.Message
		if httpErr.Err != nil {
			c.engine.Logger.Error("Handler error (HTTPError): %v", httpErr.Err)
		}
	} else {
		// Log unexpected errors
		c.engine.Logger.Error("Unhandled error in handler: %v", err)
	}

	// Attempt to send a JSON error response
	// If JSON encoding fails, fall back to plain text
	if err := c.JSON(statusCode, map[string]string{"error": message}); err != nil {
		c.engine.Logger.Error("Failed to send JSON error response: %v", err)
		c.String(statusCode, message) // Fallback to plain text
	}
}
