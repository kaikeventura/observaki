package ingest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/observaki/observaki/internal/id"
)

// Audit actions emitted by the gateway.
const (
	AuditActionCrossProjectWrite = "ingest.cross_project_write"
	AuditActionInsufficientScope = "ingest.insufficient_scope"
	AuditActionConfigLoaded      = "ingest.config_loaded"
)

// Audit outcomes.
const (
	AuditOutcomeSuccess = "success"
	AuditOutcomeDenied  = "denied"
)

// Audit actors.
const (
	AuditActorSystem = "system"
	AuditActorToken  = "token"
)

// AuditEvent is one append-only audit record. The gateway writes a hash chain
// (PrevHash -> Hash) so tampering is detectable; the WORM export and signing
// land with the control plane (AD-35).
type AuditEvent struct {
	EventID         string    `json:"event_id"`
	OccurredAt      time.Time `json:"occurred_at"`
	ActorType       string    `json:"actor_type"`
	ActorID         string    `json:"actor_id"`
	Action          string    `json:"action"`
	Outcome         string    `json:"outcome"`
	TenantID        string    `json:"tenant_id,omitempty"`
	ProjectID       string    `json:"project_id,omitempty"`
	TargetTenantID  string    `json:"target_tenant_id,omitempty"`
	TargetProjectID string    `json:"target_project_id,omitempty"`
	RequestID       string    `json:"request_id,omitempty"`
	Signal          string    `json:"signal,omitempty"`
	Protocol        string    `json:"protocol,omitempty"`
	RemoteAddr      string    `json:"remote_addr,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	ConfigVersion   string    `json:"config_version,omitempty"`
	Generation      int64     `json:"generation,omitempty"`
	PrevHash        string    `json:"prev_hash,omitempty"`
	Hash            string    `json:"hash"`
}

// AuditSink persists audit events. Writes must be append-only.
type AuditSink interface {
	Record(ctx context.Context, event AuditEvent) error
}

// AuditSinkFunc adapts a function to AuditSink.
type AuditSinkFunc func(ctx context.Context, event AuditEvent) error

// Record implements AuditSink.
func (f AuditSinkFunc) Record(ctx context.Context, event AuditEvent) error {
	return f(ctx, event)
}

// LogAudit writes audit events to the structured log. It is the development
// fallback when no audit file is configured; staging and production require a
// durable file.
type LogAudit struct {
	Logger *slog.Logger
}

// Record implements AuditSink.
func (l LogAudit) Record(_ context.Context, event AuditEvent) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn(
		"audit event",
		"action", event.Action,
		"outcome", event.Outcome,
		"actor_type", event.ActorType,
		"actor_id", event.ActorID,
		"tenant_id", event.TenantID,
		"project_id", event.ProjectID,
		"target_tenant_id", event.TargetTenantID,
		"target_project_id", event.TargetProjectID,
		"request_id", event.RequestID,
		"reason", event.Reason,
		"config_version", event.ConfigVersion,
		"generation", event.Generation,
	)
	return nil
}

// FileAudit appends JSON lines to a file and fsyncs every record. The file is
// read and verified on open so the hash chain survives restarts; a broken
// chain refuses new writes.
type FileAudit struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	prev    [sha256.Size]byte
	hasPrev bool
	closed  bool
}

// OpenFileAudit opens (or creates) the audit file and resumes its hash chain.
func OpenFileAudit(path string) (*FileAudit, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("ingest: audit file path is required")
	}
	if err := verifyAuditFile(path); err != nil {
		return nil, err
	}
	prev, hasPrev, err := lastAuditHash(path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit file: %w", err)
	}
	return &FileAudit{path: path, file: file, prev: prev, hasPrev: hasPrev}, nil
}

// Path returns the audit file path.
func (f *FileAudit) Path() string { return f.path }

// Record implements AuditSink.
func (f *FileAudit) Record(ctx context.Context, event AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("ingest: audit file is closed")
	}

	if event.EventID == "" {
		event.EventID = id.NewUUIDv7()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	event.PrevHash = ""
	if f.hasPrev {
		event.PrevHash = hex.EncodeToString(f.prev[:])
	}
	event.Hash = ""
	hash, err := auditHash(f.prev, event)
	if err != nil {
		return err
	}
	event.Hash = hex.EncodeToString(hash[:])

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode audit event: %w", err)
	}
	if _, err := f.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("sync audit event: %w", err)
	}
	f.prev = hash
	f.hasPrev = true
	return nil
}

// Close closes the audit file.
func (f *FileAudit) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	return f.file.Close()
}

func auditHash(prev [sha256.Size]byte, event AuditEvent) ([sha256.Size]byte, error) {
	event.Hash = ""
	encoded, err := json.Marshal(event)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode audit event: %w", err)
	}
	digest := sha256.New()
	_, _ = digest.Write(prev[:])
	_, _ = digest.Write(encoded)
	var hash [sha256.Size]byte
	copy(hash[:], digest.Sum(nil))
	return hash, nil
}

// VerifyAuditFile checks the hash chain of an audit file.
func VerifyAuditFile(path string) error {
	return verifyAuditFile(path)
}

func verifyAuditFile(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open audit file: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReaderSize(file, 1<<20)
	var prev [sha256.Size]byte
	hasPrev := false
	lineNumber := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) == 0 {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			if readErr != nil {
				return fmt.Errorf("read audit file: %w", readErr)
			}
			continue
		}
		lineNumber++
		var event AuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("audit file line %d is not valid JSON: %w", lineNumber, err)
		}
		expectedPrev := ""
		if hasPrev {
			expectedPrev = hex.EncodeToString(prev[:])
		}
		if event.PrevHash != expectedPrev {
			return fmt.Errorf("audit file line %d breaks the hash chain: prev_hash %q, want %q", lineNumber, event.PrevHash, expectedPrev)
		}
		expected, err := auditHash(prev, event)
		if err != nil {
			return err
		}
		if event.Hash != hex.EncodeToString(expected[:]) {
			return fmt.Errorf("audit file line %d has an invalid hash", lineNumber)
		}
		prev = expected
		hasPrev = true
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read audit file: %w", readErr)
		}
	}
}

func lastAuditHash(path string) ([sha256.Size]byte, bool, error) {
	var hash [sha256.Size]byte
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return hash, false, nil
	}
	if err != nil {
		return hash, false, fmt.Errorf("read audit file: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return hash, false, nil
	}
	var event AuditEvent
	if err := json.Unmarshal([]byte(last), &event); err != nil {
		return hash, false, fmt.Errorf("parse last audit event: %w", err)
	}
	decoded, err := hex.DecodeString(event.Hash)
	if err != nil || len(decoded) != sha256.Size {
		return hash, false, errors.New("last audit event has an invalid hash")
	}
	copy(hash[:], decoded)
	return hash, true, nil
}
