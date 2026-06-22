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

	httpsTarget, err := ParseTarget("https://api.internal")
	if err != nil {
		t.Fatalf("parse https target: %v", err)
	}
	if httpsTarget.URL != "https://api.internal:443" {
		t.Fatalf("unexpected https url: %s", httpsTarget.URL)
	}
}

func TestParseTargetRejectsNonRootURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"ftp://api.internal:21",
		"http://api.internal:8080/path",
		"http://api.internal:8080?token=secret",
		"http://user:pass@api.internal:8080",
	} {
		if _, err := ParseTarget(raw); err == nil {
			t.Fatalf("expected target %q to be rejected", raw)
		}
	}
}

func TestParseTargetWithTLSInsecureSkipVerifyRequiresHTTPS(t *testing.T) {
	t.Parallel()

	if _, err := ParseTargetWithOptions("http://10.0.0.12:8080", TargetOptions{TLSInsecureSkipVerify: true}); err == nil {
		t.Fatal("expected insecure target TLS option to reject http target")
	}
	target, err := ParseTargetWithOptions("https://api.internal:8443", TargetOptions{TLSInsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("expected https target to accept insecure target TLS option: %v", err)
	}
	if !target.TLSInsecureSkipVerify {
		t.Fatalf("expected target TLS option to be recorded: %#v", target)
	}
}
