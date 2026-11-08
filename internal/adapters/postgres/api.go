package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/observaki/observaki/internal/api"
)

// APIStore is the read-only control-plane projection the HTTP API needs
// (AD-7). It never exposes token digests and never reads telemetry (AD-3).
type APIStore struct {
	db *DB
}

// NewAPIStore builds the API store on a control-plane connection.
func (db *DB) NewAPIStore() *APIStore {
	return &APIStore{db: db}
}

var _ api.ControlPlane = (*APIStore)(nil)

// AuthenticateToken resolves a SHA-256 digest to its project and scopes. An
// unknown or expired digest is ErrTokenNotFound so the API cannot leak which
// case it was.
func (s *APIStore) AuthenticateToken(ctx context.Context, tokenHash [sha256.Size]byte) (api.Principal, error) {
	var principal api.Principal
	err := s.db.pool.QueryRow(ctx, `
		SELECT t.id::text, p.org_id::text, t.project_id::text, t.scopes
		FROM api_tokens t
		JOIN projects p ON p.id = t.project_id
		WHERE t.token_hash = $1
		  AND (t.expires_at IS NULL OR t.expires_at > now())`,
		hex.EncodeToString(tokenHash[:]),
	).Scan(&principal.TokenID, &principal.OrgID, &principal.ProjectID, &principal.Scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.Principal{}, api.ErrTokenNotFound
	}
	if err != nil {
		return api.Principal{}, fmt.Errorf("postgres: authenticate API token: %w", err)
	}
	return principal, nil
}

// Project reads one project.
func (s *APIStore) Project(ctx context.Context, projectID string) (api.Project, error) {
	var project api.Project
	err := s.db.pool.QueryRow(ctx, `
		SELECT id::text, org_id::text, name, slug, generation, created_at
		FROM projects
		WHERE id = $1`, projectID,
	).Scan(&project.ID, &project.OrgID, &project.Name, &project.Slug, &project.Generation, &project.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return api.Project{}, api.ErrProjectNotFound
	}
	if err != nil {
		return api.Project{}, fmt.Errorf("postgres: read project %s: %w", projectID, err)
	}
	return project, nil
}

// IngestTokens lists token metadata for a project. The SELECT never includes
// token_hash (AD-19).
func (s *APIStore) IngestTokens(ctx context.Context, projectID string) ([]api.IngestToken, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id::text, name, scopes, expires_at, created_at
		FROM api_tokens
		WHERE project_id = $1
		ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list ingestion tokens: %w", err)
	}
	defer rows.Close()

	tokens := make([]api.IngestToken, 0, 4)
	for rows.Next() {
		var token api.IngestToken
		if err := rows.Scan(&token.ID, &token.Name, &token.Scopes, &token.ExpiresAt, &token.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan ingestion token: %w", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate ingestion tokens: %w", err)
	}
	return tokens, nil
}
