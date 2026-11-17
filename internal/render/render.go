// Package render turns one options.Options value into the archive bytes
// returned by POST /starter.zip. It is pure with respect to the filesystem: it
// never writes and never extracts, and it does not import tui.
package render

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// starterPath is the single place this package names the generation endpoint.
const starterPath = "/starter.zip"

const formContentType = "application/x-www-form-urlencoded"

// Render requests the project archive from baseURL. On a non-200 response the
// returned error carries the server's own message verbatim, so the caller can
// print it without a second request or a client-side guess.
//
// baseURL must already be normalized by catalog.NormalizeBaseURL, so the
// metadata fetch and this call can never point at different servers.
func Render(ctx context.Context, client *http.Client, baseURL string, opts options.Options) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}

	endpoint := baseURL + starterPath
	body := opts.Form().Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("render: building request for %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", formContentType)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("render: generating project: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("render: reading project archive: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newServerError(resp.StatusCode, resp.Status, data)
	}

	return data, nil
}

// ServerError is the server rejecting the request. It carries the status and the
// payload so a caller can tell a refusal from a failure of the service itself, but
// its message is the server's own text: the
// Initializr error body already reads as a sentence for a human —
// "Invalid Spring Boot version '3.5.6', Spring Boot compatibility range is >=4.0.0"
// — and reimplementing those rules on the client would drift from the service.
type ServerError struct {
	Status  string
	Code    int
	Message string
	Body    string
}

func (e *ServerError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("generating project: server returned %s: %s", e.Status, strings.TrimSpace(e.Body))
}

// errorPayload is the shape Spring Initializr uses for rejections:
// {timestamp, status, error, message, path}.
type errorPayload struct {
	Message string `json:"message"`
}

func newServerError(code int, status string, body []byte) *ServerError {
	serverErr := &ServerError{Status: status, Code: code, Body: string(body)}

	var payload errorPayload
	if err := json.Unmarshal(body, &payload); err == nil {
		serverErr.Message = strings.TrimSpace(payload.Message)
	}
	return serverErr
}
