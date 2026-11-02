package clickhouse

import (
	"regexp"
	"strings"
	"testing"
)

func TestMigrationsAreOrdered(t *testing.T) {
	migrations, err := Migrations()
	if err != nil {
		t.Fatalf("Migrations() error = %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("Migrations() returned no migrations")
	}

	previous := 0
	for _, migration := range migrations {
		if migration.Version <= previous {
			t.Fatalf("migration %s is not ordered after version %d", migration.File, previous)
		}
		if migration.SQL == "" {
			t.Fatalf("migration %s has no SQL", migration.File)
		}
		previous = migration.Version
	}
}

func TestLogsMigrationContract(t *testing.T) {
	migration := findMigration(t, "otel_logs")
	ddl := migration.SQL

	if !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS otel_logs") {
		t.Fatal("logs migration must create otel_logs idempotently")
	}
	if !strings.Contains(ddl, "ENGINE = MergeTree") {
		t.Fatal("logs migration must use MergeTree")
	}

	for _, column := range []string{
		"tenant_id",
		"project_id",
		"timestamp",
		"ingested_at",
		"service_name",
		"severity",
		"body",
		"attributes",
		"resource_attributes",
		"trace_id",
		"span_id",
		"event_id",
		"retention_days",
	} {
		if !hasColumn(ddl, column) {
			t.Errorf("logs migration is missing column %q", column)
		}
	}

	partition := clause(ddl, "PARTITION BY")
	if !strings.Contains(partition, "ingested_at") {
		t.Errorf("PARTITION BY = %q, want a clause based on ingested_at", partition)
	}

	orderBy := clause(ddl, "ORDER BY")
	wantOrder := []string{"tenant_id", "project_id", "service_name", "timestamp"}
	previous := -1
	for _, column := range wantOrder {
		index := strings.Index(orderBy, column)
		if index < 0 {
			t.Fatalf("ORDER BY = %q, want column %q", orderBy, column)
		}
		if index <= previous {
			t.Fatalf("ORDER BY = %q, want %s in tenant/project/service/time order", orderBy, strings.Join(wantOrder, ", "))
		}
		previous = index
	}

	ttl := clause(ddl, "TTL")
	for _, fragment := range []string{"ingested_at", "retention_days", "INTERVAL"} {
		if !strings.Contains(ttl, fragment) {
			t.Errorf("TTL = %q, want %q", ttl, fragment)
		}
	}

	for _, index := range []string{"idx_body", "idx_trace", "idx_span"} {
		if !strings.Contains(ddl, index) {
			t.Errorf("logs migration is missing index %q", index)
		}
	}
	for _, codec := range []string{"ZSTD(3)", "Delta(8)"} {
		if !strings.Contains(ddl, codec) {
			t.Errorf("logs migration is missing codec %q", codec)
		}
	}
	if strings.Contains(strings.ToUpper(ddl), "SELECT *") {
		t.Error("logs migration must not contain SELECT *")
	}
}

