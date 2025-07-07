// go-swift/goswift/goswift.go
package goswift

import (
	"context" // For graceful shutdown
	"fmt"     // For error messages
	"io/fs"   // Corrected: Explicitly import io/fs for StaticFS signature
	"net/http"
	"os"     // For signal handling
	"os/signal" // For signal handling
	"path/filepath" // For Static file serving
	"strings" // For Static file serving
	"syscall" // For signal handling
	"time"
)

// HandlerFunc defines the signature for GoSwift route handlers.
type HandlerFunc func(c *Context) error

// Engine is the core application instance for GoSwift.
type Engine struct {
	router     *Router
	middleware []MiddlewareFunc
	Config     *ConfigManager
	Logger     *Logger
	SessionMan *SessionManager
	MetricsMan *MetricsManager
	Plugins    *PluginRegistry // New: Plugin Registry
	DI         *Container      // New: Dependency Injection Container
	TaskQueue  *AsyncTaskQueue // New: Asynchronous Task Queue
	// Custom error handler for the engine
	errorHandler func(err error, c *Context)
	// New: HTTP server instance for graceful shutdown
	httpServer *http.Server
}

// New creates and initializes a new GoSwift Engine.
func New() *Engine {
	e := &Engine{
		router:       newRouter(),
		middleware:   make([]MiddlewareFunc, 0),
		Config:       NewConfigManager(),
		Logger:       NewLogger(),
		SessionMan:   NewSessionManager(),
		MetricsMan:   NewMetricsManager(),
		Plugins:      NewPluginRegistry(), // Initialize Plugin Registry
		DI:           NewContainer(),      // Initialize DI Container
		TaskQueue:    NewAsyncTaskQueue(5), // Initialize Task Queue with 5 workers
		errorHandler: defaultErrorHandler, // Set default error handler
	}
	// Initialize http.Server here, but assign Handler later in Run
	e.httpServer = &http.Server{
		Handler: e, // The Engine itself implements http.Handler
	}
	return e
}

// Use registers global middleware for the Engine.
func (e *Engine) Use(mw MiddlewareFunc) {
	e.middleware = append(e.middleware, mw)
}

// SetErrorHandler allows customizing the global error handling logic.
func (e *Engine) SetErrorHandler(handler func(err error, c *Context)) {
	e.errorHandler = handler
}

// GET registers a GET route and its handler.
func (e *Engine) GET(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodGet, path, handler)
}

// POST registers a POST route and its handler.
func (e *Engine) POST(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodPost, path, handler)
}

// PUT registers a PUT route and its handler.
func (e *Engine) PUT(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodPut, path, handler)
}

// DELETE registers a DELETE route and its handler.
func (e *Engine) DELETE(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodDelete, path, handler)
}

// PATCH registers a PATCH route and its handler.
func (e *Engine) PATCH(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodPatch, path, handler)
}

// OPTIONS registers an OPTIONS route and its handler.
func (e *Engine) OPTIONS(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodOptions, path, handler)
}

// HEAD registers a HEAD route and its handler.
func (e *Engine) HEAD(path string, handler HandlerFunc) *RouteBuilder {
	return e.router.AddRoute(http.MethodHead, path, handler)
}

// Static serves static files from the given local directory under the specified URL prefix.
func (e *Engine) Static(urlPrefix, localDir string) {
	// Ensure the prefix ends with a slash for http.StripPrefix
	if !strings.HasSuffix(urlPrefix, "/") {
		urlPrefix += "/"
	}
	// Ensure the local directory path is clean
	localDir = filepath.Clean(localDir)

	// Create a file server handler
	fileServer := http.FileServer(http.Dir(localDir))

	// Register a GET route for the specified prefix with a wildcard
	e.GET(urlPrefix+"*", func(c *Context) error {
		// Strip the prefix from the request URL path before serving the file
		// This is crucial for http.FileServer to correctly locate the file
		c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, urlPrefix)
		fileServer.ServeHTTP(c.Writer, c.Request)
		return nil // FileServer handles the response, so no error is returned here
	}).Handler() // Call Handler() to finalize route registration
	e.Logger.Info("Serving static files from '%s' under URL prefix '%s'", localDir, urlPrefix)
}

