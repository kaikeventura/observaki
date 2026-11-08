package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/observaki/observaki/internal/alerting"
)

func TestIntegrationAlertingStore(t *testing.T) {
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
	if _, err := db.Pool().Exec(ctx, "TRUNCATE alert_states, alert_outbox"); err != nil {
		t.Fatalf("truncate alerting state: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "DELETE FROM silences WHERE id = '00000000-0000-4000-8000-0000000000aa'"); err != nil {
		t.Fatalf("remove drill silence: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE alert_rules SET for_seconds = 60 WHERE id = $1", DevAlertRuleID); err != nil {
		t.Fatalf("reset rule: %v", err)
	}

	store := db.NewAlertingStore()
	rules, err := store.LoadRules(ctx)
	if err != nil {
		t.Fatalf("LoadRules() error = %v", err)
	}
	if len(rules) != 1 || len(rules[0].Channels) != 1 {
		t.Fatalf("rules = %+v", rules)
	}
	rule := rules[0]
	if rule.Condition.Op != ">" || rule.Condition.Threshold != 10 {
		t.Fatalf("condition = %+v", rule.Condition)
	}

	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	silences, err := store.LoadSilences(ctx, now)
	if err != nil {
		t.Fatalf("LoadSilences(now) error = %v", err)
	}
	if len(silences) != 0 {
		t.Fatalf("expired silence leaked: %+v", silences)
	}
	silences, err = store.LoadSilences(ctx, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	if err != nil || len(silences) != 1 {
		t.Fatalf("LoadSilences(seed window) = %+v, %v", silences, err)
	}

	states, err := store.LoadStates(ctx)
	if err != nil {
		t.Fatalf("LoadStates() error = %v", err)
	}
	first := alerting.Evaluate(now, rules, states, nil, alerting.Metrics{IngestLagP95Seconds: 12})
	if first.States[0].State != alerting.StatePending {
		t.Fatalf("first state = %+v", first.States[0])
	}
	if err := store.SaveEvaluation(ctx, first.States, first.Notifications, now); err != nil {
		t.Fatalf("SaveEvaluation(pending) error = %v", err)
	}

	states, _ = store.LoadStates(ctx)
	second := alerting.Evaluate(now.Add(61*time.Second), rules, states, nil, alerting.Metrics{IngestLagP95Seconds: 12})
	if second.States[0].State != alerting.StateFiring || len(second.Notifications) != 1 {
		t.Fatalf("second = %+v", second)
	}
	if err := store.SaveEvaluation(ctx, second.States, second.Notifications, now.Add(61*time.Second)); err != nil {
		t.Fatalf("SaveEvaluation(firing) error = %v", err)
	}
	// Replaying the same transition must not duplicate the outbox row.
	if err := store.SaveEvaluation(ctx, second.States, second.Notifications, now.Add(62*time.Second)); err != nil {
		t.Fatalf("SaveEvaluation(replay) error = %v", err)
	}

	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Firing != 1 || snapshot.Pending != 0 || snapshot.NotificationsPending != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.LastEvaluationAt == nil {
		t.Fatal("heartbeat did not record the evaluation")
	}

	deliveries, err := store.ClaimDeliveries(ctx, now.Add(62*time.Second), 10)
	if err != nil {
		t.Fatalf("ClaimDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1 (dedupe failed)", len(deliveries))
	}
	if deliveries[0].Channel.Kind != "webhook" || !strings.Contains(deliveries[0].IdempotencyKey, "|2|"+deliveries[0].Channel.ID) {
		t.Fatalf("delivery = %+v", deliveries[0])
	}
	if err := store.MarkDelivered(ctx, deliveries[0].ID); err != nil {
		t.Fatalf("MarkDelivered() error = %v", err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if snapshot.NotificationsPending != 0 {
		t.Fatalf("pending after delivery = %d", snapshot.NotificationsPending)
	}
	empty, err := store.ClaimDeliveries(ctx, now.Add(62*time.Second), 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("second claim = %v, %v", empty, err)
	}

	// A silence suppresses the notification while the state still transitions.
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO silences (id, project_id, matchers, starts_at, ends_at, reason)
		VALUES ('00000000-0000-4000-8000-0000000000aa', $1, '{"rule_name":"Ingestion lag high"}'::jsonb, $2, $3, 'drill')
		ON CONFLICT DO NOTHING`, DevProjectID, now.Add(-time.Hour), now.Add(24*time.Hour)); err != nil {
		t.Fatalf("insert silence: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE alert_rules SET for_seconds = 0 WHERE id = $1", DevAlertRuleID); err != nil {
		t.Fatalf("update rule: %v", err)
	}
	rules, _ = store.LoadRules(ctx)
	silences, _ = store.LoadSilences(ctx, now.Add(3*time.Minute))

	states, _ = store.LoadStates(ctx)
	resolved := alerting.Evaluate(now.Add(2*time.Minute), rules, states, silences, alerting.Metrics{IngestLagP95Seconds: 1})
	if resolved.States[0].State != alerting.StateResolved || len(resolved.Notifications) != 1 {
		t.Fatalf("resolved = %+v", resolved)
	}
	// The silence also suppresses the resolution, as Alertmanager does.
	if !resolved.Notifications[0].Suppressed || resolved.Notifications[0].SuppressedBy != "drill" {
		t.Fatalf("resolution was not suppressed by the silence: %+v", resolved.Notifications[0])
	}
	if err := store.SaveEvaluation(ctx, resolved.States, resolved.Notifications, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("save resolved: %v", err)
	}
	states, _ = store.LoadStates(ctx)
	firing := alerting.Evaluate(now.Add(3*time.Minute), rules, states, silences, alerting.Metrics{IngestLagP95Seconds: 12})
	if firing.States[0].State != alerting.StateFiring || len(firing.Notifications) != 1 {
		t.Fatalf("firing = %+v", firing)
	}
	if !firing.Notifications[0].Suppressed || firing.Notifications[0].SuppressedBy != "drill" {
		t.Fatalf("notification was not suppressed: %+v", firing.Notifications[0])
	}
	if err := store.SaveEvaluation(ctx, firing.States, firing.Notifications, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("save firing: %v", err)
	}
	// The suppressed row is not claimable.
	claim, err := store.ClaimDeliveries(ctx, now.Add(4*time.Minute), 10)
	if err != nil {
		t.Fatalf("claim suppressed: %v", err)
	}
	for _, delivery := range claim {
		if delivery.IdempotencyKey == firing.Notifications[0].IdempotencyKey {
			t.Fatal("suppressed notification was claimed for delivery")
		}
	}

	// The dead-man heartbeat keeps the last evaluation on error.
	before := *snapshot.LastEvaluationAt
	if err := store.Heartbeat(ctx, nil, "evaluator exploded"); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if snapshot.LastError != "evaluator exploded" {
		t.Fatalf("last error = %q", snapshot.LastError)
	}
	if snapshot.LastEvaluationAt == nil || !snapshot.LastEvaluationAt.After(before.Add(-time.Second)) {
		t.Fatalf("heartbeat moved the evaluation time: %+v", snapshot.LastEvaluationAt)
	}
}
