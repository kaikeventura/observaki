//go:build linux

package resources

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	procStatPath = "/proc/stat"
	procMemPath  = "/proc/meminfo"
	procLoadPath = "/proc/loadavg"
)

func readCPUTimes() (total, idle uint64, err error) {
	return readCPUTimesFrom(procStatPath)
}

func readCPUTimesFrom(path string) (total, idle uint64, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	return parseCPUTimes(data)
}

// parseCPUTimes parses the aggregate "cpu" line of /proc/stat. Idle includes
// iowait, which is time the CPU was not running the workload.
func parseCPUTimes(data []byte) (total, idle uint64, err error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, fmt.Errorf("malformed cpu line %q", line)
		}
		values := make([]uint64, 0, len(fields)-1)
		for _, field := range fields[1:] {
			value, parseErr := strconv.ParseUint(field, 10, 64)
			if parseErr != nil {
				return 0, 0, fmt.Errorf("parse cpu field %q: %w", field, parseErr)
			}
			values = append(values, value)
		}
		// guest and guest_nice are already included in user and nice; summing
		// them again would double-count time.
		for index, value := range values {
			if index >= 8 {
				break
			}
			total += value
		}
		idle = values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		return total, idle, nil
	}
	return 0, 0, fmt.Errorf("cpu line not found in %s", procStatPath)
}

func readMemory() (total, available uint64, err error) {
	return readMemoryFrom(procMemPath)
}

func readMemoryFrom(path string) (total, available uint64, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	return parseMemInfo(data)
}

// parseMemInfo returns MemTotal and MemAvailable in bytes.
func parseMemInfo(data []byte) (total, available uint64, err error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			value, parseErr := memInfoBytes(line)
			if parseErr != nil {
				return 0, 0, parseErr
			}
			total = value
		case strings.HasPrefix(line, "MemAvailable:"):
			value, parseErr := memInfoBytes(line)
			if parseErr != nil {
				return 0, 0, parseErr
			}
			available = value
		}
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("MemTotal not found in %s", procMemPath)
	}
	return total, available, nil
}

func memInfoBytes(line string) (uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("malformed meminfo line %q", line)
	}
	kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse meminfo value %q: %w", fields[1], err)
	}
	return kilobytes * 1024, nil
}

func readLoad1() (float64, error) {
	data, err := os.ReadFile(procLoadPath)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("malformed %s", procLoadPath)
	}
	return strconv.ParseFloat(fields[0], 64)
}
