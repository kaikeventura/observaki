package selfobs

import (
	"math"
	"strconv"
	"strings"
)

type promSample struct {
	name  string
	value float64
}

// parsePrometheus parses the gauge/counter lines of a Prometheus text
// exposition. It tolerates comments, blank lines, labels and timestamps.
func parsePrometheus(body string) []promSample {
	samples := make([]promSample, 0, 64)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		space := strings.IndexByte(line, ' ')
		if space < 0 {
			continue
		}
		head := line[:space]
		fields := strings.Fields(line[space:])
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || math.IsNaN(value) {
			continue
		}
		if brace := strings.IndexByte(head, '{'); brace >= 0 {
			head = head[:brace]
		}
		samples = append(samples, promSample{name: head, value: value})
	}
	return samples
}

func promSum(samples []promSample, name string) (float64, bool) {
	total := 0.0
	found := false
	for _, sample := range samples {
		if sample.name == name {
			total += sample.value
			found = true
		}
	}
	return total, found
}

func promFirst(samples []promSample, name string) (float64, bool) {
	for _, sample := range samples {
		if sample.name == name {
			return sample.value, true
		}
	}
	return 0, false
}
