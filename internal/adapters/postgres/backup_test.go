package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSnapshotValidate(t *testing.T) {
	t.Run("accepts a complete snapshot", func(t *testing.T) {
		snapshot := emptySnapshot()
		if err := snapshot.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("rejects an unsupported format", func(t *testing.T) {
		snapshot := emptySnapshot()
		snapshot.FormatVersion = 99
		if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported backup format") {
			t.Fatalf("Validate() error = %v, want unsupported format", err)
		}
	})

	t.Run("rejects a missing table", func(t *testing.T) {
		snapshot := emptySnapshot()
		delete(snapshot.Tables, "dashboards")
		if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "missing table") {
			t.Fatalf("Validate() error = %v, want missing table", err)
		}
	})

	t.Run("rejects an unknown table", func(t *testing.T) {
		snapshot := emptySnapshot()
		snapshot.Tables["not_a_table"] = json.RawMessage(`[]`)
		if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "unknown table") {
			t.Fatalf("Validate() error = %v, want unknown table", err)
		}
	})

	t.Run("rejects a non-array table", func(t *testing.T) {
		snapshot := emptySnapshot()
		snapshot.Tables["organizations"] = json.RawMessage(`{"id":"x"}`)
		if err := snapshot.Validate(); err == nil || !strings.Contains(err.Error(), "not a JSON array") {
			t.Fatalf("Validate() error = %v, want non-array", err)
		}
	})
}

func TestSnapshotRoundTrip(t *testing.T) {
	snapshot := emptySnapshot()
	snapshot.Tables["organizations"] = json.RawMessage(`[{"id":"00000000-0000-4000-8000-000000000001","name":"Dev","slug":"dev"}]`)
	snapshot.Counts["organizations"] = 1

	var buffer bytes.Buffer
	if err := snapshot.Encode(&buffer); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := DecodeSnapshot(buffer.Bytes())
	if err != nil {
		t.Fatalf("DecodeSnapshot() error = %v", err)
	}
	if decoded.FormatVersion != BackupFormatVersion {
		t.Fatalf("FormatVersion = %d, want %d", decoded.FormatVersion, BackupFormatVersion)
	}
	var got, want []map[string]any
	if err := json.Unmarshal(decoded.Tables["organizations"], &got); err != nil {
		t.Fatalf("decode round trip: %v", err)
	}
	if err := json.Unmarshal(snapshot.Tables["organizations"], &want); err != nil {
		t.Fatalf("decode original: %v", err)
	}
	if len(got) != 1 || got[0]["slug"] != want[0]["slug"] {
		t.Fatalf("organizations round trip = %s", decoded.Tables["organizations"])
	}
}

func emptySnapshot() *Snapshot {
	tables := make(map[string]json.RawMessage, len(MetadataTables))
	for _, table := range MetadataTables {
		tables[table] = json.RawMessage(`[]`)
	}
	return &Snapshot{
		FormatVersion: BackupFormatVersion,
		CreatedAt:     time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		Tables:        tables,
		Counts:        map[string]int{},
	}
}

func TestNilDBPing(t *testing.T) {
	var db *DB
	if err := db.Ping(context.Background()); err == nil {
		t.Fatal("Ping() on a nil DB = nil, want an error")
	}
	if err := (*DB)(nil).Ping(context.Background()); err == nil {
		t.Fatal("Ping() on a nil DB pointer = nil, want an error")
	}
}

// TestIntegrationPostgres runs only when OBSERVAKI_POSTGRES_TEST_URL points to
// a disposable PostgreSQL. Plain `go test ./...` skips it.
func TestIntegrationPostgres(t *testing.T) {
	dsn := os.Getenv("OBSERVAKI_POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("OBSERVAKI_POSTGRES_TEST_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	applied, err := db.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	t.Logf("first migration applied %d versions", len(applied))
	applied, err = db.Migrate(ctx)
	if err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("second Migrate() applied %d versions, want 0", len(applied))
	}

	if err := db.SeedDev(ctx, "staging"); err == nil {
		t.Fatal("SeedDev(staging) = nil, want refusal")
	}
	if err := db.SeedDev(ctx, "development"); err != nil {
		t.Fatalf("SeedDev() error = %v", err)
	}

	before, err := db.Export(ctx, "test")
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	for _, table := range []string{"organizations", "users", "projects", "dashboards", "alert_rules"} {
		if before.Counts[table] == 0 {
			t.Fatalf("seed produced no %s rows", table)
		}
	}

	// Mutate the metadata, then restore the backup and verify the four AC1
	// categories come back together.
	if _, err := db.Pool().Exec(ctx, "DELETE FROM dashboards"); err != nil {
		t.Fatalf("delete dashboards: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE projects SET name = 'Broken'"); err != nil {
		t.Fatalf("rename project: %v", err)
	}
	if _, err := db.Restore(ctx, before, RestoreOptions{}); err == nil {
		t.Fatal("Restore() without force = nil, want refusal")
	}
	result, err := db.Restore(ctx, before, RestoreOptions{Force: true})
	if err != nil {
		t.Fatalf("Restore(force) error = %v", err)
	}
	if result.Tables["dashboards"] != 1 || result.Tables["users"] != 1 || result.Tables["alert_rules"] != 1 {
		t.Fatalf("restore counts = %+v", result.Tables)
	}

	after, err := db.Export(ctx, "test")
	if err != nil {
		t.Fatalf("Export(after) error = %v", err)
	}
	for _, table := range MetadataTables {
		if before.Counts[table] != after.Counts[table] {
			t.Fatalf("%s count after restore = %d, want %d", table, after.Counts[table], before.Counts[table])
		}
	}
	var projectName, dashboardTitle string
	if err := db.Pool().QueryRow(ctx, "SELECT name FROM projects WHERE id = $1", DevProjectID).Scan(&projectName); err != nil {
		t.Fatalf("read project: %v", err)
	}
	if projectName != "Development" {
		t.Fatalf("project name = %q, want Development", projectName)
	}
	if err := db.Pool().QueryRow(ctx, "SELECT title FROM dashboards WHERE id = $1", DevDashboardID).Scan(&dashboardTitle); err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	if dashboardTitle != "Development overview" {
		t.Fatalf("dashboard title = %q", dashboardTitle)
	}
}
