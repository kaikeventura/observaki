// Package healthcheck probes a local HTTP health endpoint.
//
// The runtime image is distroless, so container orchestration cannot rely on
// curl, wget or a shell inside the container. The Observaki binary therefore
// ships its own probe (`observaki healthcheck`), which Docker Compose and Helm
// use as the container healthcheck.
package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultURL is the API liveness endpoint on the default HTTP address.
	DefaultURL = "http://127.0.0.1:8080/healthz"
	// DefaultTimeout bounds a single probe.
	DefaultTimeout = 2 * time.Second
	// maxBodyBytes caps the response body a probe reads before closing it.
	maxBodyBytes = 4 << 10
)

// DefaultClient is the HTTP client used by probes. It has no keep-alive pool
// because a probe process makes a single request and exits.
var DefaultClient = &http.Client{Timeout: DefaultTimeout}

// Probe performs a GET against url and returns nil only for HTTP 200.
func Probe(ctx context.Context, client *http.Client, url string) error {
	if url == "" {
		return errors.New("healthcheck: url is required")
	}
	if client == nil {
		client = DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("healthcheck: build request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("healthcheck: GET %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxBodyBytes))

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: GET %s: HTTP %d", url, response.StatusCode)
	}
	return nil
}
