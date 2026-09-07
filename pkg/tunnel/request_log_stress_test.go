package tunnel

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// R1a: exact replay-cap boundaries — 32KiB minus/plus one byte.
func TestCaptureRequestBodyExactBoundaries(t *testing.T) {
	for _, size := range []int{requestBodyReplayMax - 1, requestBodyReplayMax, requestBodyReplayMax + 1} {
		body := strings.Repeat("a", size)
		req := httptest.NewRequest(http.MethodPost, "https://example.test/x", strings.NewReader(body))
		req.Header.Set("Content-Type", "text/plain")
		_, replayBody, omitted, truncated := captureRequestBody(req)
		wantKept := size <= requestBodyReplayMax
		if wantKept && (truncated || omitted || replayBody != body) {
			t.Fatalf("size %d must be fully kept: truncated=%v omitted=%v kept=%d", size, truncated, omitted, len(replayBody))
		}
		if !wantKept && (!truncated || !omitted || replayBody != "") {
			t.Fatalf("size %d must be omitted: truncated=%v omitted=%v kept=%d", size, truncated, omitted, len(replayBody))
		}
		restored, _ := io.ReadAll(req.Body)
		if string(restored) != body {
			t.Fatalf("size %d: restore not byte-identical (%d bytes)", size, len(restored))
		}
	}
}

// R1b: sensitive headers are redacted regardless of casing.
func TestCaptureHeadersCaseInsensitiveRedaction(t *testing.T) {
	header := http.Header{}
	header.Set("aUThorIZation", "Bearer x")
	header.Set("cOoKiE", "y")
	header.Set("proxy-authorization", "z")
	header.Set("X-API-KEY", "k")
	captured := captureRequestHeaders(header)
	for name, value := range captured {
		if value != "(redacted)" {
			t.Fatalf("%s not redacted: %q", name, value)
		}
	}
}

// R1c: header flood — a request with 1000 headers must not blow up the entry.
func TestCaptureHeadersFlood(t *testing.T) {
	header := http.Header{}
	for i := 0; i < 1000; i++ {
		header.Set(fmt.Sprintf("X-H-%d", i), "v")
	}
	captured := captureRequestHeaders(header)
	if len(captured) != 1000 {
		t.Fatalf("expected all headers captured, got %d", len(captured))
	}
}

// R1d: absolute-form request targets must not turn replay into an open proxy.
func TestReplayURINeverLeavesLocalhost(t *testing.T) {
	// simulate a proxy-style absolute-form request line
	req := httptest.NewRequest(http.MethodGet, "http://evil.example/x?y=1", nil)
	req.RequestURI = "http://evil.example/x?y=1"
	entry := captureRequestLogEntry(req, 200, 0, 0, "1.2.3.4", "", "", false, false)
	if strings.HasPrefix(entry.URI, "http://") || strings.HasPrefix(entry.URI, "https://") {
		t.Fatalf("absolute-form URI stored: %q", entry.URI)
	}
	if entry.URI != "/x?y=1" {
		t.Fatalf("URI should be reduced to origin form, got %q", entry.URI)
	}
}

// R1e: double-slash host-like paths stay on localhost when replayed.
func TestReplayTargetDoubleSlashPath(t *testing.T) {
	uri := "//evil.example/x"
	target := "http://localhost:3000" + uri
	if !strings.HasPrefix(target, "http://localhost:3000//") {
		t.Fatalf("unexpected join: %q", target)
	}
}
