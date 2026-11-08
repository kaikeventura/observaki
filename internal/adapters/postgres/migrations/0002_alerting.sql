-- Minimal alerting state machine (AD-32).
--
-- alert_states holds the current state per rule/fingerprint and doubles as the
-- evaluation record. Every state transition increments state_version, which is
-- the idempotency key of the notification. alert_outbox is the transactional
-- outbox: a notification exists before it is sent, the notifier dedupes by
-- idempotency_key and delivery is at-least-once.

CREATE TABLE IF NOT EXISTS alert_states (
    rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('inactive', 'pending', 'firing', 'resolved')),
    value DOUBLE PRECISION NOT NULL DEFAULT 0,
    since TIMESTAMPTZ NOT NULL,
    last_evaluated_at TIMESTAMPTZ NOT NULL,
    state_version BIGINT NOT NULL DEFAULT 1,
    PRIMARY KEY (rule_id, fingerprint)
);

CREATE TABLE IF NOT EXISTS alert_outbox (
    id BIGSERIAL PRIMARY KEY,
    rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    state_version BIGINT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('firing', 'resolved')),
    idempotency_key TEXT NOT NULL UNIQUE,
    payload JSONB NOT NULL,
    suppressed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT
);

CREATE INDEX IF NOT EXISTS alert_outbox_pending_idx
    ON alert_outbox (next_attempt_at)
    WHERE delivered_at IS NULL AND suppressed = false;

CREATE INDEX IF NOT EXISTS alert_outbox_rule_idx ON alert_outbox (rule_id, created_at);

-- Dead-man's switch input (AD-24): the health page and the API metrics expose
-- the age of the last evaluation and fail visibly when alerting stops.
CREATE TABLE IF NOT EXISTS alerting_heartbeat (
    id BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    last_evaluation_at TIMESTAMPTZ,
    last_error TEXT
);

INSERT INTO alerting_heartbeat (id) VALUES (true) ON CONFLICT (id) DO NOTHING;
