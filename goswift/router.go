// go-swift/goswift/router.go
package goswift

import (
	"fmt"    // Added: For fmt.Sprintf
	"regexp" // For regex matching
	"strings"
)

// route stores the handler, original pattern, and route-specific middleware.
type route struct {
	handler HandlerFunc
	pattern string // Original pattern like "/users/:id" or "/users/:id([0-9]+)"
	// Route-specific middleware
	before []MiddlewareFunc
	after  []MiddlewareFunc
	// Compiled regex for path constraints
	regex *regexp.Regexp
	// Names of path parameters, in order
	paramNames []string
}

// Router manages the routing logic for the GoSwift framework.
type Router struct {
	// routes maps HTTP methods to a map of path patterns to their handlers.
	// Example: {"GET": {"/users": route, "/users/:id": route}}
	routes map[string]map[string]route
}

// newRouter creates and initializes a new Router.
func newRouter() *Router {
	return &Router{
		routes: make(map[string]map[string]route),
	}
}

// RouteBuilder provides a fluent interface for adding route-specific middleware.
type RouteBuilder struct {
	router  *Router
	method  string
	path    string
	handler HandlerFunc
	beforeMW []MiddlewareFunc
	afterMW  []MiddlewareFunc
}

// AddRoute initiates a RouteBuilder for a new route.
func (r *Router) AddRoute(method, path string, handler HandlerFunc) *RouteBuilder {
	return &RouteBuilder{
		router:  r,
		method:  method,
		path:    path,
		handler: handler,
	}
}

// Before adds middleware to be run before the handler for this specific route.
func (rb *RouteBuilder) Before(mw ...MiddlewareFunc) *RouteBuilder {
	rb.beforeMW = append(rb.beforeMW, mw...)
	return rb
}

// After adds middleware to be run after the handler for this specific route.
func (rb *RouteBuilder) After(mw ...MiddlewareFunc) *RouteBuilder {
	rb.afterMW = append(rb.afterMW, mw...)
	return rb
}

// Handler finalizes the route registration. This method is implicitly called
// when the RouteBuilder is returned from Engine.GET/POST etc.
// It compiles regex and stores the route.
func (rb *RouteBuilder) Handler() {
	// Parse path for regex constraints and parameter names
	pattern := rb.path
	rePattern := "^"
	var paramNames []string

	parts := strings.Split(pattern, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ":") {
			// Extract parameter name and optional regex constraint
			paramName := part[1:]
			var constraint string
			if idx := strings.Index(paramName, "("); idx != -1 {
				constraint = paramName[idx:] // e.g., "([0-9]+)"
				paramName = paramName[:idx]  // e.g., "id"
			} else {
				constraint = "([^/]+)" // Default: match any non-slash characters
			}
			rePattern += "/" + constraint
			paramNames = append(paramNames, paramName)
		} else if part == "*" { // Wildcard
			rePattern += "/(.*)"
			paramNames = append(paramNames, "wildcard") // Name for wildcard capture
		} else if part != "" { // Static part
			rePattern += "/" + regexp.QuoteMeta(part)
		}
	}
	rePattern += "/?$" // Optional trailing slash

	compiledRegex, err := regexp.Compile(rePattern)
	if err != nil {
		// This should ideally be caught during development/testing
		panic(fmt.Sprintf("Invalid route regex pattern '%s': %v", pattern, err))
	}

	if rb.router.routes[rb.method] == nil {
		rb.router.routes[rb.method] = make(map[string]route)
	}

	rb.router.routes[rb.method][rb.path] = route{
		handler:    rb.handler,
		pattern:    rb.path,
		before:     rb.beforeMW,
		after:      rb.afterMW,
		regex:      compiledRegex,
		paramNames: paramNames,
	}
}

// MatchRoute attempts to find a matching handler for the given HTTP method and request path.
// It also extracts any path parameters.
func (r *Router) MatchRoute(method, requestPath string) (HandlerFunc, map[string]string) {
	methodRoutes, ok := r.routes[method]
	if !ok {
		return nil, nil // No routes for this method
	}

	var bestMatchHandler HandlerFunc
	var bestMatchParams map[string]string
	longestMatchLen := -1 // To prioritize more specific routes

	for pattern, rt := range methodRoutes {
		// Try regex match first
		matches := rt.regex.FindStringSubmatch(requestPath)
		if matches != nil {
			params := make(map[string]string)
			// matches[0] is the full string, subsequent are captures
			for i, name := range rt.paramNames {
				if i+1 < len(matches) { // Ensure there's a corresponding capture group
					params[name] = matches[i+1]
				}
			}

			// Prioritize longer, more specific matches (fewer wildcards/generic params)
			// Simple length check for now, a more robust system might count static segments.
			if len(pattern) > longestMatchLen {
				bestMatchHandler = rt.handler
				bestMatchParams = params
				longestMatchLen = len(pattern)

				// Apply route-specific middleware
				// The order is: beforeMW -> handler -> afterMW
				// Given applyMiddleware applies in reverse, to get: before -> handler -> after
				// We need to call applyMiddleware like this:
				// applyMiddleware(applyMiddleware(handler, rt.after...), rt.before...)

				chainedHandler := rt.handler
				// Apply 'after' middleware first (they will wrap the handler and run after it)
				chainedHandler = applyMiddleware(chainedHandler, rt.after...)
				// Apply 'before' middleware (they will wrap the 'after'-wrapped handler and run before it)
				chainedHandler = applyMiddleware(chainedHandler, rt.before...)
				bestMatchHandler = chainedHandler
			}
		}
	}

	return bestMatchHandler, bestMatchParams
}
