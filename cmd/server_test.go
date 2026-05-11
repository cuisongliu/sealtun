package cmd

import (
	"testing"

	"github.com/labring/sealtun/pkg/publicauth"
)

func TestServerProtocolValidation(t *testing.T) {
	t.Parallel()

	valid := []string{"https", "HTTPS"}
	for _, protocol := range valid {
		if err := validateProtocol(protocol); err != nil {
			t.Fatalf("expected expose protocol %s to be valid: %v", protocol, err)
		}
	}

	invalid := []string{"http", "grpc", "grpcs", "tcp", "udp", "wss"}
	for _, protocol := range invalid {
		if err := validateProtocol(protocol); err == nil {
			t.Fatalf("expected expose protocol %s to be rejected", protocol)
		}
	}
}

func TestResolveServerSecretFromEnv(t *testing.T) {
	got, err := resolveServerSecret("flag-secret", "SEALTUN_SECRET", func(name string) string {
		if name != "SEALTUN_SECRET" {
			t.Fatalf("unexpected env lookup: %s", name)
		}
		return "env-secret"
	})
	if err != nil {
		t.Fatalf("resolveServerSecret returned error: %v", err)
	}
	if got != "env-secret" {
		t.Fatalf("expected env secret to win, got %q", got)
	}
}

func TestResolveServerSecretRejectsEmptyEnv(t *testing.T) {
	_, err := resolveServerSecret("", "SEALTUN_SECRET", func(string) string { return "" })
	if err == nil {
		t.Fatal("expected empty secret env to fail")
	}
}

func TestResolveServerBasicAuthFromEnv(t *testing.T) {
	hash, err := publicauth.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveServerBasicAuth(serverBasicAuthInput{
		UserEnv:         "BASIC_USER",
		PasswordHashEnv: "BASIC_HASH",
	}, func(name string) string {
		switch name {
		case "BASIC_USER":
			return "admin"
		case "BASIC_HASH":
			return hash
		default:
			t.Fatalf("unexpected env lookup: %s", name)
			return ""
		}
	})
	if err != nil {
		t.Fatalf("resolveServerBasicAuth returned error: %v", err)
	}
	if got == nil || got.Username != "admin" || got.PasswordHash != hash {
		t.Fatalf("unexpected basic auth config: %#v", got)
	}
}

func TestResolveServerBasicAuthRequiresCompleteConfig(t *testing.T) {
	_, err := resolveServerBasicAuth(serverBasicAuthInput{User: "admin"}, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected partial basic auth config to fail")
	}
}

func TestResolveServerBasicAuthAcceptsLegacySHA256(t *testing.T) {
	hash := publicauth.LegacySHA256Hash("secret")
	got, err := resolveServerBasicAuth(serverBasicAuthInput{
		User:           "admin",
		PasswordSHA256: hash,
	}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("resolveServerBasicAuth returned error: %v", err)
	}
	if got == nil || got.PasswordHash != hash {
		t.Fatalf("unexpected basic auth config: %#v", got)
	}
}
