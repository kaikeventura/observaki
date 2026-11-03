package id

import (
	"regexp"
	"testing"
	"time"
)

var uuidV7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv7Format(t *testing.T) {
	value := NewUUIDv7()
	if !uuidV7Pattern.MatchString(value) {
		t.Fatalf("NewUUIDv7() = %q, want a version 7 UUID", value)
	}
}

func TestNewUUIDv7AtEmbedsTimestamp(t *testing.T) {
	timestamp := time.Date(2026, 11, 2, 12, 34, 56, 789_000_000, time.UTC)
	value := NewUUIDv7At(timestamp)

	if !uuidV7Pattern.MatchString(value) {
		t.Fatalf("NewUUIDv7At() = %q, want a version 7 UUID", value)
	}

	var ms uint64
	for _, c := range value[:13] {
		if c == '-' {
			continue
		}
		ms <<= 4
		switch {
		case c >= '0' && c <= '9':
			ms |= uint64(c - '0')
		case c >= 'a' && c <= 'f':
			ms |= uint64(c-'a') + 10
		}
	}
	if got, want := ms, uint64(timestamp.UnixMilli()); got != want {
		t.Fatalf("embedded timestamp = %d, want %d", got, want)
	}
}

func TestNewUUIDv7Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		value := NewUUIDv7()
		if _, ok := seen[value]; ok {
			t.Fatalf("duplicate UUID %q", value)
		}
		seen[value] = struct{}{}
	}
}
