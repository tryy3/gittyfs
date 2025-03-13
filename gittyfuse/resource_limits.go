package gittyfuse

import (
	"log"
	"runtime/debug"
	"syscall"
)

// SetResourceLimits configures memory and other resource limits
// to aid in debugging crashes and resource exhaustion
func SetResourceLimits(maxMemMB int) error {
	// Set Go garbage collector to be more aggressive
	debug.SetGCPercent(20) // Default is 100

	// Set memory limit (soft and hard)
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_AS, &rLimit)
	if err != nil {
		log.Printf("Warning: failed to get memory limits: %v", err)
		return err
	}

	// Convert MB to bytes
	maxMemBytes := uint64(maxMemMB) * 1024 * 1024

	// Set new limits
	rLimit.Cur = maxMemBytes
	rLimit.Max = maxMemBytes * 2 // Hard limit is twice the soft limit

	err = syscall.Setrlimit(syscall.RLIMIT_AS, &rLimit)
	if err != nil {
		log.Printf("Warning: failed to set memory limits: %v", err)
		return err
	}

	log.Printf("Set memory limit to %d MB (soft) / %d MB (hard)",
		maxMemMB, maxMemMB*2)

	return nil
}
