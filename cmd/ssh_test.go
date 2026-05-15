package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/session"
)

func TestSSHConnectRejectsStoppedTunnel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:        "sshdev",
		Host:            "sshdev.example.com",
		SealosHost:      "sshdev.example.com",
		LocalPort:       "22",
		Protocol:        "https",
		Secret:          "secret",
		ConnectionState: session.ConnectionStateStopped,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	cmd.SetArgs([]string{"ssh", "connect", "sshdev"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	cmd.SetArgs(nil)

	if err == nil || !strings.Contains(err.Error(), "is stopped") {
		t.Fatalf("expected stopped tunnel error, got %v", err)
	}
}

func TestSSHConnectRejectsScrubbedSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:        "sshdev",
		Host:            "sshdev.example.com",
		SealosHost:      "sshdev.example.com",
		LocalPort:       "22",
		Protocol:        "https",
		ConnectionState: session.ConnectionStateConnected,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := rootCmd
	cmd.SetArgs([]string{"ssh", "connect", "sshdev"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	cmd.SetArgs(nil)

	if err == nil || !strings.Contains(err.Error(), "local secret is unavailable") {
		t.Fatalf("expected missing secret error, got %v", err)
	}
}
