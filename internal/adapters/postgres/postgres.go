// Package postgres owns the Observaki control-plane metadata (AD-7, AD-28).
//
// It provides the versioned schema, the development seed used by the
// single-node stack, and the logical backup/restore of the metadata that the
// operator runbooks exercise. The data plane never reads this database to serve
// telemetry (AD-3); the Query Service is the only read bridge.
package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLockID serializes migrations, restores and the chart's migration
// job (AD-25: migration jobs take an advisory lock).
const migrationLockID int64 = 0x4f42534b // "OBSK"

// DB is a pooled PostgreSQL connection to the control plane.
type DB struct {
	pool *pgxpool.Pool
	dsn  string
}

// Open connects to the control plane. Callers must Close it.
func Open(ctx context.Context, dsn string) (*DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres: database URL is required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &DB{pool: pool, dsn: dsn}, nil
}

// Close releases the pool.
func (db *DB) Close() {
	if db != nil && db.pool != nil {
		db.pool.Close()
	}
}

// Pool exposes the underlying pool for the alerting store in the same package.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Ping checks that PostgreSQL is reachable. A nil DB (or a DB whose pool
// failed to open) reports an error instead of panicking, so the health page can
// degrade gracefully.
func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return errors.New("postgres: not connected")
	}
	return db.pool.Ping(ctx)
}

// MigrationRecord is one applied migration.
type MigrationRecord struct {
	Version   string
	Checksum  string
	AppliedAt time.Time
}

// Migrate applies the embedded, versioned migrations in order. It is
// idempotent: an already-applied version is skipped, and a changed checksum is
// rejected instead of silently rewriting a database.
func (db *DB) Migrate(ctx context.Context) ([]MigrationRecord, error) {
	cfg, err := pgx.ParseConfig(db.dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse database URL: %w", err)
	}
	// The simple protocol allows a migration file to contain several
	// statements.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect for migration: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS observaki_schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return nil, fmt.Errorf("postgres: create migration table: %w", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return nil, fmt.Errorf("postgres: acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("postgres: list migrations: %w", err)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("postgres: no embedded migrations")
	}

	applied := make([]MigrationRecord, 0, len(names))
	for _, name := range names {
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("postgres: read %s: %w", name, err)
		}
		version := strings.TrimSuffix(path.Base(name), ".sql")
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])

		var recorded string
		err = conn.QueryRow(ctx, "SELECT checksum FROM observaki_schema_migrations WHERE version = $1", version).Scan(&recorded)
		switch {
		case err == nil:
			if recorded != checksum {
				return nil, fmt.Errorf("postgres: migration %s changed after it was applied (recorded %s, file %s)", version, recorded, checksum)
			}
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, fmt.Errorf("postgres: read migration %s: %w", version, err)
		}

		if _, err := conn.Exec(ctx, string(body)); err != nil {
			return nil, fmt.Errorf("postgres: apply %s: %w", version, err)
		}
		var appliedAt time.Time
		if err := conn.QueryRow(ctx,
			"INSERT INTO observaki_schema_migrations (version, checksum) VALUES ($1, $2) RETURNING applied_at",
			version, checksum,
		).Scan(&appliedAt); err != nil {
			return nil, fmt.Errorf("postgres: record migration %s: %w", version, err)
		}
		applied = append(applied, MigrationRecord{Version: version, Checksum: checksum, AppliedAt: appliedAt})
	}
	return applied, nil
}

// SchemaVersion returns the last applied migration version.
func (db *DB) SchemaVersion(ctx context.Context) (string, error) {
	var version string
	err := db.pool.QueryRow(ctx,
		"SELECT version FROM observaki_schema_migrations ORDER BY version DESC LIMIT 1",
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: schema version: %w", err)
	}
	return version, nil
}
