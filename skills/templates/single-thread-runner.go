// Package main demonstrates SingleThreadTaskRunner for thread affinity
// Use this when you need all tasks on the SAME goroutine (blocking IO, CGO, etc.)
package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	taskrunner "github.com/Swind/go-task-runner"
)

// Example: Database service with blocking queries
type DBService struct {
	runner *taskrunner.SingleThreadTaskRunner
	// db *sql.DB // Hypothetical database connection
}

func NewDBService() *DBService {
	return &DBService{
		runner: taskrunner.NewSingleThreadTaskRunner(),
	}
}

// Query simulates a blocking database query
func (s *DBService) Query(sql string, callback func([]string)) {
	s.runner.PostTask(func(ctx context.Context) {
		// Simulate blocking query
		fmt.Printf("Executing query: %s\n", sql)
		time.Sleep(100 * time.Millisecond)

		// All queries execute on same goroutine - safe for connections with TLS
		results := []string{"row1", "row2", "row3"}
		callback(results)
	})
}

func (s *DBService) Stop() {
	s.runner.Stop()
}

func main() {
	fmt.Println("=== SingleThreadTaskRunner Example ===\n")

	// Example 1: Thread affinity verification
	fmt.Println("--- Example 1: Thread Affinity ---")
	runner := taskrunner.NewSingleThreadTaskRunner()
	defer runner.Stop()

	gids := make(chan uint64, 3)

	for i := 1; i <= 3; i++ {
		i := i
		runner.PostTask(func(ctx context.Context) {
			gid := getGID()
			fmt.Printf("Task %d executing on goroutine %d\n", i, gid)
			gids <- gid
		})
	}

	// Verify all tasks ran on same goroutine
	time.Sleep(100 * time.Millisecond)
	close(gids)
	var firstGID uint64
	allSame := true
	for gid := range gids {
		if firstGID == 0 {
			firstGID = gid
		} else if gid != firstGID {
			allSame = false
		}
	}
	if allSame {
		fmt.Println("✓ All tasks executed on the same goroutine!")
	}

	// Example 2: Blocking IO pattern
	fmt.Println("\n--- Example 2: Blocking IO (Database) ---")
	db := NewDBService()
	defer db.Stop()

	done := make(chan struct{})

	db.Query("SELECT * FROM users WHERE id = 1", func(results []string) {
		fmt.Printf("Query results: %v\n", results)
	})

	db.Query("SELECT * FROM orders WHERE user_id = 1", func(results []string) {
		fmt.Printf("Query results: %v\n", results)
		close(done)
	})

	<-done

	// Example 3: Delayed task
	fmt.Println("\n--- Example 3: Delayed Task ---")
	runner.PostDelayedTask(func(ctx context.Context) {
		fmt.Println("Delayed task executed on dedicated thread")
	}, 200*time.Millisecond)

	time.Sleep(300 * time.Millisecond)

	// Example 4: Repeating task
	fmt.Println("\n--- Example 4: Repeating Task ---")
	count := 0
	handle := runner.PostRepeatingTask(func(ctx context.Context) {
		count++
		fmt.Printf("Repeating task execution #%d\n", count)
	}, 100*time.Millisecond)

	time.Sleep(350 * time.Millisecond)
	handle.Stop()
	fmt.Println("Repeating task stopped")

	time.Sleep(100 * time.Millisecond)
	fmt.Println("\n✓ All tasks executed on dedicated goroutine!")
}

// getGID returns current goroutine ID (for demonstration only)
func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	var gid uint64
	fmt.Sscanf(string(b), "goroutine %d ", &gid)
	return gid
}
