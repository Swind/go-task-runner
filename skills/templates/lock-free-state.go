// Package main demonstrates lock-free state management with SequencedTaskRunner
// This pattern eliminates the need for mutexes by using sequential execution guarantees
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

// ============================================================================
// BEFORE: Traditional approach with mutex
// ============================================================================

type CounterWithMutex struct {
	mu    sync.Mutex
	count int
}

func (c *CounterWithMutex) Increment() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
}

func (c *CounterWithMutex) Get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// ============================================================================
// AFTER: Lock-free approach with SequencedTaskRunner
// ============================================================================

type LockFreeCounter struct {
	runner *taskrunner.SequencedTaskRunner
	count  int // No mutex needed! Protected by sequential execution
}

func NewLockFreeCounter() *LockFreeCounter {
	return &LockFreeCounter{
		runner: taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
	}
}

// Increment posts an increment task - returns immediately
func (c *LockFreeCounter) Increment() {
	c.runner.PostTask(func(ctx context.Context) {
		c.count++ // Lock-free! Only one task executes at a time
	})
}

// Get posts a read task with callback - async result
func (c *LockFreeCounter) Get(callback func(int)) {
	c.runner.PostTask(func(ctx context.Context) {
		callback(c.count) // Lock-free read!
	})
}

// Shutdown cleans up resources
func (c *LockFreeCounter) Shutdown() {
	c.runner.Shutdown()
}

// ============================================================================
// More Complex Example: Lock-free Cache
// ============================================================================

type LockFreeCache struct {
	runner *taskrunner.SequencedTaskRunner
	data   map[string]string // No mutex needed!
	hits   int
	misses int
}

func NewLockFreeCache() *LockFreeCache {
	return &LockFreeCache{
		runner: taskrunner.CreateTaskRunner(taskrunner.DefaultTaskTraits()),
		data:   make(map[string]string),
	}
}

func (c *LockFreeCache) Set(key, value string) {
	c.runner.PostTask(func(ctx context.Context) {
		c.data[key] = value // Lock-free write
	})
}

func (c *LockFreeCache) Get(key string, callback func(string, bool)) {
	c.runner.PostTask(func(ctx context.Context) {
		val, ok := c.data[key] // Lock-free read
		if ok {
			c.hits++ // Lock-free stats update
		} else {
			c.misses++
		}
		callback(val, ok)
	})
}

func (c *LockFreeCache) GetStats(callback func(hits, misses int)) {
	c.runner.PostTask(func(ctx context.Context) {
		callback(c.hits, c.misses) // Lock-free read of multiple fields
	})
}

func (c *LockFreeCache) Shutdown() {
	c.runner.Shutdown()
}

// ============================================================================
// Demo
// ============================================================================

func main() {
	taskrunner.InitGlobalThreadPool(4)
	defer taskrunner.ShutdownGlobalThreadPool()

	fmt.Println("=== Lock-Free Counter Demo ===")
	counter := NewLockFreeCounter()
	defer counter.Shutdown()

	// Concurrent increments - no race conditions!
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.Increment()
		}()
	}
	wg.Wait()

	// Get final value
	done := make(chan int)
	counter.Get(func(val int) {
		done <- val
	})
	finalCount := <-done
	fmt.Printf("Final count: %d (expected: 100)\n", finalCount)

	fmt.Println("\n=== Lock-Free Cache Demo ===")
	cache := NewLockFreeCache()
	defer cache.Shutdown()

	// Concurrent writes
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Concurrent reads
	time.Sleep(50 * time.Millisecond) // Let writes complete

	cacheDone := make(chan struct{})
	cache.Get("key1", func(val string, ok bool) {
		fmt.Printf("Get key1: %s, found: %v\n", val, ok)
	})
	cache.Get("key3", func(val string, ok bool) {
		fmt.Printf("Get key3: %s, found: %v\n", val, ok)
	})
	cache.GetStats(func(hits, misses int) {
		fmt.Printf("Cache stats - Hits: %d, Misses: %d\n", hits, misses)
		close(cacheDone)
	})
	<-cacheDone

	fmt.Println("\n✓ No mutexes used, no race conditions!")
}
