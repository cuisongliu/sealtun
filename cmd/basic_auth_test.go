package cmd

import "testing"

func TestResolveBasicAuthCredential(t *testing.T) {
	got, err := resolveBasicAuth(basicAuthInput{Credential: "admin:secret"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("resolveBasicAuth returned error: %v", err)
	}
	if got == nil || !got.Enabled || got.Username != "admin" {
		t.Fatalf("unexpected config: %#v", got)
	}
	if got.PasswordHash == "secret" {
		t.Fatal("basic auth password must be hashed")
	}
}

func TestResolveBasicAuthPasswordEnv(t *testing.T) {
	got, err := resolveBasicAuth(basicAuthInput{
		Username:    "admin",
		PasswordEnv: "BASIC_PASSWORD",
	}, func(name string) string {
		if name != "BASIC_PASSWORD" {
			t.Fatalf("unexpected env lookup: %s", name)
		}
		return "secret"
	})
	if err != nil {
		t.Fatalf("resolveBasicAuth returned error: %v", err)
	}
	if got == nil || got.Username != "admin" {
		t.Fatalf("unexpected config: %#v", got)
	}
}

func TestResolveBasicAuthRejectsPartialConfig(t *testing.T) {
	if _, err := resolveBasicAuth(basicAuthInput{Username: "admin"}, func(string) string { return "" }); err == nil {
		t.Fatal("expected partial basic auth config to fail")
	}
}
