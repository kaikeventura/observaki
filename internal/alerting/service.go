package alerting

import (
	"context"
	"log/slog"
	"time"

	"github.com/observaki/observaki/internal/selfobs"
)

// Service evaluates rules and drains the notification outbox.
type Service struct {
	store            Store
	metrics          func(context.Context) Metrics
	notifier         *Notifier
	interval         time.Duration
	deliveryInterval time.Duration
	logger           *slog.Logger
	now              func() time.Time
}

// NewService builds the evaluator loop.
func NewService(store Store, metrics func(context.Context) Metrics, notifier *Notifier, interval, deliveryInterval time.Duration, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if deliveryInterval <= 0 {
		deliveryInterval = 5 * time.Second
	}
	return &Service{
		store:            store,
		metrics:          metrics,
		notifier:         notifier,
		interval:         interval,
		deliveryInterval: deliveryInterval,
		logger:           logger,
		now:              time.Now,
	}
}

// Run evaluates immediately, then on the configured intervals until ctx ends.
func (s *Service) Run(ctx context.Context) {
	evaluation := time.NewTicker(s.interval)
	defer evaluation.Stop()
	delivery := time.NewTicker(s.deliveryInterval)
	defer delivery.Stop()

	s.evaluate(ctx)
	s.deliver(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-evaluation.C:
			s.evaluate(ctx)
			s.deliver(ctx)
		case <-delivery.C:
			s.deliver(ctx)
		}
	}
}

func (s *Service) evaluate(ctx context.Context) {
	now := s.now().UTC()
	rules, err := s.store.LoadRules(ctx)
	if err != nil {
		s.logger.Error("alerting: load rules", "error", err)
		s.heartbeat(ctx, nil, err.Error())
		return
	}
	silences, err := s.store.LoadSilences(ctx, now)
	if err != nil {
		s.logger.Error("alerting: load silences", "error", err)
		s.heartbeat(ctx, nil, err.Error())
		return
	}
	states, err := s.store.LoadStates(ctx)
	if err != nil {
		s.logger.Error("alerting: load states", "error", err)
		s.heartbeat(ctx, nil, err.Error())
		return
	}

	result := Evaluate(now, rules, states, silences, s.metrics(ctx))
	if err := s.store.SaveEvaluation(ctx, result.States, result.Notifications, now); err != nil {
		s.logger.Error("alerting: save evaluation", "error", err)
		s.heartbeat(ctx, nil, err.Error())
		return
	}
	if len(result.Notifications) > 0 {
		s.logger.Info("alerting: state transitions", "notifications", len(result.Notifications))
	}
	if len(result.Skipped) > 0 {
		s.logger.Debug("alerting: skipped rules", "count", len(result.Skipped))
	}
}

func (s *Service) deliver(ctx context.Context) {
	if s.notifier == nil {
		return
	}
	delivered, failed, err := s.notifier.Drain(ctx)
	if err != nil {
		s.logger.Error("alerting: drain outbox", "error", err)
		return
	}
	if delivered > 0 || failed > 0 {
		s.logger.Info("alerting: outbox drained", "delivered", delivered, "failed", failed)
	}
}

func (s *Service) heartbeat(ctx context.Context, at *time.Time, errText string) {
	if err := s.store.Heartbeat(ctx, at, errText); err != nil {
		s.logger.Error("alerting: heartbeat", "error", err)
	}
}

// AlertingSnapshot implements selfobs.AlertingSource. The status is computed
// here because the service owns the evaluation interval: an evaluation older
// than three intervals is a dead-man's switch (AD-24).
func (s *Service) AlertingSnapshot() selfobs.Alerting {
	alerting := selfobs.Alerting{
		Enabled:           true,
		StaleAfterSeconds: (3 * s.interval).Seconds(),
	}
	snapshot, err := s.store.Snapshot(context.Background())
	if err != nil {
		alerting.Status = selfobs.StatusCritical
		alerting.Detail = "alerting state is unavailable: " + err.Error()
		return alerting
	}
	alerting.LastEvaluationAt = snapshot.LastEvaluationAt
	alerting.Firing = snapshot.Firing
	alerting.Pending = snapshot.Pending
	alerting.NotificationsPending = snapshot.NotificationsPending
	alerting.LastError = snapshot.LastError

	switch {
	case snapshot.LastEvaluationAt == nil:
		alerting.Status = selfobs.StatusDegraded
		alerting.Detail = "waiting for the first evaluation"
	case s.now().Sub(*snapshot.LastEvaluationAt) > 3*s.interval:
		alerting.Status = selfobs.StatusCritical
		alerting.Detail = "evaluation is stale (dead-man's switch)"
	case snapshot.LastError != "":
		alerting.Status = selfobs.StatusDegraded
		alerting.Detail = snapshot.LastError
	default:
		alerting.Status = selfobs.StatusHealthy
	}
	return alerting
}

// Interval returns the evaluation period (used by tests and the report).
func (s *Service) Interval() time.Duration {
	return s.interval
}
