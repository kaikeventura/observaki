// Package alerting implements the minimal internal alert path (AD-32):
// threshold evaluation, state transitions, transactional outbox, dedupe and
// silences. It is deliberately scoped to the built-in metrics of Phase 0; the
// full rule compiler and the external channels arrive in Phase 1.
package alerting

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Supported internal query names.
const (
	QueryIngestLagP95     = "ingest_lag_p95_seconds"
	QueryIngestPending    = "ingest_pending_messages"
	QueryIngestDLQ        = "ingest_dlq_messages"
	QueryQueryErrorRate   = "query_error_rate"
	QueryWriterConnected  = "writer_connected"
	QueryGatewayConnected = "gateway_connected"
	QueryComponentDown    = "component_down"
)

// Alert states.
const (
	StateInactive = "inactive"
	StatePending  = "pending"
	StateFiring   = "firing"
	StateResolved = "resolved"
)

// Condition is the threshold condition stored in alert_rules.condition.
type Condition struct {
	Op        string  `json:"op"`
	Threshold float64 `json:"threshold"`
	Component string  `json:"component,omitempty"`
}

// Channel is a notification destination.
type Channel struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Kind   string          `json:"kind"`
	Config json.RawMessage `json:"config"`
}

// Rule is an enabled alert rule.
type Rule struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	Signal     string    `json:"signal"`
	Query      string    `json:"query"`
	Condition  Condition `json:"condition"`
	ForSeconds int       `json:"for_seconds"`
	Severity   string    `json:"severity"`
	Enabled    bool      `json:"enabled"`
	Channels   []Channel `json:"channels"`
}

// Silence mutes matching labels inside a time window.
type Silence struct {
	ID       string            `json:"id"`
	Matchers map[string]string `json:"matchers"`
	StartsAt time.Time         `json:"starts_at"`
	EndsAt   time.Time         `json:"ends_at"`
	Reason   string            `json:"reason"`
}

// State is the persisted state of one rule/fingerprint.
type State struct {
	RuleID        string    `json:"rule_id"`
	Fingerprint   string    `json:"fingerprint"`
	State         string    `json:"state"`
	Value         float64   `json:"value"`
	Since         time.Time `json:"since"`
	LastEvaluated time.Time `json:"last_evaluated_at"`
	StateVersion  int64     `json:"state_version"`
}

// Metrics is the snapshot the evaluator reads. Zero values are valid.
type Metrics struct {
	IngestLagP95Seconds   float64
	IngestPendingMessages float64
	IngestDLQMessages     float64
	QueryErrorRate        float64
	WriterConnected       bool
	GatewayConnected      bool
	Components            map[string]string
}

// Payload is the generic webhook body and the basis of the Slack message.
type Payload struct {
	Version        int               `json:"version"`
	RuleID         string            `json:"rule_id"`
	RuleName       string            `json:"rule_name"`
	ProjectID      string            `json:"project_id"`
	Severity       string            `json:"severity"`
	State          string            `json:"state"`
	Query          string            `json:"query"`
	Value          float64           `json:"value"`
	Threshold      float64           `json:"threshold"`
	Since          time.Time         `json:"since"`
	EvaluatedAt    time.Time         `json:"evaluated_at"`
	Summary        string            `json:"summary"`
	Labels         map[string]string `json:"labels"`
	IdempotencyKey string            `json:"idempotency_key"`
}

// Notification is one outbox row to write.
type Notification struct {
	Rule           Rule
	Channel        Channel
	Fingerprint    string
	State          string
	StateVersion   int64
	Value          float64
	Threshold      float64
	Since          time.Time
	EvaluatedAt    time.Time
	Summary        string
	Labels         map[string]string
	IdempotencyKey string
	Payload        json.RawMessage
	Suppressed     bool
	SuppressedBy   string
}

// SkippedRule explains why a rule was not evaluated.
type SkippedRule struct {
	RuleID string
	Reason string
}

// Result is the evaluator output.
type Result struct {
	States        []State
	Notifications []Notification
	Skipped       []SkippedRule
}

