package cmd

import (
	"testing"

	"github.com/labring/sealtun/pkg/session"
)

func TestExportSessionOmitsUnrecoverableSecretsByDefault(t *testing.T) {
	t.Parallel()

	basicAuth, err := newSessionBasicAuth("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	item, warnings := exportSession(session.TunnelSession{
		TunnelID:     "web",
		Protocol:     "https",
		LocalPort:    "3000",
		CustomDomain: "app.example.com",
		BasicAuth:    basicAuth,
		AccessPolicy: &session.AccessPolicy{
			IPAllowlist:       []string{"203.0.113.0/24"},
			BearerTokenHashes: []string{"sha256:abc"},
			TemporaryTokens: []session.TemporaryToken{{
				Name:      "review",
				TokenHash: "sha256:def",
				TTL:       "1h",
				ExpiresAt: "2026-01-01T00:00:00Z",
			}},
		},
	}, false)

	if item.Name != "web" || item.LocalPort != 3000 || item.Domain != "app.example.com" {
		t.Fatalf("unexpected exported tunnel: %#v", item)
	}
	if item.BasicAuth != nil {
		t.Fatalf("basic auth password hash must not be exported as config: %#v", item.BasicAuth)
	}
	if item.AccessPolicy == nil || len(item.AccessPolicy.IPAllowlist) != 1 {
		t.Fatalf("expected exportable IP policy, got %#v", item.AccessPolicy)
	}
	if item.AccessPolicy.BearerTokenEnv != "" {
		t.Fatalf("bearer token env should not be invented by default: %#v", item.AccessPolicy)
	}
	if len(item.AccessPolicy.TemporaryLinks) != 0 {
		t.Fatalf("temporary links without tokens must not be exported by default: %#v", item.AccessPolicy.TemporaryLinks)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected warnings for unrecoverable secrets, got %#v", warnings)
	}
}

func TestExportSessionCanIncludeSecretPlaceholders(t *testing.T) {
	t.Parallel()

	basicAuth, err := newSessionBasicAuth("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	item, warnings := exportSession(session.TunnelSession{
		TunnelID:  "web-dev",
		Protocol:  "https",
		LocalPort: "3000",
		BasicAuth: basicAuth,
		AccessPolicy: &session.AccessPolicy{
			BearerTokenHashes: []string{"sha256:abc"},
			TemporaryTokens: []session.TemporaryToken{{
				Name:      "review",
				TokenHash: "sha256:def",
				TTL:       "1h",
				ExpiresAt: "2026-01-01T00:00:00Z",
			}},
		},
	}, true)

	if item.BasicAuth == nil || item.BasicAuth.PasswordEnv != "SEALTUN_WEB_DEV_BASIC_AUTH_PASSWORD" {
		t.Fatalf("expected basic auth password placeholder, got %#v", item.BasicAuth)
	}
	if item.AccessPolicy == nil || item.AccessPolicy.BearerTokenEnv != "SEALTUN_WEB_DEV_BEARER_TOKEN" {
		t.Fatalf("expected bearer token placeholder, got %#v", item.AccessPolicy)
	}
	if got := item.AccessPolicy.TemporaryLinks[0].TokenEnv; got != "SEALTUN_WEB_DEV_TEMP_TOKEN_1" {
		t.Fatalf("expected temporary token placeholder, got %q", got)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings to explain placeholders cannot recover secrets")
	}
}

func TestExportSessionUsesTargetForRemoteHTTPUpstream(t *testing.T) {
	t.Parallel()

	item, warnings := exportSession(session.TunnelSession{
		TunnelID:  "api",
		Protocol:  "https",
		LocalPort: "8080",
		TargetURL: "http://10.0.0.12:8080",
	}, false)

	if item.Target != "http://10.0.0.12:8080" {
		t.Fatalf("expected target export, got %#v", item)
	}
	if item.LocalPort != 0 {
		t.Fatalf("remote target should not export localPort, got %d", item.LocalPort)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func TestExportSessionPreservesTargetTLSInsecureSkipVerify(t *testing.T) {
	t.Parallel()

	item, warnings := exportSession(session.TunnelSession{
		TunnelID:  "api",
		Protocol:  "https",
		LocalPort: "8443",
		TargetURL: "https://10.0.0.12:8443",
		TargetTLS: &session.TargetTLSConfig{InsecureSkipVerify: true},
	}, false)

	if item.Target != "https://10.0.0.12:8443" {
		t.Fatalf("expected target export, got %#v", item)
	}
	if item.TargetTLS == nil || !item.TargetTLS.InsecureSkipVerify {
		t.Fatalf("expected target TLS export, got %#v", item.TargetTLS)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func TestExportSessionRejectsInvalidLocalPort(t *testing.T) {
	t.Parallel()

	item, warnings := exportSession(session.TunnelSession{
		TunnelID:  "web",
		Protocol:  "https",
		LocalPort: "3000abc",
	}, false)

	if item.LocalPort != 0 {
		t.Fatalf("expected invalid local port to export as 0, got %d", item.LocalPort)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for invalid local port")
	}
}
