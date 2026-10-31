package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseMix(t *testing.T) {
	valid, err := parseMix("60,30,10")
	if err != nil {
		t.Fatalf("parseMix() = %v", err)
	}
	if valid.Metrics != 60 || valid.Logs != 30 || valid.Traces != 10 {
		t.Fatalf("parseMix() = %+v", valid)
	}
	if _, err := parseMix("60,30"); err == nil {
		t.Fatal("parseMix(60,30) = nil, want error")
	}
	if _, err := parseMix("60,30,20"); err == nil {
		t.Fatal("parseMix(60,30,20) = nil, want error")
	}
	if _, err := parseMix("x,30,10"); err == nil {
		t.Fatal("parseMix(x,30,10) = nil, want error")
	}
}

func TestRunCheckPayload(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--check-payload"}, &stdout, &stderr); err != nil {
		t.Fatalf("run(--check-payload) = %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "weighted gzip") {
		t.Fatalf("stdout does not contain the weighted result:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "payload check: ok") {
		t.Fatalf("stderr does not contain the check result:\n%s", stderr.String())
	}
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--dry-run", "unexpected"}, &stdout, &stderr); err == nil {
		t.Fatal("run() = nil, want error")
	}
}

func TestRunRejectsInvalidStartTime(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--check-payload", "--start-time", "not-a-time"}, &stdout, &stderr); err == nil {
		t.Fatal("run() = nil, want error")
	}
}
