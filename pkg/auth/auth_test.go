package auth

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPClientDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	client := httpClient()
	if client.CheckRedirect == nil {
		t.Fatal("expected auth HTTP client to disable redirects")
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.test/redirect", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(req, []*http.Request{req}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("expected ErrUseLastResponse, got %v", err)
	}
}

func TestDecodeLimitedJSONRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	var payload TokenResponse
	body := `{"access_token":"token"}` + strings.Repeat(" ", maxAuthResponseBodyBytes)
	if err := decodeLimitedJSON(strings.NewReader(body), &payload); err == nil {
		t.Fatal("expected oversized JSON response to be rejected")
	}
}

func TestReadBoundedResponseBodyReturnsTruncatedBodyAndError(t *testing.T) {
	t.Parallel()

	body, err := readBoundedResponseBody(strings.NewReader(strings.Repeat("x", maxAuthResponseBodyBytes+1)))
	if err == nil {
		t.Fatal("expected oversized response error")
	}
	if len(body) != maxAuthResponseBodyBytes {
		t.Fatalf("expected truncated body length %d, got %d", maxAuthResponseBodyBytes, len(body))
	}
}
