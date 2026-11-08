package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/observaki/observaki/internal/alerting"
)

// AlertingStore implements alerting.Store on PostgreSQL (AD-32).
type AlertingStore struct {
	db *DB
}

// NewAlertingStore builds the alerting store.
func (db *DB) NewAlertingStore() *AlertingStore {
	return &AlertingStore{db: db}
}

var _ alerting.Store = (*AlertingStore)(nil)

// LoadRules reads the enabled rules and their channels.
func (s *AlertingStore) LoadRules(ctx context.Context) ([]alerting.Rule, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id::text, project_id::text, name, signal, query, condition, for_seconds, severity, enabled
		FROM alert_rules
		WHERE enabled
		ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: load alert rules: %w", err)
	}
	defer rows.Close()

	rules := make([]alerting.Rule, 0, 8)
	index := make(map[string]int)
	for rows.Next() {
		var rule alerting.Rule
		var condition []byte
		if err := rows.Scan(
			&rule.ID, &rule.ProjectID, &rule.Name, &rule.Signal, &rule.Query,
			&condition, &rule.ForSeconds, &rule.Severity, &rule.Enabled,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan alert rule: %w", err)
		}
		if err := json.Unmarshal(condition, &rule.Condition); err != nil {
			return nil, fmt.Errorf("postgres: decode condition for rule %s: %w", rule.ID, err)
		}
		index[rule.ID] = len(rules)
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate alert rules: %w", err)
	}

	channelRows, err := s.db.pool.Query(ctx, `
		SELECT arc.rule_id::text, c.id::text, c.name, c.kind, c.config
		FROM alert_rule_channels arc
		JOIN notification_channels c ON c.id = arc.channel_id
		ORDER BY c.name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: load alert channels: %w", err)
	}
	defer channelRows.Close()
	for channelRows.Next() {
		var ruleID string
		var channel alerting.Channel
		var config []byte
		if err := channelRows.Scan(&ruleID, &channel.ID, &channel.Name, &channel.Kind, &config); err != nil {
			return nil, fmt.Errorf("postgres: scan alert channel: %w", err)
		}
		channel.Config = json.RawMessage(config)
		if position, ok := index[ruleID]; ok {
			rules[position].Channels = append(rules[position].Channels, channel)
		}
	}
	if err := channelRows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate alert channels: %w", err)
	}
	return rules, nil
}

// LoadSilences reads the silences active at now.
func (s *AlertingStore) LoadSilences(ctx context.Context, now time.Time) ([]alerting.Silence, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id::text, matchers, starts_at, ends_at, reason
		FROM silences
		WHERE starts_at <= $1 AND ends_at > $1
		ORDER BY starts_at`, now)
	if err != nil {
		return nil, fmt.Errorf("postgres: load silences: %w", err)
	}
	defer rows.Close()

	silences := make([]alerting.Silence, 0, 4)
	for rows.Next() {
		var silence alerting.Silence
		var matchers []byte
		if err := rows.Scan(&silence.ID, &matchers, &silence.StartsAt, &silence.EndsAt, &silence.Reason); err != nil {
			return nil, fmt.Errorf("postgres: scan silence: %w", err)
		}
		if err := json.Unmarshal(matchers, &silence.Matchers); err != nil {
			return nil, fmt.Errorf("postgres: decode silence matchers: %w", err)
		}
		silences = append(silences, silence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate silences: %w", err)
	}
	return silences, nil
}

