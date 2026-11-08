package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Development metadata ids. They are fixed so the Compose/Helm evaluation stack
// and the operator drills address the same rows.
const (
	DevOrganizationID = "00000000-0000-4000-8000-000000000001"
	DevProjectID      = "00000000-0000-4000-8000-000000000002"
	DevUserID         = "00000000-0000-4000-8000-000000000003"
	DevDashboardID    = "00000000-0000-4000-8000-000000000004"
	DevChannelID      = "00000000-0000-4000-8000-000000000005"
	DevAlertRuleID    = "00000000-0000-4000-8000-000000000006"
	DevSilenceID      = "00000000-0000-4000-8000-000000000007"
	DevSLOID          = "00000000-0000-4000-8000-000000000008"
	DevAPITokenID     = "00000000-0000-4000-8000-000000000009"
	DevMembershipID   = "00000000-0000-4000-8000-00000000000a"
)

// DevIngestToken is the public evaluation token; only its SHA-256 is stored in
// the metadata (AD-19). Never reuse it outside a local stack.
const DevIngestToken = "observaki-local-dev-token"

// SeedDev inserts the evaluation metadata used by the single-node stack and the
// Story 5.4 drills. It is idempotent and refuses to run outside development or
// test, where a public token and a public webhook channel must not exist.
func (db *DB) SeedDev(ctx context.Context, environment string) error {
	if environment != "development" && environment != "test" {
		return fmt.Errorf("postgres: refusing to seed development metadata in environment %q", environment)
	}
	hash := sha256.Sum256([]byte(DevIngestToken))
	tokenHash := hex.EncodeToString(hash[:])

	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, slug) VALUES ($1, 'Development', 'dev') ON CONFLICT DO NOTHING`,
			[]any{DevOrganizationID}},
		{`INSERT INTO users (id, org_id, email, display_name)
		  VALUES ($1, $2, 'dev@observaki.local', 'Development User') ON CONFLICT DO NOTHING`,
			[]any{DevUserID, DevOrganizationID}},
		{`INSERT INTO projects (id, org_id, name, slug, generation)
		  VALUES ($1, $2, 'Development', 'dev', 1) ON CONFLICT DO NOTHING`,
			[]any{DevProjectID, DevOrganizationID}},
		{`INSERT INTO memberships (id, user_id, org_id, project_id, role)
		  VALUES ($1, $2, $3, $4, 'admin') ON CONFLICT DO NOTHING`,
			[]any{DevMembershipID, DevUserID, DevOrganizationID, DevProjectID}},
		{`INSERT INTO retention_policies (project_id, signal, retention_days, storage_tier)
		  VALUES ($1, 'logs', 15, 'hot'), ($1, 'traces', 15, 'hot'), ($1, 'metrics', 30, 'hot')
		  ON CONFLICT DO NOTHING`, []any{DevProjectID}},
		{`INSERT INTO dashboards (id, project_id, title, definition, version, created_by)
		  VALUES ($1, $2, 'Development overview',
		          '{"schemaVersion":1,"panels":[{"title":"Ingest rate","type":"timeseries","query":"sum(rate(observaki_gateway_events_total[5m]))"}]}'::jsonb,
		          1, $3) ON CONFLICT DO NOTHING`,
			[]any{DevDashboardID, DevProjectID, DevUserID}},
		{`INSERT INTO notification_channels (id, org_id, project_id, name, kind, config)
		  VALUES ($1, $2, $3, 'Local webhook', 'webhook',
		          '{"url":"http://host.docker.internal:19090/alert","headers":{"X-Observaki-Test":"1"}}'::jsonb)
		  ON CONFLICT DO NOTHING`,
			[]any{DevChannelID, DevOrganizationID, DevProjectID}},
		{`INSERT INTO alert_rules (id, project_id, name, signal, query, condition, for_seconds, severity, enabled)
		  VALUES ($1, $2, 'Ingestion lag high', 'metrics', 'ingest_lag_p95_seconds',
		          '{"op":">","threshold":10}'::jsonb, 60, 'warning', true)
		  ON CONFLICT DO NOTHING`,
			[]any{DevAlertRuleID, DevProjectID}},
		{`INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			[]any{DevAlertRuleID, DevChannelID}},
		{`INSERT INTO api_tokens (id, project_id, name, token_hash, scopes, expires_at)
		  VALUES ($1, $2, 'Development ingest token', $3, ARRAY['ingest','query','admin'], '2126-01-01T00:00:00Z')
		  ON CONFLICT DO NOTHING`,
			[]any{DevAPITokenID, DevProjectID, tokenHash}},
		// An expired silence gives the backup a silence row without muting the
		// seeded rule.
		{`INSERT INTO silences (id, project_id, matchers, starts_at, ends_at, reason, created_by)
		  VALUES ($1, $2, '{"rule_name":"Maintenance"}'::jsonb,
		          '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z', 'Example expired silence', $3)
		  ON CONFLICT DO NOTHING`,
			[]any{DevSilenceID, DevProjectID, DevUserID}},
		{`INSERT INTO slos (id, project_id, service_name, sli, target, window_days)
		  VALUES ($1, $2, 'gateway', '{"type":"availability"}'::jsonb, 0.999, 30)
		  ON CONFLICT DO NOTHING`,
			[]any{DevSLOID, DevProjectID}},
		{`INSERT INTO error_budgets (slo_id, consumed, remaining) VALUES ($1, 0, 1) ON CONFLICT DO NOTHING`,
			[]any{DevSLOID}},
	}

	for _, statement := range statements {
		if _, err := db.pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			return fmt.Errorf("postgres: seed development metadata: %w", err)
		}
	}
	return nil
}
