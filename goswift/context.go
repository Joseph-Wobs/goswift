// go-swift/goswift/context.go
package goswift

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Context holds request-scoped information and provides convenience methods.
type Context struct {
	Writer *responseWriter // Use custom response writer to capture status code
	Request *http.Request
	// Stores path parameters extracted by the router
	pathParams map[string]string
	// Stores request-scoped data
	data map[string]interface{}
	mu   sync.RWMutex // Mutex for data map access
	// Reference to the engine for accessing logger, config, etc.
	engine *Engine
}

// newContext creates a new Context for a given HTTP request and response.
func newContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{
		Writer:     &responseWriter{ResponseWriter: w}, // Wrap original writer
		Request:    r,
		pathParams: make(map[string]string),
		data:       make(map[string]interface{}),
	}
}

// responseWriter is a wrapper around http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// Status returns the HTTP status code written to the response.
func (c *Context) Status() int {
	if c.Writer.status == 0 {
		return http.StatusOK // Default to 200 if WriteHeader hasn't been called
	}
	return c.Writer.status
}

// SetPathParams sets the path parameters extracted by the router.
func (c *Context) SetPathParams(params map[string]string) {
	c.pathParams = params
}

// Param returns the value of a path parameter by name.
func (c *Context) Param(key string) string {
	return c.pathParams[key]
}

// Query returns the value of a URL query parameter by name.
func (c *Context) Query(key string) string {
	return c.Request.URL.Query().Get(key)
}

// QueryParams returns all URL query parameters as a url.Values map.
func (c *Context) QueryParams() url.Values {
	return c.Request.URL.Query()
}

// FormValue returns the string value of a form field from a POST, PUT, or PATCH request.
// It calls Request.ParseForm internally if needed.
func (c *Context) FormValue(key string) string {
	return c.Request.FormValue(key)
}

// ValidateRequired checks if the specified form fields are non-empty.
// Returns an error message if any field is empty, otherwise an empty string.
func (c *Context) ValidateRequired(fields ...string) string {
	for _, field := range fields {
		if c.FormValue(field) == "" {
			return fmt.Sprintf("'%s' is a required field.", field)
		}
	}
	return "" // No errors
}

// BindForm binds application/x-www-form-urlencoded or multipart/form-data
// from the request body into the provided struct using 'form' tags.
// Supports string, int, float64, and bool types.
func (c *Context) BindForm(v interface{}) error {
	if err := c.Request.ParseForm(); err != nil {
		return fmt.Errorf("failed to parse form data: %w", err)
	}

	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("BindForm expects a pointer to a struct")
	}

	elem := val.Elem()
	typ := elem.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		formTag := field.Tag.Get("form")
		if formTag == "" {
			continue // Skip fields without a 'form' tag
		}

		formValue := c.Request.Form.Get(formTag)
		if formValue == "" {
			continue // Skip if form value is empty
		}

		fieldVal := elem.Field(i)
		if !fieldVal.CanSet() {
			continue // Skip unexported fields
		}

		switch field.Type.Kind() {
		case reflect.String:
			fieldVal.SetString(formValue)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if intVal, err := strconv.ParseInt(formValue, 10, field.Type.Bits()); err == nil {
				fieldVal.SetInt(intVal)
			} else {
				return fmt.Errorf("failed to parse int for field '%s': %w", formTag, err)
			}
		case reflect.Float32, reflect.Float64:
			if floatVal, err := strconv.ParseFloat(formValue, field.Type.Bits()); err == nil {
				fieldVal.SetFloat(floatVal)
			} else {
				return fmt.Errorf("failed to parse float for field '%s': %w", formTag, err)
			}
		case reflect.Bool:
			// Handle common boolean strings
			if boolVal, err := strconv.ParseBool(formValue); err == nil {
				fieldVal.SetBool(boolVal)
			} else if strings.ToLower(formValue) == "on" || strings.ToLower(formValue) == "true" || formValue == "1" {
				fieldVal.SetBool(true)
			} else if strings.ToLower(formValue) == "off" || strings.ToLower(formValue) == "false" || formValue == "0" {
				fieldVal.SetBool(false)
			} else {
				return fmt.Errorf("failed to parse bool for field '%s': invalid value '%s'", formTag, formValue)
			}
		default:
			// For simplicity, skip unsupported types or return an error
			// return fmt.Errorf("unsupported field type for BindForm: %s", field.Type.Kind())
			c.engine.Logger.Warning("BindForm skipping unsupported field type %s for field %s", field.Type.Kind(), formTag)
		}
	}
	return nil
}

// BindJSON binds the request body (assuming JSON) into the provided interface.
func (c *Context) BindJSON(v interface{}) error {
	if c.Request.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer c.Request.Body.Close()
	return json.NewDecoder(c.Request.Body).Decode(v)
}

// Set sets a key-value pair in the request-scoped data.
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Get retrieves a value from the request-scoped data by key.
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

// TraceID retrieves the trace ID from the request-scoped data.
// It returns an empty string if no trace ID is set.
func (c *Context) TraceID() string {
	if id, ok := c.Get("traceID"); ok {
		if traceIDStr, isString := id.(string); isString {
			return traceIDStr
		}
	}
	return ""
}

// JSON sends a JSON response with the given status code and data.
func (c *Context) JSON(statusCode int, data interface{}) error {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(statusCode)
	return json.NewEncoder(c.Writer).Encode(data)
}

// String sends a plain text response with the given status code and format.
func (c *Context) String(statusCode int, format string, args ...interface{}) error {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.WriteHeader(statusCode)
	_, err := fmt.Fprintf(c.Writer, format, args...)
	return err
}

// HTML sends an HTML response with the given status code and HTML string.
func (c *Context) HTML(statusCode int, html string) error {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(statusCode)
	_, err := c.Writer.Write([]byte(html))
	return err
}

// NoContent sends a response with only the status code and no body.
func (c *Context) NoContent(statusCode int) error {
	c.Writer.WriteHeader(statusCode)
	return nil
}

// File sends a file as a download.
func (c *Context) File(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to open file: %v", err))
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("Failed to get file info: %v", err))
	}

	fileName := filepath.Base(filePath)
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	c.Writer.Header().Set("Content-Type", "application/octet-stream") // Generic binary stream
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	c.Writer.WriteHeader(http.StatusOK)
	_, err = io.Copy(c.Writer, file)
	return err
}

// Redirect redirects the client to a new URL with the given status code.
func (c *Context) Redirect(statusCode int, url string) {
	http.Redirect(c.Writer, c.Request, url, statusCode)
}