// Evaluate runs the state machine for every enabled rule. It is pure: the
// caller persists the returned states and notifications in one transaction.
func Evaluate(now time.Time, rules []Rule, previous map[string]State, silences []Silence, metrics Metrics) Result {
	result := Result{
		States:        make([]State, 0, len(rules)),
		Notifications: make([]Notification, 0, len(rules)),
	}
	for _, rule := range rules {
		if !rule.Enabled {
			result.Skipped = append(result.Skipped, SkippedRule{RuleID: rule.ID, Reason: "rule is disabled"})
			continue
		}
		if rule.Signal != "metrics" {
			result.Skipped = append(result.Skipped, SkippedRule{RuleID: rule.ID, Reason: "only metrics rules are evaluated in Phase 0"})
			continue
		}
		value, ok := metrics.value(rule)
		if !ok {
			result.Skipped = append(result.Skipped, SkippedRule{RuleID: rule.ID, Reason: "unsupported internal query " + rule.Query})
			continue
		}

		fingerprint := rule.ID
		state, ok := previous[rule.ID]
		if !ok {
			state = State{
				RuleID:      rule.ID,
				Fingerprint: fingerprint,
				State:       StateInactive,
				Since:       now,
			}
		}
		breach := compare(value, rule.Condition)
		notify := false

		switch {
		case breach && (state.State == StateInactive || state.State == StateResolved || state.State == ""):
			state.Since = now
			state.StateVersion++
			if rule.ForSeconds <= 0 {
				state.State = StateFiring
				notify = true
			} else {
				state.State = StatePending
			}
		case breach && state.State == StatePending:
			if now.Sub(state.Since) >= time.Duration(rule.ForSeconds)*time.Second {
				state.State = StateFiring
				state.StateVersion++
				notify = true
			}
		case breach && state.State == StateFiring:
			// Still firing; keep the version and the original since.
		case !breach && state.State == StateFiring:
			state.State = StateResolved
			state.StateVersion++
			state.Since = now
			notify = true
		case !breach && state.State == StatePending:
			state.State = StateInactive
			state.StateVersion++
			state.Since = now
		}

		state.Value = value
		state.LastEvaluated = now
		result.States = append(result.States, state)

		if !notify {
			continue
		}
		labels := ruleLabels(rule)
		silencedBy := matchSilence(labels, silences, now)
		summary := summarize(rule, state.State, value)
		for _, channel := range rule.Channels {
			payload := Payload{
				Version:        1,
				RuleID:         rule.ID,
				RuleName:       rule.Name,
				ProjectID:      rule.ProjectID,
				Severity:       rule.Severity,
				State:          state.State,
				Query:          rule.Query,
				Value:          value,
				Threshold:      rule.Condition.Threshold,
				Since:          state.Since,
				EvaluatedAt:    now,
				Summary:        summary,
				Labels:         labels,
				IdempotencyKey: idempotencyKey(rule.ID, fingerprint, state.StateVersion, channel.ID),
			}
			raw, _ := json.Marshal(payload)
			result.Notifications = append(result.Notifications, Notification{
				Rule:           rule,
				Channel:        channel,
				Fingerprint:    fingerprint,
				State:          state.State,
				StateVersion:   state.StateVersion,
				Value:          value,
				Threshold:      rule.Condition.Threshold,
				Since:          state.Since,
				EvaluatedAt:    now,
				Summary:        summary,
				Labels:         labels,
				IdempotencyKey: payload.IdempotencyKey,
				Payload:        raw,
				Suppressed:     silencedBy != "",
				SuppressedBy:   silencedBy,
			})
		}
	}
	return result
}

func (m Metrics) value(rule Rule) (float64, bool) {
	switch rule.Query {
	case QueryIngestLagP95:
		return m.IngestLagP95Seconds, true
	case QueryIngestPending:
		return m.IngestPendingMessages, true
	case QueryIngestDLQ:
		return m.IngestDLQMessages, true
	case QueryQueryErrorRate:
		return m.QueryErrorRate, true
	case QueryWriterConnected:
		return boolValue(m.WriterConnected), true
	case QueryGatewayConnected:
		return boolValue(m.GatewayConnected), true
	case QueryComponentDown:
		status := m.Components[rule.Condition.Component]
		switch status {
		case "critical", "unknown", "":
			return 1, true
		default:
			return 0, true
		}
	default:
		return 0, false
	}
}

