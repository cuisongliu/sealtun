package auth

import (
	"errors"
	"io"
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

func TestGetInitDataUsesClientAppConfigDomain(t *testing.T) {
	originalTransport := http.DefaultTransport
	var urls []string
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		urls = append(urls, req.URL.String())
		if req.URL.String() != "https://applaunchpad.gzg.sealos.run/api/platform/getClientAppConfig" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":200,"data":{"domain":"sealosgzg.site"}}`)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	got, err := GetInitData("gzg")
	if err != nil {
		t.Fatalf("expected client app config lookup to succeed: %v", err)
	}
	if got == nil || got.Data.SealosDomain != "sealosgzg.site" {
		t.Fatalf("unexpected init data payload: %#v", got)
	}
	if len(urls) != 1 {
		t.Fatalf("expected one request, got %v", urls)
	}
}

func TestGetInitDataFallsBackToLegacyInitDataEndpoint(t *testing.T) {
	originalTransport := http.DefaultTransport
	var urls []string
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		urls = append(urls, req.URL.String())
		switch req.URL.String() {
		case "https://applaunchpad.gzg.sealos.run/api/platform/getClientAppConfig":
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("not found")),
				Request:    req,
			}, nil
		case "https://applaunchpad.gzg.sealos.run/api/platform/getInitData":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":{"SEALOS_DOMAIN":"sealosgzg.site"}}`)),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected URL: %s", req.URL.String())
			return nil, nil
		}
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	got, err := GetInitData("gzg")
	if err != nil {
		t.Fatalf("expected legacy init data fallback to succeed: %v", err)
	}
	if got == nil || got.Data.SealosDomain != "sealosgzg.site" {
		t.Fatalf("unexpected init data payload: %#v", got)
	}
	if len(urls) != 2 {
		t.Fatalf("expected two requests, got %v", urls)
	}
}

func TestGetInitDataFallsBackToKnownRegionDomainWhenBothEndpointsFail(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("<!DOCTYPE html><script src=\"/_next/static/chunks/pages/404.js\"></script>")),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	got, err := GetInitData("gzg")
	if err != nil {
		t.Fatalf("expected known region fallback to succeed: %v", err)
	}
	if got == nil || got.Data.SealosDomain != "sealosgzg.site" {
		t.Fatalf("unexpected init data fallback: %#v", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
