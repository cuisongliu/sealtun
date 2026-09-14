package cmd

import (
	"strings"
	"testing"

	tunnelprotocol "github.com/labring/sealtun/pkg/protocol"
)

func TestResolveAlias(t *testing.T) {
	cases := map[string][2]string{
		"ssh":   {tunnelprotocol.TCP, tunnelprotocol.SSH},
		"SSH":   {tunnelprotocol.TCP, tunnelprotocol.SSH},
		"http":  {tunnelprotocol.HTTPS, "http"},
		"https": {tunnelprotocol.HTTPS, ""},
		"tcp":   {tunnelprotocol.TCP, ""},
	}
	for input, want := range cases {
		canonical, alias := tunnelprotocol.ResolveAlias(input)
		if canonical != want[0] || alias != want[1] {
			t.Fatalf("ResolveAlias(%q) = %q, %q; want %q, %q", input, canonical, alias, want[0], want[1])
		}
	}
}

func TestValidateExposeAcceptsAliases(t *testing.T) {
	for _, p := range []string{"https", "tcp", "ssh", "http"} {
		if err := validateProtocol(p); err != nil {
			t.Fatalf("%s should be accepted, got %v", p, err)
		}
	}
	if err := validateProtocol("udp"); err == nil || !strings.Contains(err.Error(), "https and tcp") {
		t.Fatalf("udp should be rejected with the new protocol set message, got %v", err)
	}
}

func TestSSHTemplateEmitsTCP(t *testing.T) {
	spec, ok := protocolTemplateSpec("ssh")
	if !ok {
		t.Fatal("ssh template missing")
	}
	if spec.protocol != tunnelprotocol.TCP {
		t.Fatalf("ssh template must emit canonical tcp, got %q", spec.protocol)
	}
}

func TestDiscoveryCommandUsesCanonicalProtocol(t *testing.T) {
	item := applyPortHints(discoverItem{Port: 22})
	if !strings.Contains(item.Command, "--protocol tcp") {
		t.Fatalf("discovery command for port 22 must use tcp, got %q", item.Command)
	}
	if item.TemplateHint != "ssh" {
		t.Fatalf("template hint should stay ssh, got %q", item.TemplateHint)
	}
}

func TestApplyProtocolAliasWarnings(t *testing.T) {
	if got := applyProtocolAliasWarnings(applyTunnel{Protocol: "ssh"}); len(got) != 1 || !strings.Contains(got[0], "normalized to tcp") {
		t.Fatalf("ssh YAML must warn, got %#v", got)
	}
	if got := applyProtocolAliasWarnings(applyTunnel{Protocol: "http"}); len(got) != 1 || !strings.Contains(got[0], "https") {
		t.Fatalf("http YAML must warn, got %#v", got)
	}
	if got := applyProtocolAliasWarnings(applyTunnel{Protocol: "tcp"}); len(got) != 0 {
		t.Fatalf("tcp must not warn, got %#v", got)
	}
}

func TestNormalizeApplyTunnelNormalizesAliases(t *testing.T) {
	normalized, err := normalizeApplyTunnel(applyTunnel{Name: "s", LocalPort: 22, Protocol: "ssh"})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Protocol != tunnelprotocol.TCP {
		t.Fatalf("apply must normalize ssh to tcp, got %q", normalized.Protocol)
	}
	normalized, err = normalizeApplyTunnel(applyTunnel{Name: "w", LocalPort: 3000, Protocol: "http"})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Protocol != tunnelprotocol.HTTPS {
		t.Fatalf("apply must normalize http to https, got %q", normalized.Protocol)
	}
}
