package tunnel

import "testing"

func TestParseTargetNormalizesHTTPUpstream(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("http://10.0.0.12:8080")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if target.URL != "http://10.0.0.12:8080" {
		t.Fatalf("unexpected url: %s", target.URL)
	}
	if target.Address != "10.0.0.12:8080" || target.Port != "8080" {
		t.Fatalf("unexpected target: %#v", target)
	}
	if target.HostHeader != "10.0.0.12:8080" {
		t.Fatalf("unexpected host header: %s", target.HostHeader)
	}
}

func TestParseTargetAppliesDefaultPorts(t *testing.T) {
	t.Parallel()

	httpTarget, err := ParseTarget("http://api.internal")
	if err != nil {
		t.Fatalf("parse http target: %v", err)
	}
	if httpTarget.URL != "http://api.internal:80" {
		t.Fatalf("unexpected http url: %s", httpTarget.URL)
	}
	if httpTarget.HostHeader != "api.internal" {
		t.Fatalf("unexpected http host header: %s", httpTarget.HostHeader)
	}

	httpsTarget, err := ParseTarget("https://api.internal")
	if err != nil {
		t.Fatalf("parse https target: %v", err)
	}
	if httpsTarget.URL != "https://api.internal:443" {
		t.Fatalf("unexpected https url: %s", httpsTarget.URL)
	}
	if httpsTarget.HostHeader != "api.internal" {
		t.Fatalf("unexpected https host header: %s", httpsTarget.HostHeader)
	}

	httpsExplicitDefaultPortTarget, err := ParseTarget("https://api.internal:443")
	if err != nil {
		t.Fatalf("parse https target with explicit default port: %v", err)
	}
	if httpsExplicitDefaultPortTarget.HostHeader != "api.internal" {
		t.Fatalf("unexpected https explicit default-port host header: %s", httpsExplicitDefaultPortTarget.HostHeader)
	}
}

func TestParseTargetPreservesSubRoute(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("https://api.internal/base/path")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if target.URL != "https://api.internal:443/base/path" {
		t.Fatalf("unexpected url: %s", target.URL)
	}
}

func TestParseTargetRejectsUnsupportedURLParts(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"ftp://api.internal:21",
		"http://api.internal:8080?token=secret",
		"http://user:pass@api.internal:8080",
	} {
		if _, err := ParseTarget(raw); err == nil {
			t.Fatalf("expected target %q to be rejected", raw)
		}
	}
}
