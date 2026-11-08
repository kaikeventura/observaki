package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/observaki/observaki/internal/api"
)

// TestIntegrationAPIStore runs only when OBSERVAKI_POSTGRES_TEST_URL points to
// a disposable PostgreSQL. It covers the token digest lookup, the project read
// and the digest-free token listing used by Story 5.5.
func TestIntegrationAPIStore(t *testing.T) {
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
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := db.SeedDev(ctx, "development"); err != nil {
		t.Fatalf("SeedDev() error = %v", err)
	}

	// An expired token must be indistinguishable from an unknown one.
	expiredID := "00000000-0000-4000-8000-0000000000e1"
	expiredHash := sha256.Sum256([]byte("expired-api-token"))
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO api_tokens (id, project_id, name, token_hash, scopes, expires_at)
		VALUES ($1, $2, 'Expired token', $3, ARRAY['query'], '2026-01-01T00:00:00Z')
		ON CONFLICT (id) DO UPDATE SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at`,
		expiredID, DevProjectID, hex.EncodeToString(expiredHash[:])); err != nil {
		t.Fatalf("insert expired token: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), "DELETE FROM api_tokens WHERE id = $1", expiredID)
	})

	store := db.NewAPIStore()

	digest := sha256.Sum256([]byte(DevIngestToken))
	principal, err := store.AuthenticateToken(ctx, digest)
	if err != nil {
		t.Fatalf("AuthenticateToken() error = %v", err)
	}
	if principal.ProjectID != DevProjectID || principal.OrgID != DevOrganizationID {
		t.Fatalf("principal = %+v", principal)
	}
	if !principal.HasScope("admin") || !principal.HasScope("query") || !principal.HasScope("ingest") {
		t.Fatalf("scopes = %v", principal.Scopes)
	}

	if _, err := store.AuthenticateToken(ctx, sha256.Sum256([]byte("wrong-token"))); !errors.Is(err, api.ErrTokenNotFound) {
		t.Fatalf("unknown token error = %v, want ErrTokenNotFound", err)
	}
	if _, err := store.AuthenticateToken(ctx, expiredHash); !errors.Is(err, api.ErrTokenNotFound) {
		t.Fatalf("expired token error = %v, want ErrTokenNotFound", err)
	}

	project, err := store.Project(ctx, DevProjectID)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if project.ID != DevProjectID || project.Slug != "dev" || project.Generation != 1 {
		t.Fatalf("project = %+v", project)
	}
	if _, err := store.Project(ctx, "00000000-0000-4000-8000-0000000000ff"); !errors.Is(err, api.ErrProjectNotFound) {
		t.Fatalf("missing project error = %v, want ErrProjectNotFound", err)
	}

	tokens, err := store.IngestTokens(ctx, DevProjectID)
	if err != nil {
		t.Fatalf("IngestTokens() error = %v", err)
	}
	var developmentToken *api.IngestToken
	for i := range tokens {
		if tokens[i].Name == "Development ingest token" {
			developmentToken = &tokens[i]
			break
		}
	}
	if developmentToken == nil {
		t.Fatalf("tokens = %+v, want the development token", tokens)
	}
	if len(developmentToken.Scopes) != 3 {
		t.Fatalf("token scopes = %v", developmentToken.Scopes)
	}
}
