package cmd

import (
	"os"
	"testing"
	"time"

	"github.com/labring/sealtun/pkg/session"
)

func TestValidateLocalPort(t *testing.T) {
	t.Parallel()

	validPorts := []string{"1", "8080", "65535"}
	for _, port := range validPorts {
		if err := validateLocalPort(port); err != nil {
			t.Fatalf("expected port %s to be valid, got error: %v", port, err)
		}
	}

	invalidPorts := []string{"0", "65536", "-1", "abc"}
	for _, port := range invalidPorts {
		if err := validateLocalPort(port); err == nil {
			t.Fatalf("expected port %s to be invalid", port)
		}
	}
}

func TestValidateProtocol(t *testing.T) {
	t.Parallel()

	validProtocols := []string{"https", "HTTPS", "ssh", "SSH", "tcp", "TCP"}
	for _, protocol := range validProtocols {
		if err := validateProtocol(protocol); err != nil {
			t.Fatalf("expected protocol %s to be valid, got error: %v", protocol, err)
		}
	}

	invalidProtocols := []string{"http", "grpc", "grpcs", "udp", "ws", "wss", ""}
	for _, protocol := range invalidProtocols {
		if err := validateProtocol(protocol); err == nil {
			t.Fatalf("expected %s to be rejected", protocol)
		}
	}
}

func TestResolveExposeTargetDefaultsLocalPort(t *testing.T) {
	t.Parallel()

	localPort, targetURL, err := resolveExposeTarget([]string{"3000"}, "")
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if localPort != "3000" || targetURL != "http://localhost:3000" {
		t.Fatalf("unexpected target: localPort=%s target=%s", localPort, targetURL)
	}
}

func TestResolveExposeTargetAcceptsRemoteHTTPUpstream(t *testing.T) {
	t.Parallel()

	localPort, targetURL, err := resolveExposeTarget(nil, "http://10.0.0.12:8080")
	if err != nil {
		t.Fatalf("resolve remote target: %v", err)
	}
	if localPort != "8080" || targetURL != "http://10.0.0.12:8080" {
		t.Fatalf("unexpected remote target: localPort=%s target=%s", localPort, targetURL)
	}
}

func TestResolveExposeTargetRejectsMismatchedPortAndTarget(t *testing.T) {
	t.Parallel()

	if _, _, err := resolveExposeTarget([]string{"3000"}, "http://10.0.0.12:8080"); err == nil {
		t.Fatal("expected mismatched positional port and target port to fail")
	}
}

func TestValidateTargetTLSOptionsRequiresHTTPSTarget(t *testing.T) {
	t.Parallel()

	if err := validateTargetTLSOptions("", true); err == nil {
		t.Fatal("expected missing target to fail")
	}
	if err := validateTargetTLSOptions("http://10.0.0.12:8080", true); err == nil {
		t.Fatal("expected http target to reject insecure TLS option")
	}
	if err := validateTargetTLSOptions("https://10.0.0.12:8443", true); err != nil {
		t.Fatalf("expected https target to accept insecure TLS option: %v", err)
	}
}

func TestForegroundCleanupRequiresCurrentProcessOwnership(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := session.Save(session.TunnelSession{
		TunnelID:        "foreown",
		Mode:            "foreground",
		PID:             os.Getpid(),
		ConnectionState: session.ConnectionStateConnected,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("save foreground session: %v", err)
	}
	if !foregroundSessionOwnedByCurrentProcess("foreown") {
		t.Fatal("expected current foreground process to own the session")
	}

	latest, err := session.Get("foreown")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	latest.Mode = "daemon"
	latest.PID = 0
	latest.ConnectionState = session.ConnectionStatePending
	if err := session.Update(*latest); err != nil {
		t.Fatalf("update daemon session: %v", err)
	}
	if foregroundSessionOwnedByCurrentProcess("foreown") {
		t.Fatal("foreground cleanup must not own a session that has moved to daemon mode")
	}
}