// LoadStates reads the current state of every rule.
func (s *AlertingStore) LoadStates(ctx context.Context) (map[string]alerting.State, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT rule_id::text, fingerprint, state, value, since, last_evaluated_at, state_version
		FROM alert_states`)
	if err != nil {
		return nil, fmt.Errorf("postgres: load alert states: %w", err)
	}
	defer rows.Close()

	states := make(map[string]alerting.State)
	for rows.Next() {
		var state alerting.State
		if err := rows.Scan(
			&state.RuleID, &state.Fingerprint, &state.State, &state.Value,
			&state.Since, &state.LastEvaluated, &state.StateVersion,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan alert state: %w", err)
		}
		states[state.RuleID] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate alert states: %w", err)
	}
	return states, nil
}

// SaveEvaluation persists the states and the outbox in one transaction.
func (s *AlertingStore) SaveEvaluation(ctx context.Context, states []alerting.State, notifications []alerting.Notification, evaluatedAt time.Time) error {
	tx, err := s.db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin alert evaluation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	for _, state := range states {
		if _, err := tx.Exec(ctx, `
			INSERT INTO alert_states (rule_id, fingerprint, state, value, since, last_evaluated_at, state_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (rule_id, fingerprint) DO UPDATE SET
				state = EXCLUDED.state,
				value = EXCLUDED.value,
				since = EXCLUDED.since,
				last_evaluated_at = EXCLUDED.last_evaluated_at,
				state_version = EXCLUDED.state_version`,
			state.RuleID, state.Fingerprint, state.State, state.Value, state.Since, state.LastEvaluated, state.StateVersion,
		); err != nil {
			return fmt.Errorf("postgres: save alert state %s: %w", state.RuleID, err)
		}
	}
	for _, notification := range notifications {
		if _, err := tx.Exec(ctx, `
			INSERT INTO alert_outbox
				(rule_id, channel_id, fingerprint, state_version, state, idempotency_key, payload, suppressed, next_attempt_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (idempotency_key) DO NOTHING`,
			notification.Rule.ID, notification.Channel.ID, notification.Fingerprint,
			notification.StateVersion, notification.State, notification.IdempotencyKey,
			string(notification.Payload), notification.Suppressed, evaluatedAt,
		); err != nil {
			return fmt.Errorf("postgres: insert alert outbox: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		"UPDATE alerting_heartbeat SET last_evaluation_at = $1, last_error = NULL WHERE id = true",
		evaluatedAt,
	); err != nil {
		return fmt.Errorf("postgres: update alerting heartbeat: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit alert evaluation: %w", err)
	}
	return nil
}

// Heartbeat records an evaluation failure without moving the dead-man timer.
func (s *AlertingStore) Heartbeat(ctx context.Context, at *time.Time, errText string) error {
	var err error
	if at == nil {
		_, err = s.db.pool.Exec(ctx,
			"UPDATE alerting_heartbeat SET last_error = NULLIF($1, '') WHERE id = true", errText)
	} else {
		_, err = s.db.pool.Exec(ctx,
			"UPDATE alerting_heartbeat SET last_evaluation_at = $1, last_error = NULLIF($2, '') WHERE id = true",
			*at, errText)
	}
	if err != nil {
		return fmt.Errorf("postgres: alerting heartbeat: %w", err)
	}
	return nil
}

// ClaimDeliveries claims a batch with SKIP LOCKED so several API replicas never
// send the same notification twice.
func (s *AlertingStore) ClaimDeliveries(ctx context.Context, now time.Time, limit int) ([]alerting.Delivery, error) {
	rows, err := s.db.pool.Query(ctx, `
		UPDATE alert_outbox o
		SET attempts = o.attempts + 1
		WHERE o.id IN (
			SELECT id FROM alert_outbox
			WHERE delivered_at IS NULL AND suppressed = false AND next_attempt_at <= $1
			ORDER BY id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING o.id, o.idempotency_key, o.payload, o.attempts, o.channel_id`,
		now, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: claim deliveries: %w", err)
	}
	defer rows.Close()

	type claimed struct {
		delivery  alerting.Delivery
		channelID string
	}
	claimedRows := make([]claimed, 0, limit)
	for rows.Next() {
		var row claimed
		var payload []byte
		if err := rows.Scan(&row.delivery.ID, &row.delivery.IdempotencyKey, &payload, &row.delivery.Attempts, &row.channelID); err != nil {
			return nil, fmt.Errorf("postgres: scan delivery: %w", err)
		}
		row.delivery.Payload = json.RawMessage(payload)
		claimedRows = append(claimedRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate deliveries: %w", err)
	}

	deliveries := make([]alerting.Delivery, 0, len(claimedRows))
	for _, row := range claimedRows {
		var channel alerting.Channel
		var config []byte
		if err := s.db.pool.QueryRow(ctx,
			"SELECT id::text, name, kind, config FROM notification_channels WHERE id = $1", row.channelID,
		).Scan(&channel.ID, &channel.Name, &channel.Kind, &config); err != nil {
			return nil, fmt.Errorf("postgres: load delivery channel: %w", err)
		}
		channel.Config = json.RawMessage(config)
		row.delivery.Channel = channel
		deliveries = append(deliveries, row.delivery)
	}
	return deliveries, nil
}

// MarkDelivered finalizes a delivery.
func (s *AlertingStore) MarkDelivered(ctx context.Context, id int64) error {
	if _, err := s.db.pool.Exec(ctx,
		"UPDATE alert_outbox SET delivered_at = now(), last_error = NULL WHERE id = $1", id,
	); err != nil {
		return fmt.Errorf("postgres: mark delivery delivered: %w", err)
	}
	return nil
}

// MarkDeliveryFailed schedules a retry with the given backoff.
func (s *AlertingStore) MarkDeliveryFailed(ctx context.Context, id int64, nextAttempt time.Time, message string) error {
	if _, err := s.db.pool.Exec(ctx,
		"UPDATE alert_outbox SET next_attempt_at = $2, last_error = $3 WHERE id = $1",
		id, nextAttempt, message,
	); err != nil {
		return fmt.Errorf("postgres: mark delivery failed: %w", err)
	}
	return nil
}

// Snapshot returns the dead-man's switch data.
func (s *AlertingStore) Snapshot(ctx context.Context) (alerting.StoreSnapshot, error) {
	var snapshot alerting.StoreSnapshot
	err := s.db.pool.QueryRow(ctx, `
		SELECT hb.last_evaluation_at, COALESCE(hb.last_error, ''),
			(SELECT count(*) FROM alert_states WHERE state = 'firing'),
			(SELECT count(*) FROM alert_states WHERE state = 'pending'),
			(SELECT count(*) FROM alert_outbox WHERE delivered_at IS NULL AND suppressed = false)
		FROM alerting_heartbeat hb
		WHERE hb.id = true`,
	).Scan(
		&snapshot.LastEvaluationAt, &snapshot.LastError,
		&snapshot.Firing, &snapshot.Pending, &snapshot.NotificationsPending,
	)
	if err != nil {
		return alerting.StoreSnapshot{}, fmt.Errorf("postgres: alerting snapshot: %w", err)
	}
	return snapshot, nil
}
