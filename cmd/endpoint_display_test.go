package cmd

import "testing"

func TestEndpointDisplaySSH(t *testing.T) {
	got := endpointDisplay("ssh", "ssh.example.com", "control.example.com", 32022)
	if got.Kind != "ssh" {
		t.Fatalf("expected ssh kind, got %q", got.Kind)
	}
	if got.Host != "ssh.example.com" || got.Port != 32022 {
		t.Fatalf("unexpected ssh endpoint: %#v", got)
	}
	if got.Command != "ssh <user>@ssh.example.com -p 32022" {
		t.Fatalf("unexpected ssh command: %q", got.Command)
	}
	if got.ControlHost != "control.example.com" {
		t.Fatalf("unexpected control host: %q", got.ControlHost)
	}
}

func TestEndpointDisplayHTTPS(t *testing.T) {
	got := endpointDisplay("https", "app.example.com", "control.example.com", 0)
	if got.Kind != "https" {
		t.Fatalf("expected https kind, got %q", got.Kind)
	}
	if got.URL != "https://app.example.com" {
		t.Fatalf("unexpected url: %q", got.URL)
	}
	if got.Command != "" || got.Port != 0 {
		t.Fatalf("https endpoint should not include ssh fields: %#v", got)
	}
}

func TestEndpointDisplayTCP(t *testing.T) {
	got := endpointDisplay("tcp", "db.example.com", "control.example.com", 35432)
	if got.Kind != "tcp" {
		t.Fatalf("expected tcp kind, got %q", got.Kind)
	}
	if got.Host != "db.example.com" || got.Port != 35432 {
		t.Fatalf("unexpected tcp endpoint: %#v", got)
	}
	if got.Command != "" {
		t.Fatalf("tcp endpoint should not include ssh command: %#v", got)
	}
	if label := endpointLabel("tcp", "db.example.com", "control.example.com", 35432); label != "db.example.com:35432" {
		t.Fatalf("unexpected tcp endpoint label: %q", label)
	}
}