func TestTracesMigrationContract(t *testing.T) {
	migration := findMigration(t, "otel_traces")
	ddl := migration.SQL

	if !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS otel_traces") {
		t.Fatal("traces migration must create otel_traces idempotently")
	}
	if !strings.Contains(ddl, "ENGINE = MergeTree") {
		t.Fatal("traces migration must use MergeTree")
	}

	for _, column := range []string{
		"tenant_id",
		"project_id",
		"timestamp",
		"ingested_at",
		"trace_id",
		"span_id",
		"parent_span_id",
		"service_name",
		"operation_name",
		"duration_ns",
		"status_code",
		"attributes",
		"events",
		"links",
		"event_id",
		"retention_days",
	} {
		if !hasColumn(ddl, column) {
			t.Errorf("traces migration is missing column %q", column)
		}
	}

	partition := clause(ddl, "PARTITION BY")
	if !strings.Contains(partition, "ingested_at") {
		t.Errorf("PARTITION BY = %q, want a clause based on ingested_at", partition)
	}

	orderBy := clause(ddl, "ORDER BY")
	wantOrder := []string{"tenant_id", "project_id", "service_name", "status_code", "timestamp"}
	previous := -1
	for _, column := range wantOrder {
		index := strings.Index(orderBy, column)
		if index < 0 {
			t.Fatalf("ORDER BY = %q, want column %q", orderBy, column)
		}
		if index <= previous {
			t.Fatalf("ORDER BY = %q, want %s in tenant/project/service/status/time order", orderBy, strings.Join(wantOrder, ", "))
		}
		previous = index
	}

	ttl := clause(ddl, "TTL")
	for _, fragment := range []string{"ingested_at", "retention_days", "INTERVAL"} {
		if !strings.Contains(ttl, fragment) {
			t.Errorf("TTL = %q, want %q", ttl, fragment)
		}
	}

	for _, index := range []string{"idx_trace", "idx_span", "idx_parent", "idx_service"} {
		if !strings.Contains(ddl, index) {
			t.Errorf("traces migration is missing index %q", index)
		}
	}
	for _, codec := range []string{"ZSTD(3)", "Delta(8)"} {
		if !strings.Contains(ddl, codec) {
			t.Errorf("traces migration is missing codec %q", codec)
		}
	}

	// AD-21: RED belongs to the gateway and VictoriaMetrics. The traces table
	// must not grow a span_metrics/RED materialized view in ClickHouse. Check
	// only the statements; the header comment explains the rule.
	statements := sqlWithoutComments(ddl)
	lowerStatements := strings.ToLower(statements)
	for _, forbidden := range []string{"materialized view", "span_metrics"} {
		if strings.Contains(lowerStatements, forbidden) {
			t.Errorf("traces migration must not contain %q (AD-21)", forbidden)
		}
	}
	if strings.Contains(strings.ToUpper(statements), "SELECT *") {
		t.Error("traces migration must not contain SELECT *")
	}
}

// sqlWithoutComments removes whole-line SQL comments. It is enough for the
// migration files, which use -- comments outside string literals.
func sqlWithoutComments(ddl string) string {
	var statements strings.Builder
	for _, line := range strings.Split(ddl, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		statements.WriteString(line)
		statements.WriteByte('\n')
	}
	return statements.String()
}

func TestParseMigrationName(t *testing.T) {
	tests := []struct {
		file    string
		version int
		name    string
		wantErr bool
	}{
		{file: "0001_otel_logs.sql", version: 1, name: "otel_logs"},
		{file: "0010_add_index.sql", version: 10, name: "add_index"},
		{file: "1_bad.sql", wantErr: true},
		{file: "0000_zero.sql", wantErr: true},
		{file: "0001_.sql", wantErr: true},
		{file: "abcd_name.sql", wantErr: true},
		{file: "0001_noext", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			version, name, err := parseMigrationName(test.file)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseMigrationName(%q) error = nil, want an error", test.file)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMigrationName(%q) error = %v", test.file, err)
			}
			if version != test.version || name != test.name {
				t.Fatalf("parseMigrationName(%q) = (%d, %q), want (%d, %q)", test.file, version, name, test.version, test.name)
			}
		})
	}
}

func findMigration(t *testing.T, name string) Migration {
	t.Helper()
	migrations, err := Migrations()
	if err != nil {
		t.Fatalf("Migrations() error = %v", err)
	}
	for _, migration := range migrations {
		if migration.Name == name {
			return migration
		}
	}
	t.Fatalf("migration %q not found", name)
	return Migration{}
}

// hasColumn reports whether name is the first token of a column definition in
// the CREATE TABLE block. It deliberately ignores indexes and clauses so a
// comment or an index name cannot satisfy a column requirement.
func hasColumn(ddl, name string) bool {
	for _, line := range strings.Split(ddl, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

func clause(ddl, keyword string) string {
	expression := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(keyword) + `\s+(.+)$`)
	match := expression.FindStringSubmatch(ddl)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
