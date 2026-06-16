package cmd

import "testing"

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