// StaticFS serves static files from an embedded file system (Go 1.16+ embed).
func (e *Engine) StaticFS(urlPrefix string, fsys fs.FS) { // Corrected: Changed embed.FS to fs.FS
	if !strings.HasSuffix(urlPrefix, "/") {
		urlPrefix += "/"
	}

	// Create a file server from the embedded file system
	fileServer := http.FileServer(http.FS(fsys)) // http.FS expects fs.FS

	e.GET(urlPrefix+"*", func(c *Context) error {
		// Strip the prefix from the request URL path
		c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, urlPrefix)
		fileServer.ServeHTTP(c.Writer, c.Request)
		return nil
	}).Handler() // Call Handler() to finalize route registration
	e.Logger.Info("Serving embedded static files under URL prefix '%s'", urlPrefix)
}


// ServeHTTP implements the http.Handler interface for the Engine.
// It dispatches requests to the appropriate handler after applying middleware.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create a new context for the request
	c := newContext(w, r)
	c.engine = e // Provide access to the engine (for logger, config, session manager etc.)

	// Find the handler and path parameters
	handler, params := e.router.MatchRoute(r.Method, r.URL.Path)

	// Set path parameters in the context
	c.SetPathParams(params)

	// If no handler is found, return a 404 Not Found error
	if handler == nil {
		e.errorHandler(NewHTTPError(http.StatusNotFound, "Not Found"), c)
		return
	}

	// Chain global middleware and execute the handler
	finalHandler := applyMiddleware(handler, e.middleware...)

	// Execute the chained handler and handle any returned errors
	if err := finalHandler(c); err != nil {
		e.errorHandler(err, c)
	}
}

// Run starts the HTTP server on the specified address with graceful shutdown.
func (e *Engine) Run(addr string) error {
	e.httpServer.Addr = addr
	e.Logger.Info("GoSwift server listening on %s", addr)

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM) // Listen for Ctrl+C and kill signals

	go func() {
		<-quit // Block until a signal is received
		e.Logger.Info("Shutting down server...")

		// Create a context with a timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // 10-second graceful timeout
		defer cancel()

		// Shutdown the HTTP server
		if err := e.httpServer.Shutdown(ctx); err != nil {
			e.Logger.Error("Server shutdown failed: %v", err)
		}

		// Shutdown the TaskQueue, waiting for ongoing tasks to complete
		e.Logger.Info("Shutting down task queue...")
		e.TaskQueue.Shutdown()
		e.Logger.Info("Task queue shut down.")

		e.Logger.Info("Server exited gracefully.")
	}()

	// Start the HTTP server
	if err := e.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed to start: %w", err)
	}
	return nil
}

// Now returns the current time, useful for consistent time reporting.
func Now() time.Time {
	return time.Now()
}

// RouterGroup allows for grouping routes with a common prefix and middleware.
type RouterGroup struct {
	engine     *Engine
	prefix     string
	middleware []MiddlewareFunc
}

// Group creates a new RouterGroup with the given prefix.
func (e *Engine) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		engine: e,
		prefix: prefix,
	}
}

// Use applies middleware to the RouterGroup.
func (rg *RouterGroup) Use(mw MiddlewareFunc) {
	rg.middleware = append(rg.middleware, mw)
}

// GET registers a GET route within the group.
func (rg *RouterGroup) GET(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodGet, fullPath, handler).Before(rg.middleware...)
}

// POST registers a POST route within the group.
func (rg *RouterGroup) POST(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodPost, fullPath, handler).Before(rg.middleware...)
}

// PUT registers a PUT route within the group.
func (rg *RouterGroup) PUT(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodPut, fullPath, handler).Before(rg.middleware...)
}

// DELETE registers a DELETE route within the group.
func (rg *RouterGroup) DELETE(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodDelete, fullPath, handler).Before(rg.middleware...)
}

// PATCH registers a PATCH route within the group.
func (rg *RouterGroup) PATCH(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodPatch, fullPath, handler).Before(rg.middleware...)
}

// OPTIONS registers an OPTIONS route within the group.
func (rg *RouterGroup) OPTIONS(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodOptions, fullPath, handler).Before(rg.middleware...)
}

// HEAD registers a HEAD route within the group.
func (rg *RouterGroup) HEAD(path string, handler HandlerFunc) *RouteBuilder {
	fullPath := rg.prefix + path
	return rg.engine.router.AddRoute(http.MethodHead, fullPath, handler).Before(rg.middleware...)
}
