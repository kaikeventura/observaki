package alerting

import (
	"context"
	"encoding/json"
	"time"
)

// Delivery is one claimed outbox row.
type Delivery struct {
	ID             int64
	IdempotencyKey string
	Channel        Channel
	Payload        json.RawMessage
	Attempts       int
}

// StoreSnapshot is the dead-man's switch view.
type StoreSnapshot struct {
	LastEvaluationAt     *time.Time
	LastError            string
	Firing               int
	Pending              int
	NotificationsPending int
}

// Store persists rules, states and the outbox (AD-32).
type Store interface {
	LoadRules(ctx context.Context) ([]Rule, error)
	LoadSilences(ctx context.Context, now time.Time) ([]Silence, error)
	LoadStates(ctx context.Context) (map[string]State, error)
	SaveEvaluation(ctx context.Context, states []State, notifications []Notification, evaluatedAt time.Time) error
	Heartbeat(ctx context.Context, at *time.Time, errText string) error
	ClaimDeliveries(ctx context.Context, now time.Time, limit int) ([]Delivery, error)
	MarkDelivered(ctx context.Context, id int64) error
	MarkDeliveryFailed(ctx context.Context, id int64, nextAttempt time.Time, message string) error
	Snapshot(ctx context.Context) (StoreSnapshot, error)
}
