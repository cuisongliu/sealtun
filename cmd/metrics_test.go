package cmd

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/k8s"
	"github.com/labring/sealtun/pkg/session"
)

func TestMetricsHTTPClientDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	client := newMetricsHTTPClient()
	req, err := http.NewRequest(http.MethodGet, "https://example.com/_sealtun/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirectReq, err := http.NewRequest(http.MethodGet, "https://evil.example/_sealtun/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := client.CheckRedirect(redirectReq, []*http.Request{req}); err != http.ErrUseLastResponse {
		t.Fatalf("expected redirects to be blocked, got %v", err)
	}
}

func TestDecodeMetricsJSONRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	var payload map[string]interface{}
	body := `{"totalRequests":1}` + strings.Repeat(" ", metricsResponseMaxBytes)
	if err := decodeMetricsJSON(strings.NewReader(body), &payload); err == nil {
		t.Fatal("expected oversized metrics response to be rejected")
	}
}

func TestFetchServerMetricsRejectsInvalidSessionHost(t *testing.T) {
	t.Parallel()

	_, err := fetchServerMetrics(context.Background(), session.TunnelSession{
		SealosHost: "sealtun.example.com@127.0.0.1",
		Secret:     "secret",
	})
	if err == nil {
		t.Fatal("expected invalid session host to be rejected")
	}
	if !strings.Contains(err.Error(), "invalid session metrics host") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectMetricsPayloadRefreshesRemoteHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	originalCollector := collectSessionRemoteState
	collectSessionRemoteState = func(ctx context.Context, sess session.TunnelSession) (*k8s.TunnelRemoteState, error) {
		return &k8s.TunnelRemoteState{
			PublicHost:   "ai-gateway.code05.com",
			SealosHost:   "sealtun-web-ns-demo.bja.sealos.run",
			CustomDomain: "ai-gateway.code05.com",
			Protocol:     "https",
			DeploymentOK: true,
		}, nil
	}
	t.Cleanup(func() { collectSessionRemoteState = originalCollector })

	if err := session.Save(session.TunnelSession{
		TunnelID:  "web",
		Protocol:  "https",
		Host:      "old.example.com",
		LocalPort: "3000",
		Secret:    "secret",
		Region:    "https://bja.sealos.run",
		Namespace: "ns-demo",
	}); err != nil {
		t.Fatal(err)
	}

	oldRemote := metricsRemote
	oldServer := metricsServer
	metricsRemote = false
	metricsServer = false
	t.Cleanup(func() {
		metricsRemote = oldRemote
		metricsServer = oldServer
	})

	payload, err := collectMetricsPayloadWithContext(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Host != "ai-gateway.code05.com" || payload.SealosHost != "sealtun-web-ns-demo.bja.sealos.run" || payload.CustomDomain != "ai-gateway.code05.com" {
		t.Fatalf("expected refreshed remote hosts, got %#v", payload)
	}
}
