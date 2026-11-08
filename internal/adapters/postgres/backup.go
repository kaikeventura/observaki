package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// BackupFormatVersion is the logical backup format. It is independent from the
// schema migrations: a newer binary can still restore an older document.
const BackupFormatVersion = 1

// MetadataTables lists the control-plane tables in dependency order. Export and
// restore use this order; derived alerting state (alert_states, alert_outbox)
// is intentionally excluded because it is recomputed by the evaluator.
var MetadataTables = []string{
	"organizations",
	"users",
	"projects",
	"retention_policies",
	"dashboards",
	"notification_channels",
	"alert_rules",
	"alert_rule_channels",
	"memberships",
	"api_tokens",
	"silences",
	"slos",
	"error_budgets",
	"audit_events",
}

var metadataTableSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(MetadataTables))
	for _, table := range MetadataTables {
		set[table] = struct{}{}
	}
	return set
}()

// Snapshot is the logical metadata backup (AC1).
type Snapshot struct {
	FormatVersion int                        `json:"format_version"`
	CreatedAt     time.Time                  `json:"created_at"`
	AppVersion    string                     `json:"app_version,omitempty"`
	SchemaVersion string                     `json:"schema_version,omitempty"`
	Tables        map[string]json.RawMessage `json:"tables"`
	Counts        map[string]int             `json:"counts"`
}

// Encode writes the snapshot as indented JSON without HTML escaping.
func (s *Snapshot) Encode(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s); err != nil {
		return fmt.Errorf("postgres: encode backup: %w", err)
	}
	return nil
}

// DecodeSnapshot parses and validates a backup document.
func DecodeSnapshot(data []byte) (*Snapshot, error) {
	var snapshot Snapshot
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("postgres: decode backup: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// Validate checks the format version and that every metadata table is present
// exactly once as a JSON array. Unknown tables are rejected so a typo cannot
// silently drop data.
func (s *Snapshot) Validate() error {
	if s.FormatVersion != BackupFormatVersion {
		return fmt.Errorf("postgres: unsupported backup format %d (expected %d)", s.FormatVersion, BackupFormatVersion)
	}
	for table := range s.Tables {
		if _, ok := metadataTableSet[table]; !ok {
			return fmt.Errorf("postgres: backup contains unknown table %q", table)
		}
	}
	for _, table := range MetadataTables {
		raw, ok := s.Tables[table]
		if !ok {
			return fmt.Errorf("postgres: backup is missing table %q", table)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return fmt.Errorf("postgres: backup table %q is not a JSON array: %w", table, err)
		}
	}
	return nil
}

// Export reads every metadata table into a Snapshot.
func (db *DB) Export(ctx context.Context, appVersion string) (*Snapshot, error) {
	schemaVersion, err := db.SchemaVersion(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{
		FormatVersion: BackupFormatVersion,
		CreatedAt:     time.Now().UTC(),
		AppVersion:    appVersion,
		SchemaVersion: schemaVersion,
		Tables:        make(map[string]json.RawMessage, len(MetadataTables)),
		Counts:        make(map[string]int, len(MetadataTables)),
	}
	for _, table := range MetadataTables {
		query := fmt.Sprintf("SELECT COALESCE(json_agg(t), '[]'::json)::text FROM public.%s t", table)
		var raw []byte
		if err := db.pool.QueryRow(ctx, query).Scan(&raw); err != nil {
			return nil, fmt.Errorf("postgres: export %s: %w", table, err)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, fmt.Errorf("postgres: decode exported %s: %w", table, err)
		}
		snapshot.Tables[table] = json.RawMessage(raw)
		snapshot.Counts[table] = len(rows)
	}
	return snapshot, nil
}

// RestoreOptions controls a restore.
type RestoreOptions struct {
	// Force replaces existing metadata. Without it a non-empty control plane is
	// left untouched.
	Force bool
}

// RestoreResult reports what the restore wrote.
type RestoreResult struct {
	FormatVersion int
	Elapsed       time.Duration
	Tables        map[string]int64
}

// Restore atomically replaces the control-plane metadata with the snapshot. It
// runs in one transaction under the migration advisory lock, so a failed
// restore leaves the previous metadata intact.
func (db *DB) Restore(ctx context.Context, snapshot *Snapshot, options RestoreOptions) (*RestoreResult, error) {
	if snapshot == nil {
		return nil, errors.New("postgres: backup snapshot is required")
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	started := time.Now()

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin restore: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockID); err != nil {
		return nil, fmt.Errorf("postgres: acquire restore lock: %w", err)
	}

	if !options.Force {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM organizations)").Scan(&exists); err != nil {
			return nil, fmt.Errorf("postgres: inspect target: %w", err)
		}
		if exists {
			return nil, errors.New("postgres: target already has metadata; pass --force to replace it")
		}
	}

	tables := append(append([]string{}, MetadataTables...), "alerting_heartbeat")
	if _, err := tx.Exec(ctx, "TRUNCATE TABLE "+quoteIdentifiers(tables)+" RESTART IDENTITY CASCADE"); err != nil {
		return nil, fmt.Errorf("postgres: clear metadata: %w", err)
	}

	result := &RestoreResult{Tables: make(map[string]int64, len(MetadataTables))}
	for _, table := range MetadataTables {
		raw := snapshot.Tables[table]
		var rows []json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, fmt.Errorf("postgres: restore %s: %w", table, err)
		}
		if len(rows) == 0 {
			result.Tables[table] = 0
			continue
		}
		query := fmt.Sprintf(
			"INSERT INTO public.%s SELECT * FROM json_populate_recordset(NULL::public.%s, $1::json)",
			table, table,
		)
		tag, err := tx.Exec(ctx, query, string(raw))
		if err != nil {
			return nil, fmt.Errorf("postgres: restore %s: %w", table, err)
		}
		result.Tables[table] = tag.RowsAffected()
	}

	// audit_events is BIGSERIAL; a restore inserts the original ids, so the
	// sequence must continue after the highest one.
	if _, err := tx.Exec(ctx, `
		SELECT setval(
			pg_get_serial_sequence('public.audit_events', 'id'),
			COALESCE(max(id), 1),
			max(id) IS NOT NULL
		) FROM public.audit_events`); err != nil {
		return nil, fmt.Errorf("postgres: reset audit sequence: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO alerting_heartbeat (id) VALUES (true) ON CONFLICT (id) DO NOTHING"); err != nil {
		return nil, fmt.Errorf("postgres: reset alerting heartbeat: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: commit restore: %w", err)
	}
	result.FormatVersion = snapshot.FormatVersion
	result.Elapsed = time.Since(started)
	return result, nil
}

func quoteIdentifiers(tables []string) string {
	quoted := make([]string, 0, len(tables))
	for _, table := range tables {
		quoted = append(quoted, `"`+table+`"`)
	}
	return strings.Join(quoted, ", ")
}
