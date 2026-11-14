package catalog

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeBaseURL turns the value of --server into the single base every
// request is built from: an absolute http or https URL with no trailing slash.
//
// The two conveniences it allows are the ones people actually type: a bare host
// ("start.spring.io", "localhost:8080") gets https, and a trailing slash is
// dropped. Everything else is refused by name, because a typo in --server
// should fail before a request is built, not produce a confusing 404 later.
func NormalizeBaseURL(raw string) (string, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return "", fmt.Errorf("server URL is empty")
	}

	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("server URL %q is not a valid URL: %w", raw, err)
	}
	if parsed.Fragment != "" || parsed.RawQuery != "" {
		return "", fmt.Errorf("server URL %q must not carry a query or fragment", raw)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("server URL %q must not carry credentials", raw)
	}

	switch parsed.Scheme {
	case "http", "https":
	case "":
		return "", fmt.Errorf("server URL %q has no scheme; expected http or https", raw)
	default:
		return "", fmt.Errorf("server URL %q uses scheme %q; expected http or https", raw, parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("server URL %q has no host", raw)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}
