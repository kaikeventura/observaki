package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRecordsNotification(t *testing.T) {
	var entries []record
	handler := newHandler(func(entry record) error {
		entries = append(entries, entry)
		return nil
	})

	request := httptest.NewRequest(http.MethodPost, "/alert", strings.NewReader(`{"state":"firing"}`))
	request.Header.Set("Idempotency-Key", "rule|1|1|channel")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].IdempotencyKey != "rule|1|1|channel" || entries[0].Path != "/alert" {
		t.Fatalf("entry = %+v", entries[0])
	}
	var body map[string]string
	if err := json.Unmarshal(entries[0].Body, &body); err != nil || body["state"] != "firing" {
		t.Fatalf("body = %s, err = %v", entries[0].Body, err)
	}
}
