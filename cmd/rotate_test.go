package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labring/sealtun/pkg/session"
)

func TestRotateServerSecretRejectsStoppedAndExpiredBeforeRemoteMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stopped := session.TunnelSession{
		TunnelID:        "stopped",
		Host:            "stopped.example.com",
		LocalPort:       "3000",
		Secret:          "secret",
		ConnectionState: session.ConnectionStateStopped,
	}
	if err := session.Save(stopped); err != nil {
		t.Fatal(err)
	}
	if _, err := rotateTunnelServerSecret(context.Background(), "stopped"); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("expected stopped tunnel rejection, got %v", err)
	}

	expired := session.TunnelSession{
		TunnelID:  "expired",
		Protocol:  "https",
		Host:      "expired.example.com",
		LocalPort: "3000",
		Secret:    "secret",
		ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
	}
	if err := session.Save(expired); err != nil {
		t.Fatal(err)
	}
	if _, err := rotateTunnelServerSecret(context.Background(), "expired"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired tunnel rejection, got %v", err)
	}
}

func TestRotateServerSecretReturnsPayloadWhenCommittedWaitFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:  "rotatepartial",
		Protocol:  "https",
		Host:      "rotate.example.com",
		LocalPort: "3000",
		Secret:    "old-secret",
	}); err != nil {
		t.Fatal(err)
	}
	previousUpdate := updateTunnelServerSecret
	updateTunnelServerSecret = func(_ context.Context, sess *session.TunnelSession, newSecret string, _ time.Time) error {
		sess.Secret = newSecret
		if err := session.Update(*sess); err != nil {
			t.Fatal(err)
		}
		return committedServerSecretError{err: fmt.Errorf("wait failed")}
	}
	t.Cleanup(func() { updateTunnelServerSecret = previousUpdate })

	payload, err := rotateTunnelServerSecret(context.Background(), "rotatepartial")
	if err == nil || !strings.Contains(err.Error(), "wait failed") {
		t.Fatalf("expected committed wait failure, got %v", err)
	}
	if payload == nil || payload.ServerSecret == "" || payload.ServerSecret == "old-secret" {
		t.Fatalf("expected one-time server secret payload despite committed wait failure, got %#v", payload)
	}
}
