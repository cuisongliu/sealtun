package tunnel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogRingEvictsOldest(t *testing.T) {
	log := newRequestLog()
	for i := 0; i < requestLogCapacity+10; i++ {
		log.add(RequestLogEntry{Method: "GET", Path: "/x"})
	}
	entries := log.list(0)
	if len(entries) != requestLogCapacity {
		t.Fatalf("expected %d entries, got %d", requestLogCapacity, len(entries))
	}
	// newest first with monotonically increasing seq
	if entries[0].Seq != int64(requestLogCapacity+10) {
		t.Fatalf("newest entry should have the highest seq, got %d", entries[0].Seq)
	}
	if entries[0].Seq <= entries[1].Seq {
		t.Fatal("entries must be newest-first")
	}
}

func TestCaptureRequestHeadersRedaction(t *testing.T) {
	header := http.Header{
		"Authorization": {"Bearer supersecret"},
		"Cookie":        {"session=abc"},
		"X-Custom":      {"visible"},
		"X-Long":        {strings.Repeat("x", 600)},
	}
	captured := captureRequestHeaders(header)
	if captured["Authorization"] != "(redacted)" || captured["Cookie"] != "(redacted)" {
		t.Fatalf("sensitive headers must be redacted: %#v", captured)
	}
	if captured["X-Custom"] != "visible" {
		t.Fatalf("ordinary header lost: %#v", captured)
	}
	if len(captured["X-Long"]) > requestHeaderValueMax+3 {
		t.Fatalf("long header value must be capped, got %d chars", len(captured["X-Long"]))
	}
}

func TestCaptureRequestBodyPreviewRestoresBody(t *testing.T) {
	body := strings.Repeat(`{"a":1}`, 1000) // ~7KB, over the 4KB preview cap
	req := httptest.NewRequest(http.MethodPost, "https://example.test/submit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	preview, truncated := captureRequestBodyPreview(req)
	if !truncated {
		t.Fatal("oversized body must be marked truncated")
	}
	if len(preview) != requestBodyPreviewMax {
		t.Fatalf("preview should be exactly %d bytes, got %d", requestBodyPreviewMax, len(preview))
	}
	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != body {
		t.Fatalf("proxy body must be byte-identical after capture, got %d bytes want %d", len(restored), len(body))
	}
}

func TestCaptureRequestBodyPreviewSkipsBinary(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/upload", strings.NewReader("\x00\x01\x02"))
	req.Header.Set("Content-Type", "application/octet-stream")
	preview, truncated := captureRequestBodyPreview(req)
	if preview != "" || truncated {
		t.Fatalf("binary bodies must be skipped, got %q %v", preview, truncated)
	}
}

func TestRequestsEndpointRequiresSecretAndSkipsUpgrades(t *testing.T) {
	server := NewServer("secret", 8080, "https", "3000")

	// public traffic without a connected client still flows through the log
	req := httptest.NewRequest(http.MethodGet, "https://example.test/page?q=1", nil)
	server.ServeHTTP(httptest.NewRecorder(), req)

	// upgrade requests must not be logged (they are long-lived streams, not exchanges)
	upgradeReq := httptest.NewRequest(http.MethodGet, "https://example.test/ws", nil)
	upgradeReq.Header.Set("Connection", "Upgrade")
	upgradeReq.Header.Set("Upgrade", "websocket")
	server.ServeHTTP(httptest.NewRecorder(), upgradeReq)

	// unauthorized is rejected
	unauthorized := httptest.NewRequest(http.MethodGet, "/_sealtun/requests", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, unauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without secret, got %d", rec.Code)
	}

	authorized := httptest.NewRequest(http.MethodGet, "/_sealtun/requests", nil)
	authorized.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, authorized)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with secret, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/page") {
		t.Fatalf("ordinary request missing from log: %s", body)
	}
	if strings.Contains(body, "/ws") {
		t.Fatalf("upgrade request must not be logged: %s", body)
	}
	if strings.Contains(body, "q=1") {
		t.Fatalf("query values must stay redacted: %s", body)
	}
}
