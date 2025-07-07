// go-swift/goswift/taskqueue.go
package goswift

import (
	"fmt" // For logging panics
	"sync"
)

// AsyncTaskQueue manages asynchronous tasks with a worker pool.
type AsyncTaskQueue struct {
	tasks    chan func() // Channel to send tasks to workers
	wg       sync.WaitGroup // WaitGroup to track active tasks
	workers  int           // Number of worker goroutines
	shutdown chan struct{} // Signal channel for graceful shutdown
	mu       sync.Mutex    // Protects shutdown state
	isShuttingDown bool
}

// NewAsyncTaskQueue creates and starts a new asynchronous task queue with a given number of workers.
func NewAsyncTaskQueue(workers int) *AsyncTaskQueue {
	if workers <= 0 {
		workers = 1 // Ensure at least one worker
	}
	tq := &AsyncTaskQueue{
		tasks:    make(chan func()),
		workers:  workers,
		shutdown: make(chan struct{}),
	}

	for i := 0; i < workers; i++ {
		go tq.worker()
	}
	return tq
}

// worker is a goroutine that processes tasks from the queue.
func (tq *AsyncTaskQueue) worker() {
	for {
		select {
		case task, ok := <-tq.tasks:
			if !ok { // Channel closed, time to exit
				return
			}
			func() {
				defer tq.wg.Done() // Decrement counter when task is done
				defer func() {
					if r := recover(); r != nil {
						// Log task panics to prevent worker crash
						// This logger is not directly from context, so use standard log or pass engine.Logger
						fmt.Printf("AsyncTaskQueue: Recovered from task panic: %v\n", r)
					}
				}()
				task() // Execute the task
			}()
		case <-tq.shutdown:
			// Received shutdown signal, but keep processing existing tasks in channel
			// The loop will exit when tq.tasks is closed and drained.
			return
		}
	}
}

// Go submits a task to the asynchronous queue.
func (tq *AsyncTaskQueue) Go(task func()) {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	if tq.isShuttingDown {
		// Log or handle error if trying to add tasks during shutdown
		fmt.Println("AsyncTaskQueue: Cannot add task, queue is shutting down.")
		return
	}

	tq.wg.Add(1) // Increment counter before sending task
	tq.tasks <- task
}

// Shutdown gracefully stops the task queue.
// It stops accepting new tasks, waits for existing tasks to complete, and then stops workers.
func (tq *AsyncTaskQueue) Shutdown() {
	tq.mu.Lock()
	tq.isShuttingDown = true
	tq.mu.Unlock()

	close(tq.tasks) // Close the task channel to signal workers no new tasks
	close(tq.shutdown) // Signal workers to start draining and exit

	tq.wg.Wait() // Wait for all active tasks to complete
}
