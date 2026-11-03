-- 0004_tenancy.sql — row-level tenancy and authorization epoch (AD-3, AD-20).
--
-- Every telemetry row already carries tenant_id/project_id. AD-20 also requires
-- project_generation and authz_epoch in the storage key so a project move or an
-- authorization revocation invalidates old rows and old cache entries. The
-- columns are additive and default to 1, so rows written before this migration
-- remain readable under the initial generation/epoch.
--
-- Isolation has two layers:
--   1. The Query Service injects the tenant/project predicate into every query
--      (AD-3/AD-17). It never trusts a client header.
--   2. ClickHouse row policies are the backstop. The policy reads the caller's
--      scope from tenant_scope and compares tenant_id, project_generation and
--      authz_epoch. A user with a stale epoch sees nothing until the control
--      plane provisions the new scope.
--
-- tenant_scope is a small control-plane mapping, not telemetry. The Query
-- Service user is provisioned per tenant scope by the config reconciler
-- (AD-19) and has SELECT only on this row plus the telemetry tables. The
-- policy on tenant_scope hides the other users' rows.
--
-- The migration is idempotent: ADD COLUMN IF NOT EXISTS, CREATE ... IF NOT
-- EXISTS. It does not rewrite data. The row policies are attached to the
-- observaki_query role; a user must have only that role for the policy to be
-- fail-closed (multiple permissive roles are OR'ed by ClickHouse).
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS project_generation UInt64 DEFAULT 1;
ALTER TABLE otel_logs ADD COLUMN IF NOT EXISTS authz_epoch UInt64 DEFAULT 1;
ALTER TABLE otel_traces ADD COLUMN IF NOT EXISTS project_generation UInt64 DEFAULT 1;
ALTER TABLE otel_traces ADD COLUMN IF NOT EXISTS authz_epoch UInt64 DEFAULT 1;

CREATE TABLE IF NOT EXISTS tenant_scope
(
    ch_user             String,
    tenant_id           LowCardinality(String),
    project_id          LowCardinality(String),
    project_generation  UInt64,
    authz_epoch         UInt64,
    updated_at          DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
ORDER BY (ch_user, tenant_id, project_id)
SETTINGS index_granularity = 8192;

CREATE ROLE IF NOT EXISTS observaki_query;
GRANT SELECT ON otel_logs TO observaki_query;
GRANT SELECT ON otel_traces TO observaki_query;
GRANT SELECT ON tenant_scope TO observaki_query;

CREATE ROW POLICY IF NOT EXISTS tenant_scope_self ON tenant_scope
FOR SELECT USING ch_user = currentUser()
TO observaki_query;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_logs ON otel_logs
FOR SELECT USING
    tenant_id = (SELECT tenant_id FROM tenant_scope WHERE ch_user = currentUser())
    AND project_generation = (SELECT project_generation FROM tenant_scope WHERE ch_user = currentUser())
    AND authz_epoch = (SELECT authz_epoch FROM tenant_scope WHERE ch_user = currentUser())
TO observaki_query;

CREATE ROW POLICY IF NOT EXISTS tenant_isolation_traces ON otel_traces
FOR SELECT USING
    tenant_id = (SELECT tenant_id FROM tenant_scope WHERE ch_user = currentUser())
    AND project_generation = (SELECT project_generation FROM tenant_scope WHERE ch_user = currentUser())
    AND authz_epoch = (SELECT authz_epoch FROM tenant_scope WHERE ch_user = currentUser())
TO observaki_query;
