package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labring/sealtun/pkg/session"
)

func TestDaemonTunnelFingerprintChangesForForwardingInputs(t *testing.T) {
	base := session.TunnelSession{
		TunnelID:   "abc123",
		SealosHost: "sealtun-abc123-default.sealosgzg.site",
		LocalPort:  "3000",
		Protocol:   "https",
		Secret:     "secret",
	}
	baseFingerprint := daemonTunnelFingerprint(base)

	tests := []struct {
		name string
		mut  func(*session.TunnelSession)
	}{
		{name: "control host", mut: func(sess *session.TunnelSession) { sess.SealosHost = "other.sealosgzg.site" }},
		{name: "local port", mut: func(sess *session.TunnelSession) { sess.LocalPort = "3001" }},
		{name: "protocol", mut: func(sess *session.TunnelSession) { sess.Protocol = "http" }},
		{name: "secret", mut: func(sess *session.TunnelSession) { sess.Secret = "new-secret" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := base
			tt.mut(&next)
			if got := daemonTunnelFingerprint(next); got == baseFingerprint {
				t.Fatalf("expected fingerprint to change for %s", tt.name)
			}
		})
	}
}

func TestOpenDaemonLogFileRejectsSymlink(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(home, "outside.log")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(home, "daemon.log")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	file, err := openDaemonLogFile(linked)
	if err == nil {
		_ = file.Close()
		t.Fatal("expected symlinked daemon log to be rejected")
	}
}
