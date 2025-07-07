// go-swift/goswift/config.go
package goswift

import (
	"os"
	"sync"
)

// ConfigManager handles application configuration.
type ConfigManager struct {
	values map[string]string
	mu     sync.RWMutex
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		values: make(map[string]string),
	}
}

// Get retrieves a configuration value by key.
// It first checks environment variables, then the internal map.
func (cm *ConfigManager) Get(key string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.values[key]
}

// Set sets a configuration value. This will be overridden by environment variables.
func (cm *ConfigManager) Set(key, value string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.values[key] = value
}
