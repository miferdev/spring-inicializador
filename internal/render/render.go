// Package render turns one options.Options value into the archive bytes
// returned by POST /starter.zip. It is pure with respect to the filesystem: it
// never writes and never extracts, and it does not import tui.
package render

import (
	"context"
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
// returned error carries the server's body verbatim, so the caller can print
// the server's own message without a second request or a client-side guess.
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
		return nil, fmt.Errorf("render: generating project: server returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	return data, nil
}