func boolValue(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func compare(value float64, condition Condition) bool {
	switch condition.Op {
	case ">":
		return value > condition.Threshold
	case ">=":
		return value >= condition.Threshold
	case "<":
		return value < condition.Threshold
	case "<=":
		return value <= condition.Threshold
	default:
		return false
	}
}

func ruleLabels(rule Rule) map[string]string {
	return map[string]string{
		"rule_id":    rule.ID,
		"rule_name":  rule.Name,
		"severity":   rule.Severity,
		"project_id": rule.ProjectID,
	}
}

// matchSilence returns the reason of the first active silence whose matchers
// all match the labels, or "" when the alert is not silenced.
func matchSilence(labels map[string]string, silences []Silence, now time.Time) string {
	for _, silence := range silences {
		if now.Before(silence.StartsAt) || !now.Before(silence.EndsAt) {
			continue
		}
		matched := true
		for key, value := range silence.Matchers {
			if labels[key] != value {
				matched = false
				break
			}
		}
		if matched {
			return silence.Reason
		}
	}
	return ""
}

func idempotencyKey(ruleID, fingerprint string, version int64, channelID string) string {
	return fmt.Sprintf("%s|%s|%d|%s", ruleID, fingerprint, version, channelID)
}

func summarize(rule Rule, state string, value float64) string {
	verb := "is firing"
	if state == StateResolved {
		verb = "resolved"
	}
	switch rule.Query {
	case QueryIngestLagP95:
		return fmt.Sprintf("%s %s: ingestion lag p95 is %s (threshold %s)",
			rule.Name, verb, formatDuration(value), formatDuration(rule.Condition.Threshold))
	case QueryIngestPending:
		return fmt.Sprintf("%s %s: %s pending WAL messages (threshold %s)",
			rule.Name, verb, formatNumber(value), formatNumber(rule.Condition.Threshold))
	case QueryIngestDLQ:
		return fmt.Sprintf("%s %s: %s dead-letter messages (threshold %s)",
			rule.Name, verb, formatNumber(value), formatNumber(rule.Condition.Threshold))
	case QueryQueryErrorRate:
		return fmt.Sprintf("%s %s: query error rate is %.2f%% (threshold %.2f%%)",
			rule.Name, verb, value*100, rule.Condition.Threshold*100)
	case QueryWriterConnected:
		return fmt.Sprintf("%s %s: writer WAL connection is %s (threshold %s)",
			rule.Name, verb, connectedLabel(value), connectedLabel(rule.Condition.Threshold))
	case QueryGatewayConnected:
		return fmt.Sprintf("%s %s: gateway WAL connection is %s (threshold %s)",
			rule.Name, verb, connectedLabel(value), connectedLabel(rule.Condition.Threshold))
	case QueryComponentDown:
		return fmt.Sprintf("%s %s: component %q is down (threshold %s)",
			rule.Name, verb, rule.Condition.Component, formatNumber(rule.Condition.Threshold))
	default:
		return fmt.Sprintf("%s %s: value %s (threshold %s)",
			rule.Name, verb, formatNumber(value), formatNumber(rule.Condition.Threshold))
	}
}

func formatDuration(seconds float64) string {
	return time.Duration(seconds * float64(time.Second)).Round(time.Millisecond).String()
}

func formatNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func connectedLabel(value float64) string {
	if value >= 0.5 {
		return "connected"
	}
	return "disconnected"
}

// SortedChannelIDs is a deterministic helper used by tests and logs.
func SortedChannelIDs(rule Rule) []string {
	ids := make([]string, 0, len(rule.Channels))
	for _, channel := range rule.Channels {
		ids = append(ids, channel.ID)
	}
	sort.Strings(ids)
	return ids
}
