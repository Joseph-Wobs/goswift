// go-swift/goswift/di.go
package goswift

import (
	"fmt"
	"reflect"
	"sync"
)

// Container is a simple Dependency Injection container.
type Container struct {
	mu       sync.RWMutex
	bindings map[reflect.Type]interface{}
}

// NewContainer creates and initializes a new DI Container.
func NewContainer() *Container {
	return &Container{
		bindings: make(map[reflect.Type]interface{}),
	}
}

// Bind registers a value (instance or factory function) with the container.
// The value's type is used as the key for resolution.
// If a factory function is provided (func() T), it will be called on Resolve.
// If an instance is provided, it will be returned directly.
func (c *Container) Bind(value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	valType := reflect.TypeOf(value)
	c.bindings[valType] = value
}

// Resolve resolves a dependency from the container.
// It takes a pointer to the desired type (e.g., &MyService{}).
// It returns the resolved instance and an error if not found or cannot be resolved.
func (c *Container) Resolve(ptrToType interface{}) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	targetVal := reflect.ValueOf(ptrToType)
	if targetVal.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("Resolve expects a pointer to the target type, got %s", targetVal.Kind())
	}

	targetType := targetVal.Elem().Type()

	// Try to find a direct binding for the target type
	if boundValue, ok := c.bindings[targetType]; ok {
		return boundValue, nil
	}

	// Try to find a binding for a factory function that returns the target type
	for boundType, boundValue := range c.bindings {
		if boundType.Kind() == reflect.Func && boundType.NumOut() == 1 && boundType.Out(0) == targetType {
			// Found a factory function, call it
			// For simplicity, assume factory takes no arguments.
			if boundType.NumIn() != 0 {
				return nil, fmt.Errorf("factory function for type %s has arguments, not supported by simple Resolve", targetType)
			}
			results := reflect.ValueOf(boundValue).Call(nil)
			if len(results) > 0 {
				return results[0].Interface(), nil
			}
		}
	}

	return nil, fmt.Errorf("no binding found for type %s", targetType)
}

// MustResolve resolves a dependency, panicking if it cannot be resolved.
// Use this when you are certain the dependency exists.
func (c *Container) MustResolve(ptrToType interface{}) interface{} {
	resolved, err := c.Resolve(ptrToType)
	if err != nil {
		panic(fmt.Sprintf("failed to resolve dependency for type %s: %v", reflect.TypeOf(ptrToType).Elem(), err))
	}
	return resolved
}
