package tunnel

import (
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	requestLogCapacity    = 200
	requestBodyPreviewMax = 4 << 10  // 4 KiB display preview per request
	requestBodyReplayMax  = 32 << 10 // 32 KiB full body kept for replay
	requestHeaderValueMax = 512
)

// RequestLogEntry is one captured public HTTP exchange. Headers and bodies
// are redacted and size-capped at capture time; the log lives only in the
// tunnel pod's memory and is served exclusively over the secret-protected
// /_sealtun/requests endpoint.
type RequestLogEntry struct {
	Seq           int64             `json:"seq"`
	Time          string            `json:"time"`
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Status        int               `json:"status"`
	DurationMs    int64             `json:"durationMs"`
	ClientIP      string            `json:"clientIp,omitempty"`
	BytesOut      int64             `json:"bytesOut,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	BodyPreview   string            `json:"bodyPreview,omitempty"`
	BodyTruncated bool              `json:"bodyTruncated,omitempty"`
	// URI is the raw request URI (path + query) for replay; it is only served
	// over the secret-protected endpoint, never printed in the table view.
	URI string `json:"uri,omitempty"`
	// Body holds the full request body for replay when it is textual and at
	// most requestBodyReplayMax bytes; BodyOmitted reports a skipped body.
	Body        string `json:"body,omitempty"`
	BodyOmitted bool   `json:"bodyOmitted,omitempty"`
}

type requestLog struct {
	mu      sync.Mutex
	entries []RequestLogEntry
	nextSeq int64
}

func newRequestLog() *requestLog {
	return &requestLog{entries: make([]RequestLogEntry, 0, requestLogCapacity)}
}

func (l *requestLog) add(entry RequestLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextSeq++
	entry.Seq = l.nextSeq
	if len(l.entries) < requestLogCapacity {
		l.entries = append(l.entries, entry)
		return
	}
	copy(l.entries, l.entries[1:])
	l.entries[len(l.entries)-1] = entry
}

// list returns up to limit newest entries, newest first.
func (l *requestLog) list(limit int) []RequestLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}
	out := make([]RequestLogEntry, 0, limit)
	for i := len(l.entries) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, l.entries[i])
	}
	return out
}

// sensitiveRequestHeaders are never stored verbatim in the log.
var sensitiveRequestHeaders = map[string]bool{
	"Authorization":       true,
	"Cookie":              true,
	"Set-Cookie":          true,
	"Proxy-Authorization": true,
	"X-Api-Key":           true,
}

func captureRequestHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	captured := make(map[string]string, len(header))
	for name, values := range header {
		canonical := http.CanonicalHeaderKey(name)
		if sensitiveRequestHeaders[canonical] {
			captured[canonical] = "(redacted)"
			continue
		}
		value := strings.Join(values, ", ")
		if len(value) > requestHeaderValueMax {
			value = value[:requestHeaderValueMax] + "…"
		}
		captured[canonical] = value
	}
	return captured
}

// captureRequestBody reads a text-ish request body for display preview (4
// KiB) and replay (32 KiB), then restores r.Body so the proxy forwards it
// untouched. Binary, streaming, and oversized bodies are marked omitted
// rather than buffered, keeping memory bounded by the ring capacity.
func captureRequestBody(r *http.Request) (preview, replayBody string, omitted, truncated bool) {
	if r.Body == nil || r.Body == http.NoBody {
		return "", "", false, false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !isTextualMediaType(mediaType) {
		return "", "", true, false
	}
	limited := io.LimitReader(r.Body, requestBodyReplayMax+1)
	buf, _ := io.ReadAll(limited)
	truncated = len(buf) > requestBodyReplayMax
	// text holds every consumed byte (cap plus the one-byte probe), so the
	// restore below is byte-identical by construction.
	text := string(buf)
	preview = text
	if len(preview) > requestBodyPreviewMax {
		preview = preview[:requestBodyPreviewMax]
	}
	r.Body = io.NopCloser(io.MultiReader(strings.NewReader(text), r.Body))
	if truncated {
		return preview, "", true, true
	}
	return preview, text, false, false
}

func isTextualMediaType(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/xhtml+xml",
		"application/x-www-form-urlencoded", "application/problem+json",
		"application/graphql", "application/ld+json":
		return true
	}
	return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
}

func isUpgradeRequest(r *http.Request) bool {
	return r.Header.Get("Upgrade") != ""
}

func captureRequestLogEntry(r *http.Request, status int, bytesOut int64, duration time.Duration, clientIP string, preview, replayBody string, omitted, truncated bool) RequestLogEntry {
	uri := r.URL.RequestURI()
	if len(uri) > 2048 {
		uri = uri[:2048]
	}
	return RequestLogEntry{
		Time:          time.Now().UTC().Format(time.RFC3339),
		Method:        r.Method,
		Path:          redactedRequestPath(r),
		Status:        status,
		DurationMs:    duration.Milliseconds(),
		ClientIP:      clientIP,
		BytesOut:      bytesOut,
		Headers:       captureRequestHeaders(r.Header),
		BodyPreview:   preview,
		BodyTruncated: truncated,
		URI:           uri,
		Body:          replayBody,
		BodyOmitted:   omitted,
	}
}
