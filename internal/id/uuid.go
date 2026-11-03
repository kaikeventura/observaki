// Package id generates the identifiers used across Observaki services.
package id

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewUUIDv7 returns a UUIDv7 (RFC 9562) string for the current time.
func NewUUIDv7() string {
	return NewUUIDv7At(time.Now())
}

// NewUUIDv7At returns a UUIDv7 string whose timestamp is t. The random tail is
// generated with crypto/rand; the millisecond timestamp makes the identifier
// time-ordered, which keeps indexes and logs sorted by creation time.
func NewUUIDv7At(t time.Time) string {
	var b [16]byte
	// crypto/rand.Read never returns an error and always fills b (Go 1.24+).
	_, _ = rand.Read(b[:])

	ms := uint64(t.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)

	// Version 7 and RFC 9562 variant bits.
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
