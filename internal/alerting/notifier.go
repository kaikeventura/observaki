package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultDeliveryBatch = 20
	maxDeliveryBackoff   = 5 * time.Minute
)

// Notifier drains the transactional outbox and delivers webhook/Slack
// notifications at least once (AD-32). The idempotency key lets the receiver
// dedupe retries.
type Notifier struct {
	store  Store
	client *http.Client
	logger *slog.Logger
	now    func() time.Time
}

// NewNotifier builds a notifier with a per-delivery timeout.
func NewNotifier(store Store, timeout time.Duration, logger *slog.Logger) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Notifier{
		store:  store,
		client: &http.Client{Timeout: timeout},
		logger: logger,
		now:    time.Now,
	}
}

// Drain claims and delivers a batch. It returns how many notifications were
// delivered and how many failed.
func (n *Notifier) Drain(ctx context.Context) (int, int, error) {
	now := n.now().UTC()
	deliveries, err := n.store.ClaimDeliveries(ctx, now, defaultDeliveryBatch)
	if err != nil {
		return 0, 0, fmt.Errorf("claim outbox: %w", err)
	}
	delivered, failed := 0, 0
	for _, delivery := range deliveries {
		if err := n.deliverOne(ctx, delivery, now); err != nil {
			failed++
			continue
		}
		delivered++
	}
	return delivered, failed, nil
}

type channelConfig struct {
	URL        string            `json:"url"`
	WebhookURL string            `json:"webhook_url"`
	Headers    map[string]string `json:"headers"`
}

func (n *Notifier) deliverOne(ctx context.Context, delivery Delivery, now time.Time) error {
	config, target, err := parseChannelConfig(delivery.Channel.Config)
	if err != nil {
		n.recordFailure(ctx, delivery, now, err)
		return err
	}

	body, contentType, err := notificationBody(delivery)
	if err != nil {
		n.recordFailure(ctx, delivery, now, err)
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		n.recordFailure(ctx, delivery, now, err)
		return err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Idempotency-Key", delivery.IdempotencyKey)
	for key, value := range config.Headers {
		request.Header.Set(key, value)
	}

	response, err := n.client.Do(request)
	if err != nil {
		n.recordFailure(ctx, delivery, now, redactError(err, target))
		return err
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := fmt.Errorf("channel %q returned HTTP %d: %s", delivery.Channel.Name, response.StatusCode, truncate(string(responseBody), 200))
		n.recordFailure(ctx, delivery, now, err)
		return err
	}
	if err := n.store.MarkDelivered(ctx, delivery.ID); err != nil {
		return fmt.Errorf("mark delivered: %w", err)
	}
	return nil
}

func (n *Notifier) recordFailure(ctx context.Context, delivery Delivery, now time.Time, cause error) {
	backoff := deliveryBackoff(delivery.Attempts)
	message := redactError(cause, "").Error()
	n.logger.Warn("alerting: delivery failed",
		"channel", delivery.Channel.Name,
		"kind", delivery.Channel.Kind,
		"attempts", delivery.Attempts,
		"retry_in", backoff.String(),
		"error", message,
	)
	if err := n.store.MarkDeliveryFailed(ctx, delivery.ID, now.Add(backoff), message); err != nil {
		n.logger.Error("alerting: record delivery failure", "error", err)
	}
}

func deliveryBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 7 {
		attempts = 7
	}
	backoff := 5 * time.Second * time.Duration(1<<(attempts-1))
	if backoff > maxDeliveryBackoff {
		backoff = maxDeliveryBackoff
	}
	return backoff
}

func parseChannelConfig(raw json.RawMessage) (channelConfig, string, error) {
	var config channelConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, "", fmt.Errorf("invalid channel config: %w", err)
	}
	target := strings.TrimSpace(config.URL)
	if target == "" {
		target = strings.TrimSpace(config.WebhookURL)
	}
	if target == "" {
		return config, "", errors.New("channel config has no url")
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return config, "", fmt.Errorf("invalid channel url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return config, "", fmt.Errorf("channel url scheme %q is not http or https", parsed.Scheme)
	}
	return config, target, nil
}

func notificationBody(delivery Delivery) ([]byte, string, error) {
	switch delivery.Channel.Kind {
	case "webhook":
		return delivery.Payload, "application/json", nil
	case "slack":
		var payload Payload
		if err := json.Unmarshal(delivery.Payload, &payload); err != nil {
			return nil, "", fmt.Errorf("decode payload: %w", err)
		}
		body, err := json.Marshal(map[string]string{"text": payload.Summary})
		if err != nil {
			return nil, "", err
		}
		return body, "application/json", nil
	default:
		return nil, "", fmt.Errorf("channel kind %q is not supported in Phase 0", delivery.Channel.Kind)
	}
}

// redactError removes a webhook URL (which may carry a secret token) from an
// error message before it is logged or persisted.
func redactError(err error, target string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if target != "" {
		message = strings.ReplaceAll(message, target, "<redacted>")
	}
	return errors.New(message)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
