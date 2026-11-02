//go:build !linux

package resources

import "errors"

// errUnsupported marks readings that the current platform cannot provide.
var errUnsupported = errors.New("resource sampling is not supported on this platform")

func readCPUTimes() (total, idle uint64, err error) {
	return 0, 0, errUnsupported
}

func readMemory() (total, available uint64, err error) {
	return 0, 0, errUnsupported
}

func readLoad1() (float64, error) {
	return 0, errUnsupported
}
