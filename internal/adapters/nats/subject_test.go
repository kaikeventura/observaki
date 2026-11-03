package nats

import (
	"strings"
	"testing"

	"github.com/observaki/observaki/internal/ingest"
)

func TestSubjectShardsAndEncodesTenantTokens(t *testing.T) {
	batch := &ingest.Batch{
		Signal:           ingest.SignalLogs,
		TenantID:         "tenant.a",
		ProjectID:        "project:b",
		ConfigGeneration: 12,
	}
	subject := subjectFor("observaki.ingest.v1", batch, 5)
	tokens := strings.Split(subject, ".")
	if len(tokens) != 8 {
		t.Fatalf("subject %q has %d tokens, want 8", subject, len(tokens))
	}
	if tokens[6] != "logs" || tokens[7] != "5" {
		t.Fatalf("subject %q does not carry signal/shard", subject)
	}
	if strings.Contains(tokens[3], ":") || strings.Contains(tokens[4], "@") {
		t.Fatalf("subject %q has unescaped tenant/project tokens", subject)
	}
	if got := epochToken(12); got != "e12" {
		t.Fatalf("epochToken(12) = %q", got)
	}
}

func TestStreamNamesAndSubjects(t *testing.T) {
	for _, signal := range []ingest.Signal{ingest.SignalTraces, ingest.SignalLogs, ingest.SignalMetrics} {
		if got, want := StreamName("OBSERVAKI", signal), "OBSERVAKI_"+strings.ToUpper(signal.String()); got != want {
			t.Fatalf("StreamName(%s) = %q, want %q", signal, got, want)
		}
		subject := streamSubject("observaki.ingest.v1", signal)
		if strings.Count(subject, ".") != 7 {
			t.Fatalf("stream subject %q has the wrong token count", subject)
		}
	}
}

func TestShardForSpreadsRequests(t *testing.T) {
	batch := &ingest.Batch{Signal: ingest.SignalLogs, TenantID: "tenant", ProjectID: "project"}
	shards := map[int]struct{}{}
	for index := 0; index < 64; index++ {
		shards[shardFor(batch, "request-"+string(rune('a'+index%26))+string(rune('a'+index/26)), 8)] = struct{}{}
	}
	if len(shards) < 2 {
		t.Fatalf("shards = %v, want more than one", shards)
	}
}
