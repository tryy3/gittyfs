package gittyfuse

import (
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

// DebugMonitor starts a background goroutine that logs memory usage
// and other useful diagnostic information at regular intervals
func StartDebugMonitor(interval time.Duration) (cancel func()) {
	done := make(chan struct{})
	ticker := time.NewTicker(interval)

	logFile, err := os.Create("gittyfs_debug.log")
	if err != nil {
		log.Printf("Failed to create debug log file: %v", err)
		logFile = nil
	}

	// Write debug logs to both stderr and the debug file
	debugLogger := log.New(os.Stderr, "DEBUG: ", log.LstdFlags)
	if logFile != nil {
		debugLogger.SetOutput(os.NewFile(uintptr(logFile.Fd()), "debuglog"))
		defer logFile.Close()
	}

	// Create a recovery function for panics
	defer func() {
		if r := recover(); r != nil {
			debugLogger.Printf("PANIC in debug monitor: %v\n%s", r, debug.Stack())
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				debugLogger.Printf("PANIC in debug monitor: %v\n%s", r, debug.Stack())
			}
			ticker.Stop()
			if logFile != nil {
				logFile.Close()
			}
		}()

		for {
			select {
			case <-ticker.C:
				var mem runtime.MemStats
				runtime.ReadMemStats(&mem)

				debugLogger.Printf("Memory: Alloc=%v MiB, TotalAlloc=%v MiB, Sys=%v MiB, NumGC=%v",
					mem.Alloc/1024/1024,
					mem.TotalAlloc/1024/1024,
					mem.Sys/1024/1024,
					mem.NumGC)

				// Log number of goroutines
				debugLogger.Printf("Goroutines: %d", runtime.NumGoroutine())

				// Trigger GC to help avoid OOM killer
				runtime.GC()

			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}
