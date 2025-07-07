// go-swift/goswift/debug.go
package goswift

import (
	"fmt"
	"net/http"
	"reflect" // Added for getFunctionName
	"runtime"
	"runtime/pprof"
	"time"
)

// DebugRoutesHandler exposes all registered routes in JSON format.
func DebugRoutesHandler(c *Context) error {
	// Note: Accessing router.routes directly for simplicity.
	// In a more complex scenario, you might want a dedicated method
	// on Router to export routes cleanly.
	routesInfo := make(map[string]map[string]string)
	for method, patterns := range c.engine.router.routes {
		routesInfo[method] = make(map[string]string)
		for pattern, route := range patterns {
			routesInfo[method][pattern] = fmt.Sprintf("Handler: %s, Before: %d, After: %d",
				getFunctionName(route.handler), len(route.before), len(route.after))
		}
	}
	return c.JSON(http.StatusOK, routesInfo)
}

// DebugConfigHandler exposes active configuration values.
func DebugConfigHandler(c *Context) error {
	// Access config values directly (ConfigManager already has Get/Set)
	// For security, ensure sensitive config values are not exposed in production.
	// This example exposes all, but you'd filter in a real app.
	configValues := make(map[string]string)
	c.engine.Config.mu.RLock() // Access internal map safely
	for k, v := range c.engine.Config.values {
		configValues[k] = v
	}
	c.engine.Config.mu.RUnlock()
	return c.JSON(http.StatusOK, configValues)
}

// DebugMemoryHandler exposes current memory usage statistics.
func DebugMemoryHandler(c *Context) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memStats := map[string]interface{}{
		"Alloc":        fmt.Sprintf("%v MB", bToMb(m.Alloc)),
		"TotalAlloc":   fmt.Sprintf("%v MB", bToMb(m.TotalAlloc)),
		"Sys":          fmt.Sprintf("%v MB", bToMb(m.Sys)),
		"NumGC":        m.NumGC,
		"LastGC":       time.Unix(0, int64(m.LastGC)).Format(time.RFC3339),
		"HeapObjects":  m.HeapObjects,
		"LiveObjects":  m.Mallocs - m.Frees,
		"Goroutines":   runtime.NumGoroutine(),
		"CgoCalls":     runtime.NumCgoCall(),
		"NumCPU":       runtime.NumCPU(),
		"GoVersion":    runtime.Version(),
		"GoOS":         runtime.GOOS,
		"GoArch":       runtime.GOARCH,
	}
	return c.JSON(http.StatusOK, memStats)
}

// DebugGoroutinesHandler exposes information about all active goroutines.
// This can be very verbose; use with caution in production.
func DebugGoroutinesHandler(c *Context) error {
	// Get a profile of active goroutines
	buf := make([]byte, 1<<20) // 1MB buffer
	n := runtime.Stack(buf, true) // Get stack traces for all goroutines
	return c.String(http.StatusOK, string(buf[:n]))
}

// DebugPprofHandler serves pprof profiles.
// This is a more advanced debugging endpoint, typically exposed on a separate port or protected.
// For simplicity, we'll just expose the index and a common profile.
func DebugPprofHandler(c *Context) error {
	// This is a simplified integration. For full pprof, you'd typically use
	// "net/http/pprof" directly, often on a separate debug port.
	// Here, we provide a basic way to get a common profile.
	profileType := c.Param("profile")
	if profileType == "" {
		profileType = "heap" // Default profile
	}

	p := pprof.Lookup(profileType)
	if p == nil {
		return NewHTTPError(http.StatusNotFound, fmt.Sprintf("Profile '%s' not found", profileType))
	}

	c.Writer.Header().Set("Content-Type", "application/octet-stream")
	p.WriteTo(c.Writer, 1) // 1 for text format
	return nil
}


// Helper to convert bytes to MB
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

// getFunctionName extracts the name of a function from its reflection value.
func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
