//go:build !linux

package resources

import "time"

// ChildUsage is the raw accumulated usage of reaped child processes.
type ChildUsage struct {
	Available     bool
	UserSeconds   float64
	SystemSeconds float64
	MaxRSSBytes   uint64
}

// ReadChildren is not available on this platform.
func ReadChildren() ChildUsage {
	return ChildUsage{}
}

// Delta returns the child usage accumulated between two readings.
func (before ChildUsage) Delta(after ChildUsage, elapsed time.Duration, cpus int) Children {
	return Children{}
}
