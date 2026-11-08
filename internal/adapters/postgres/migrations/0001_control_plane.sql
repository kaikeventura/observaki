-- Control-plane metadata (AD-7, AD-20, AD-28, AD-35).
--
-- PostgreSQL is the source of truth for organizations, projects, users,
-- dashboards, alert rules, notification channels, tokens, silences, SLOs and
-- the append-only audit trail. The data plane never depends on this database to
-- serve telemetry (AD-3); the Query Service is the only read bridge.

CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    generation BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);

-- Local user directory. OIDC/SCIM remains the identity provider in Phase 3; the
-- row keeps the role assignment and the display metadata that memberships use.
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, email)
);

CREATE TABLE IF NOT EXISTS retention_policies (
    project_id UUID NOT NULL REFERENCES projects(id),
    signal TEXT NOT NULL CHECK (signal IN ('logs', 'traces', 'metrics')),
    retention_days INT NOT NULL CHECK (retention_days > 0),
    storage_tier TEXT NOT NULL DEFAULT 'hot',
    PRIMARY KEY (project_id, signal)
);

CREATE TABLE IF NOT EXISTS dashboards (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    title TEXT NOT NULL,
    definition JSONB NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_by UUID REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id),
    project_id UUID REFERENCES projects(id),
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('slack', 'webhook', 'email', 'pagerduty', 'opsgenie', 'teams')),
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    signal TEXT NOT NULL CHECK (signal IN ('metrics', 'logs', 'traces')),
    query TEXT NOT NULL,
    condition JSONB NOT NULL,
    for_seconds INT NOT NULL DEFAULT 300 CHECK (for_seconds >= 0),
    severity TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_rule_channels (
    rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    PRIMARY KEY (rule_id, channel_id)
);

-- project_id is nullable for organization-level memberships; NULLS NOT DISTINCT
-- keeps the composite key unique for those rows (PostgreSQL 15+).
CREATE TABLE IF NOT EXISTS memberships (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES organizations(id),
    project_id UUID REFERENCES projects(id),
    role TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'viewer')),
    CONSTRAINT memberships_scope_key UNIQUE NULLS NOT DISTINCT (user_id, org_id, project_id)
);

CREATE TABLE IF NOT EXISTS api_tokens (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS silences (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    matchers JSONB NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    created_by UUID REFERENCES users(id),
    CHECK (ends_at > starts_at)
);

CREATE TABLE IF NOT EXISTS slos (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id),
    service_name TEXT NOT NULL,
    sli JSONB NOT NULL,
    target NUMERIC NOT NULL,
    window_days INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS error_budgets (
    slo_id UUID PRIMARY KEY REFERENCES slos(id) ON DELETE CASCADE,
    consumed NUMERIC NOT NULL DEFAULT 0,
    remaining NUMERIC NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only audit trail; the application never updates or deletes a row
-- (AD-35). prev_hash/hash form the verifiable chain.
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    org_id UUID NOT NULL,
    actor_id UUID,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    metadata JSONB,
    prev_hash TEXT,
    hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS projects_org_idx ON projects (org_id);
CREATE INDEX IF NOT EXISTS users_org_idx ON users (org_id);
CREATE INDEX IF NOT EXISTS dashboards_project_idx ON dashboards (project_id);
CREATE INDEX IF NOT EXISTS alert_rules_project_idx ON alert_rules (project_id);
CREATE INDEX IF NOT EXISTS notification_channels_org_idx ON notification_channels (org_id);
CREATE INDEX IF NOT EXISTS api_tokens_project_idx ON api_tokens (project_id);
CREATE INDEX IF NOT EXISTS silences_project_window_idx ON silences (project_id, starts_at, ends_at);
CREATE INDEX IF NOT EXISTS slos_project_idx ON slos (project_id);
CREATE INDEX IF NOT EXISTS audit_events_org_created_idx ON audit_events (org_id, created_at);
