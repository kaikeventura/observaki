package ingest

import (
	"strings"
	"testing"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/proto"
)

func benchmarkLogRecord(bodySize int) *logspb.LogRecord {
	body := strings.Repeat("checkout latency budget ", bodySize/24+1)
	return &logspb.LogRecord{Body: stringValue(body)}
}

func BenchmarkDecodeRedact1000Logs(b *testing.B) {
	request := logsRequest()
	for i := 0; i < 1000; i++ {
		request.ResourceLogs[0].ScopeLogs[0].LogRecords = append(
			request.ResourceLogs[0].ScopeLogs[0].LogRecords,
			benchmarkLogRecord(820),
		)
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		b.Fatal(err)
	}
	redactor := newRedactor(DefaultRedactionConfig())
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded collectorlogspb.ExportLogsServiceRequest
		if err := proto.Unmarshal(raw, &decoded); err != nil {
			b.Fatal(err)
		}
		redactor.apply(&Batch{Signal: SignalLogs, Logs: &decoded})
	}
}
