package api

import (
	"fmt"
	"net/url"
)

// Endpoint returns the scheme and host of rawURL — the part of the API base
// URL that tells the user which server was called. Path, query and userinfo
// are dropped, so nothing but the server's address reaches an error message.
func Endpoint(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Scheme + "://" + u.Host
}

// UnauthorizedError is the error for a 401. A token that is valid on one
// environment is rejected by another, so the message names the server that
// was called instead of claiming the session expired: a dev `api_url` with a
// prod token looks exactly like an expired session otherwise (WA-2694).
func UnauthorizedError(rawURL string) error {
	return fmt.Errorf(
		"auth failed against %s (401): wrong --api-url / config api_url, or the session expired (run: weside auth login)",
		Endpoint(rawURL),
	)
}

// ConnectionError wraps a transport failure with the server that was called,
// so a stale `api_url` (a dev server that is not running) is visible.
func ConnectionError(rawURL string, err error) error {
	return fmt.Errorf("cannot reach %s (wrong --api-url / config api_url?): %w", Endpoint(rawURL), err)
}
