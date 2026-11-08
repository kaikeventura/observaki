package alerting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testRule() Rule {
	return Rule{
		ID:         "rule-1",
		ProjectID:  "project-1",
		Name:       "Ingestion lag high",
		Signal:     "metrics",
		Query:      QueryIngestLagP95,
		Condition:  Condition{Op: ">", Threshold: 10},
		ForSeconds: 60,
		Severity:   "warning",
		Enabled:    true,
		Channels: []Channel{{
			ID: "channel-1", Name: "Local webhook", Kind: "webhook",
			Config: json.RawMessage(`{"url":"http://example.test/alert"}`),
		}},
	}
}

func TestEvaluateFiresAfterForAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	rule := testRule()
	metrics := Metrics{IngestLagP95Seconds: 12}

	first := Evaluate(now, []Rule{rule}, nil, nil, metrics)
	if len(first.States) != 1 || first.States[0].State != StatePending {
		t.Fatalf("first state = %+v", first.States)
	}
	if len(first.Notifications) != 0 {
		t.Fatalf("pending emitted %d notifications", len(first.Notifications))
	}

	second := Evaluate(now.Add(30*time.Second), []Rule{rule}, map[string]State{rule.ID: first.States[0]}, nil, metrics)
	if second.States[0].State != StatePending || second.States[0].StateVersion != 1 {
		t.Fatalf("second state = %+v", second.States[0])
	}

	third := Evaluate(now.Add(61*time.Second), []Rule{rule}, map[string]State{rule.ID: second.States[0]}, nil, metrics)
	if third.States[0].State != StateFiring || third.States[0].StateVersion != 2 {
		t.Fatalf("third state = %+v", third.States[0])
	}
	if len(third.Notifications) != 1 {
		t.Fatalf("firing notifications = %d, want 1", len(third.Notifications))
	}
	notification := third.Notifications[0]
	if notification.Suppressed {
		t.Fatal("notification was suppressed without a silence")
	}
	if !strings.Contains(notification.Summary, "ingestion lag p95 is 12s") {
		t.Fatalf("summary = %q", notification.Summary)
	}
	var payload Payload
	if err := json.Unmarshal(notification.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.State != StateFiring || payload.Threshold != 10 || payload.RuleName != rule.Name {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.IdempotencyKey != notification.IdempotencyKey || !strings.Contains(notification.IdempotencyKey, "|2|channel-1") {
		t.Fatalf("idempotency key = %q", notification.IdempotencyKey)
	}

	// A still-firing evaluation must not bump the version or emit again.
	fourth := Evaluate(now.Add(90*time.Second), []Rule{rule}, map[string]State{rule.ID: third.States[0]}, nil, metrics)
	if fourth.States[0].State != StateFiring || fourth.States[0].StateVersion != 2 {
		t.Fatalf("stable firing state = %+v", fourth.States[0])
	}
	if len(fourth.Notifications) != 0 {
		t.Fatalf("stable firing emitted %d notifications", len(fourth.Notifications))
	}

	// Resolving emits one resolved notification with a new version.
	resolved := Evaluate(now.Add(120*time.Second), []Rule{rule}, map[string]State{rule.ID: fourth.States[0]}, nil, Metrics{IngestLagP95Seconds: 1})
	if resolved.States[0].State != StateResolved || resolved.States[0].StateVersion != 3 {
		t.Fatalf("resolved state = %+v", resolved.States[0])
	}
	if len(resolved.Notifications) != 1 || resolved.Notifications[0].State != StateResolved {
		t.Fatalf("resolve notifications = %+v", resolved.Notifications)
	}
}

func TestEvaluateZeroForFiresImmediately(t *testing.T) {
	rule := testRule()
	rule.ForSeconds = 0
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	result := Evaluate(now, []Rule{rule}, nil, nil, Metrics{IngestLagP95Seconds: 11})
	if result.States[0].State != StateFiring || result.States[0].StateVersion != 1 {
		t.Fatalf("state = %+v", result.States[0])
	}
	if len(result.Notifications) != 1 {
		t.Fatalf("notifications = %d", len(result.Notifications))
	}
}

func TestEvaluateSilence(t *testing.T) {
	rule := testRule()
	rule.ForSeconds = 0
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	silences := []Silence{{
		ID: "silence-1", Matchers: map[string]string{"rule_name": rule.Name},
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(time.Hour), Reason: "maintenance",
	}}
	result := Evaluate(now, []Rule{rule}, nil, silences, Metrics{IngestLagP95Seconds: 12})
	if result.States[0].State != StateFiring {
		t.Fatalf("state = %+v", result.States[0])
	}
	if len(result.Notifications) != 1 || !result.Notifications[0].Suppressed {
		t.Fatalf("notifications = %+v", result.Notifications)
	}
	if result.Notifications[0].SuppressedBy != "maintenance" {
		t.Fatalf("suppressed by = %q", result.Notifications[0].SuppressedBy)
	}

	// An expired silence does not suppress.
	expired := []Silence{{
		ID: "silence-2", Matchers: map[string]string{"rule_name": rule.Name},
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour),
	}}
	notSilenced := Evaluate(now, []Rule{rule}, nil, expired, Metrics{IngestLagP95Seconds: 12})
	if notSilenced.Notifications[0].Suppressed {
		t.Fatal("expired silence suppressed the notification")
	}
}

func TestEvaluateSkipsUnsupportedRules(t *testing.T) {
	rule := testRule()
	rule.Query = "promql:up"
	result := Evaluate(time.Now(), []Rule{rule}, nil, nil, Metrics{})
	if len(result.States) != 0 {
		t.Fatalf("states = %+v", result.States)
	}
	if len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "unsupported") {
		t.Fatalf("skipped = %+v", result.Skipped)
	}

	rule.Signal = "logs"
	rule.Query = QueryIngestLagP95
	result = Evaluate(time.Now(), []Rule{rule}, nil, nil, Metrics{})
	if len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "metrics rules") {
		t.Fatalf("skipped = %+v", result.Skipped)
	}
}

func TestComponentDownMetric(t *testing.T) {
	rule := testRule()
	rule.Query = QueryComponentDown
	rule.Condition = Condition{Op: ">", Threshold: 0.5, Component: "clickhouse"}
	rule.ForSeconds = 0

	result := Evaluate(time.Now(), []Rule{rule}, nil, nil, Metrics{Components: map[string]string{"clickhouse": "critical"}})
	if result.States[0].State != StateFiring {
		t.Fatalf("state = %+v", result.States[0])
	}
	result = Evaluate(time.Now(), []Rule{rule}, nil, nil, Metrics{Components: map[string]string{"clickhouse": "healthy"}})
	if len(result.States) != 1 || result.States[0].State != StateInactive {
		t.Fatalf("healthy state = %+v", result.States)
	}
}

func TestDeliveryBackoff(t *testing.T) {
	tests := map[int]time.Duration{
		1: 5 * time.Second,
		2: 10 * time.Second,
		3: 20 * time.Second,
		6: 160 * time.Second,
		9: maxDeliveryBackoff,
	}
	for attempts, want := range tests {
		if got := deliveryBackoff(attempts); got != want {
			t.Fatalf("deliveryBackoff(%d) = %s, want %s", attempts, got, want)
		}
	}
}
