package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu          sync.Mutex
	deliveries  []Delivery
	delivered   []int64
	failed      []failedDelivery
	claimErr    error
	states      []State
	notified    []Notification
	heartbeatAt *time.Time
}

type failedDelivery struct {
	id      int64
	next    time.Time
	message string
}

func (f *fakeStore) LoadRules(context.Context) ([]Rule, error) { return nil, nil }
func (f *fakeStore) LoadSilences(context.Context, time.Time) ([]Silence, error) {
	return nil, nil
}
func (f *fakeStore) LoadStates(context.Context) (map[string]State, error) { return nil, nil }
func (f *fakeStore) SaveEvaluation(_ context.Context, states []State, notifications []Notification, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states = states
	f.notified = notifications
	return nil
}
func (f *fakeStore) Heartbeat(_ context.Context, at *time.Time, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeatAt = at
	return nil
}
func (f *fakeStore) ClaimDeliveries(context.Context, time.Time, int) ([]Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	deliveries := f.deliveries
	f.deliveries = nil
	return deliveries, nil
}
func (f *fakeStore) MarkDelivered(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, id)
	return nil
}
func (f *fakeStore) MarkDeliveryFailed(_ context.Context, id int64, next time.Time, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, failedDelivery{id: id, next: next, message: message})
	return nil
}
func (f *fakeStore) Snapshot(context.Context) (StoreSnapshot, error) {
	return StoreSnapshot{}, nil
}

func TestDrainWebhook(t *testing.T) {
	var gotIdempotencyKey, gotContentType, gotBody string
	var gotCustomHeader string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdempotencyKey = r.Header.Get("Idempotency-Key")
		gotContentType = r.Header.Get("Content-Type")
		gotCustomHeader = r.Header.Get("X-Test")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	store := &fakeStore{deliveries: []Delivery{{
		ID: 7, IdempotencyKey: "rule|fingerprint|2|channel-1", Attempts: 1,
		Channel: Channel{ID: "channel-1", Name: "test", Kind: "webhook", Config: json.RawMessage(`{"url":"` + receiver.URL + `/alert","headers":{"X-Test":"1"}}`)},
		Payload: json.RawMessage(`{"version":1,"summary":"firing"}`),
	}}}
	notifier := NewNotifier(store, time.Second, nil)
	delivered, failed, err := notifier.Drain(context.Background())
	if err != nil || delivered != 1 || failed != 0 {
		t.Fatalf("Drain() = %d, %d, %v", delivered, failed, err)
	}
	if gotIdempotencyKey != "rule|fingerprint|2|channel-1" {
		t.Fatalf("Idempotency-Key = %q", gotIdempotencyKey)
	}
	if gotContentType != "application/json" || gotCustomHeader != "1" {
		t.Fatalf("headers = %q %q", gotContentType, gotCustomHeader)
	}
	if gotBody != `{"version":1,"summary":"firing"}` {
		t.Fatalf("body = %q", gotBody)
	}
	if len(store.delivered) != 1 || store.delivered[0] != 7 {
		t.Fatalf("delivered = %v", store.delivered)
	}
}

func TestDrainSlack(t *testing.T) {
	var gotBody string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	store := &fakeStore{deliveries: []Delivery{{
		ID: 8, IdempotencyKey: "key", Attempts: 1,
		Channel: Channel{ID: "channel-2", Name: "slack", Kind: "slack", Config: json.RawMessage(`{"webhook_url":"` + receiver.URL + `"}`)},
		Payload: json.RawMessage(`{"version":1,"summary":"Ingestion lag high is firing","state":"firing"}`),
	}}}
	notifier := NewNotifier(store, time.Second, nil)
	if delivered, failed, err := notifier.Drain(context.Background()); err != nil || delivered != 1 || failed != 0 {
		t.Fatalf("Drain() = %d, %d, %v", delivered, failed, err)
	}
	if gotBody != `{"text":"Ingestion lag high is firing"}` {
		t.Fatalf("slack body = %q", gotBody)
	}
}

func TestDrainFailureAndRetry(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer receiver.Close()

	store := &fakeStore{deliveries: []Delivery{{
		ID: 9, IdempotencyKey: "key", Attempts: 2,
		Channel: Channel{ID: "channel-1", Name: "test", Kind: "webhook", Config: json.RawMessage(`{"url":"` + receiver.URL + `"}`)},
		Payload: json.RawMessage(`{}`),
	}}}
	notifier := NewNotifier(store, time.Second, nil)
	delivered, failed, err := notifier.Drain(context.Background())
	if err != nil || delivered != 0 || failed != 1 {
		t.Fatalf("Drain() = %d, %d, %v", delivered, failed, err)
	}
	if len(store.failed) != 1 {
		t.Fatalf("failed = %+v", store.failed)
	}
	if !strings.Contains(store.failed[0].message, "HTTP 500") {
		t.Fatalf("message = %q", store.failed[0].message)
	}
	backoff := time.Until(store.failed[0].next)
	if backoff < 5*time.Second || backoff > 15*time.Second {
		t.Fatalf("backoff = %s, want about 10s", backoff)
	}
}

func TestDrainRejectsUnsupportedChannel(t *testing.T) {
	store := &fakeStore{deliveries: []Delivery{{
		ID: 10, IdempotencyKey: "key", Attempts: 1,
		Channel: Channel{ID: "channel-3", Name: "email", Kind: "email", Config: json.RawMessage(`{"url":"http://example.test"}`)},
		Payload: json.RawMessage(`{}`),
	}}}
	notifier := NewNotifier(store, time.Second, nil)
	if _, failed, err := notifier.Drain(context.Background()); err != nil || failed != 1 {
		t.Fatalf("Drain() failed = %d, err = %v", failed, err)
	}
	if len(store.failed) != 1 || !strings.Contains(store.failed[0].message, "not supported") {
		t.Fatalf("failed = %+v", store.failed)
	}
}

type failingTransport struct {
	url string
}

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial " + f.url + ": connection refused")
}

func TestDrainRedactsChannelURL(t *testing.T) {
	target := "http://127.0.0.1:1/hooks/secret-token"
	store := &fakeStore{deliveries: []Delivery{{
		ID: 11, IdempotencyKey: "key", Attempts: 1,
		Channel: Channel{ID: "channel-1", Name: "test", Kind: "webhook", Config: json.RawMessage(`{"url":"` + target + `"}`)},
		Payload: json.RawMessage(`{}`),
	}}}
	notifier := NewNotifier(store, time.Second, nil)
	notifier.client.Transport = failingTransport{url: target}
	if _, failed, err := notifier.Drain(context.Background()); err != nil || failed != 1 {
		t.Fatalf("Drain() failed = %d, err = %v", failed, err)
	}
	if strings.Contains(store.failed[0].message, "secret-token") {
		t.Fatalf("secret leaked in error: %q", store.failed[0].message)
	}
	if !strings.Contains(store.failed[0].message, "<redacted>") {
		t.Fatalf("error was not redacted: %q", store.failed[0].message)
	}
}
