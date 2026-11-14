package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// metadataPath is the single place this package names the metadata endpoint.
const metadataPath = "/metadata/client"

// Fetch downloads and parses the metadata document from baseURL. It performs
// exactly one GET (no retries) and leaves caching to a later story, so callers
// always see the server's current catalog rather than a stale snapshot.
func Fetch(ctx context.Context, client *http.Client, baseURL string) (*Catalog, error) {
	if client == nil {
		client = http.DefaultClient
	}

	endpoint := strings.TrimRight(baseURL, "/") + metadataPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("catalog: building metadata request for %s: %w", endpoint, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog: fetching metadata from %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("catalog: reading metadata from %s: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog: fetching metadata from %s: unexpected status %s", endpoint, resp.Status)
	}

	c, err := Parse(body)
	if err != nil {
		return nil, fmt.Errorf("catalog: metadata from %s: %w", endpoint, err)
	}
	return c, nil
}
