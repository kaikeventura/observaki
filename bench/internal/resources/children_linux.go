//go:build linux

package resources

import (
	"syscall"
	"time"
)

// ChildUsage is the raw accumulated usage of reaped child processes.
type ChildUsage struct {
	Available     bool
	UserSeconds   float64
	SystemSeconds float64
	MaxRSSBytes   uint64
}

// ReadChildren returns the accumulated usage of the process children. On
// Linux, ru_maxrss is reported in kilobytes.
func ReadChildren() ChildUsage {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
		return ChildUsage{}
	}
	return ChildUsage{
		Available:     true,
		UserSeconds:   timevalSeconds(usage.Utime),
		SystemSeconds: timevalSeconds(usage.Stime),
		MaxRSSBytes:   uint64(usage.Maxrss) * 1024,
	}
}

func timevalSeconds(value syscall.Timeval) float64 {
	return float64(value.Sec) + float64(value.Usec)/1e6
}

// Delta returns the child usage accumulated between two readings, normalized
// as a percentage of the whole machine over elapsed.
func (before ChildUsage) Delta(after ChildUsage, elapsed time.Duration, cpus int) Children {
	if !after.Available {
		return Children{}
	}
	user := after.UserSeconds - before.UserSeconds
	system := after.SystemSeconds - before.SystemSeconds
	if user < 0 {
		user = 0
	}
	if system < 0 {
		system = 0
	}
	children := Children{
		Available:     true,
		UserSeconds:   user,
		SystemSeconds: system,
		MaxRSSBytes:   after.MaxRSSBytes,
	}
	if elapsed > 0 && cpus > 0 {
		children.CPUPercent = (user + system) / elapsed.Seconds() / float64(cpus) * 100
	}
	return children
}
