package gittyfuse

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

// ProfilingContext holds the state for ongoing profiling
type ProfilingContext struct {
	cpuFile    *os.File
	cpuProfile *pprof.Profile
	memFile    *os.File
	stopChan   chan struct{}
	interval   time.Duration
}

// StartProfiling begins CPU and memory profiling with periodic writes to disk
func StartProfiling(cpuFilePath string, memFilePath string, flushInterval time.Duration) (*ProfilingContext, error) {
	ctx := &ProfilingContext{
		stopChan: make(chan struct{}),
		interval: flushInterval,
	}

	// Set up CPU profiling
	var err error
	ctx.cpuFile, err = os.Create(cpuFilePath)
	if err != nil {
		log.Printf("Failed to create CPU profile file: %v", err)
		return nil, err
	}

	if err := pprof.StartCPUProfile(ctx.cpuFile); err != nil {
		log.Printf("Failed to start CPU profiling: %v", err)
		ctx.cpuFile.Close()
		return nil, err
	}

	// Set up memory profiling
	ctx.memFile, err = os.Create(memFilePath)
	if err != nil {
		log.Printf("Failed to create memory profile file: %v", err)
		pprof.StopCPUProfile()
		ctx.cpuFile.Close()
		return nil, err
	}

	// Start background goroutine to periodically flush profiles
	go ctx.periodicFlush()

	log.Printf("Profiling started - CPU: %s, Memory: %s", cpuFilePath, memFilePath)
	return ctx, nil
}

// periodicFlush writes profile data to disk at regular intervals
func (p *ProfilingContext) periodicFlush() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Force garbage collection before memory profile
			runtime.GC()

			// Write memory profile
			if err := pprof.WriteHeapProfile(p.memFile); err != nil {
				log.Printf("Error writing memory profile: %v", err)
			} else {
				// Seek back to beginning for next write
				if _, err := p.memFile.Seek(0, 0); err != nil {
					log.Printf("Error seeking memory profile file: %v", err)
				}
				log.Printf("Memory profile written")
			}

			// We cannot force CPU profile to flush, but we can log activity
			log.Printf("Profiling still active")

		case <-p.stopChan:
			return
		}
	}
}

// StopProfiling ends profiling and closes profile files
func (p *ProfilingContext) StopProfiling() {
	if p == nil {
		return
	}

	// Signal the background goroutine to stop
	close(p.stopChan)

	// Stop CPU profiling
	pprof.StopCPUProfile()
	if p.cpuFile != nil {
		p.cpuFile.Close()
	}

	// Close memory profile file
	if p.memFile != nil {
		p.memFile.Close()
	}

	log.Printf("Profiling stopped")
}
