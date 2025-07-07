// go-swift/goswift/plugin.go
package goswift

import (
	"fmt"
	"sync"
)

// PluginRegistry allows registering and retrieving reusable modules/plugins.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]interface{}
}

// NewPluginRegistry creates and initializes a new PluginRegistry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]interface{}),
	}
}

// RegisterPlugin registers a plugin instance with a given name.
// It returns an error if a plugin with the same name already exists.
func (pr *PluginRegistry) RegisterPlugin(name string, plugin interface{}) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.plugins[name]; exists {
		return fmt.Errorf("plugin with name '%s' already registered", name)
	}
	pr.plugins[name] = plugin
	return nil
}

// GetPlugin retrieves a plugin instance by its name.
// It returns the plugin and true if found, otherwise nil and false.
func (pr *PluginRegistry) GetPlugin(name string) (interface{}, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	plugin, ok := pr.plugins[name]
	return plugin, ok
}

// MustGetPlugin retrieves a plugin instance by its name.
// It panics if the plugin is not found. Use this when you are certain the plugin exists.
func (pr *PluginRegistry) MustGetPlugin(name string) interface{} {
	plugin, ok := pr.GetPlugin(name)
	if !ok { // Corrected: Check !ok instead of undefined 'err'
		panic(fmt.Sprintf("plugin '%s' not found in registry", name))
	}
	return plugin
}
