// Package clickhouse owns the ClickHouse schema and adapters used by the data
// plane. Migrations are additive and versioned; they are embedded so the
// migration runner and the storage benchmark apply the same bytes.
package clickhouse

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migration is one versioned, additive ClickHouse schema migration.
type Migration struct {
	Version int
	Name    string
	File    string
	SQL     string
}

// Migrations returns the embedded migrations ordered by version. It rejects
// malformed filenames and duplicate versions so an accidental edit cannot
// change the apply order silently.
func Migrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read clickhouse migrations: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, name, err := parseMigrationName(entry.Name())
		if err != nil {
			return nil, err
		}
		if previous, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate clickhouse migration version %d: %s and %s", version, previous, entry.Name())
		}
		seen[version] = entry.Name()

		body, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read clickhouse migration %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			File:    entry.Name(),
			SQL:     strings.TrimSpace(string(body)),
		})
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no clickhouse migrations embedded")
	}

	sort.Slice(migrations, func(i, j int) bool {
		if migrations[i].Version != migrations[j].Version {
			return migrations[i].Version < migrations[j].Version
		}
		return migrations[i].File < migrations[j].File
	})
	return migrations, nil
}

// parseMigrationName parses NNNN_name.sql. The version must be a positive
// integer; the name is the stable migration identifier used in tests.
func parseMigrationName(file string) (int, string, error) {
	if !strings.HasSuffix(file, ".sql") {
		return 0, "", fmt.Errorf("invalid clickhouse migration %q: expected NNNN_name.sql", file)
	}
	base := strings.TrimSuffix(file, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 || len(parts[0]) != 4 || parts[1] == "" {
		return 0, "", fmt.Errorf("invalid clickhouse migration %q: expected NNNN_name.sql", file)
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil || version <= 0 {
		return 0, "", fmt.Errorf("invalid clickhouse migration %q: version must be a positive integer", file)
	}
	return version, parts[1], nil
}
