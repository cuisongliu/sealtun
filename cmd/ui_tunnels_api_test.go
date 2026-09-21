package cmd

import "testing"

func TestExtractTunnelIDFromOutput(t *testing.T) {
	withURL := "[+] Preparing tunnel abc123def4567890...\n[+] Public URL: https://sealtun-abc123def4567890-ns-x.sealosgzg.site\n"
	if got := extractTunnelIDFromOutput(withURL); got != "abc123def4567890" {
		t.Fatalf("url extraction = %q", got)
	}
	plain := "[+] Preparing tunnel abc123def4567890...\n"
	if got := extractTunnelIDFromOutput(plain); got != "abc123def4567890" {
		t.Fatalf("fallback extraction = %q", got)
	}
}
