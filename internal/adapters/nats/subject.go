package nats

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/observaki/observaki/internal/ingest"
)

// Subject layout:
//
//	<prefix>.<tenant>.<project>.e<authz_epoch>.<signal>.<shard>
//
// Tenant, project and generation are encoded as base64url tokens because the
// auth snapshot permits characters, such as '.', that NATS uses as subject
// separators (AD-20). The tokens are reversible for operators.
func subjectFor(prefix string, batch *ingest.Batch, shard int) string {
	return prefix + "." +
		encodeSubjectToken(batch.TenantID) + "." +
		encodeSubjectToken(batch.ProjectID) + "." +
		epochToken(batch.ConfigGeneration) + "." +
		batch.Signal.String() + "." +
		strconv.Itoa(shard)
}

func streamSubject(prefix string, signal ingest.Signal) string {
	return prefix + ".*.*.*." + signal.String() + ".*"
}

// StreamSubject returns the wildcard subject a signal stream stores. It is
// exported so the WAL consumer can create a filtered durable consumer with
// exactly the same layout as the gateway publisher.
func StreamSubject(prefix string, signal ingest.Signal) string {
	return streamSubject(prefix, signal)
}

func streamName(prefix string, signal ingest.Signal) string {
	return prefix + "_" + strings.ToUpper(signal.String())
}

func epochToken(generation int64) string {
	return "e" + strconv.FormatInt(generation, 10)
}

func encodeSubjectToken(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// shardFor spreads one tenant's requests across shards without any shared
// state. Request IDs are unique, so a single tenant cannot pin all traffic to
// one subject; retries may land on a different shard and are deduplicated by
// event_id downstream (AD-2, AD-18).
func shardFor(batch *ingest.Batch, requestID string, shards int) int {
	hash := uint64(14695981039346656037)
	hash = fnvString(hash, batch.TenantID)
	hash = fnvString(hash, batch.ProjectID)
	hash = fnvString(hash, batch.Signal.String())
	hash = fnvString(hash, requestID)
	return int(hash % uint64(shards))
}

func fnvString(hash uint64, value string) uint64 {
	for index := 0; index < len(value); index++ {
		hash ^= uint64(value[index])
		hash *= 1099511628211
	}
	return hash
}
