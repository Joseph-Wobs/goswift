// go-swift/goswift/logger.go
package goswift

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// LogLevel defines the severity of a log message.
type LogLevel int

const (
	INFO LogLevel = iota
	WARNING
	ERROR
)

// String returns the string representation of a LogLevel.
func (l LogLevel) String() string {
	switch l {
	case INFO:
		return "INFO"
	case WARNING:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides a simple logging utility.
type Logger struct {
	*log.Logger
	mu sync.Mutex
}

// NewLogger creates a new Logger instance.
func NewLogger() *Logger {
	return &Logger{
		Logger: log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile),
	}
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warning logs a warning message.
func (l *Logger) Warning(format string, args ...interface{}) {
	l.log(WARNING, format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// log formats and writes the log message to the output.
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Printf("[%s] %s", level.String(), fmt.Sprintf(format, args...))
}
