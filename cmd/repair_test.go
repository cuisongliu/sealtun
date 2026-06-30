package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labring/sealtun/pkg/session"
)

func TestRepairDryRunPlansButDoesNotExecute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := session.Save(session.TunnelSession{
		TunnelID:        "repairdry",
		Secret:          "secret",
		Mode:            "daemon",
		ConnectionState: session.ConnectionStateStopped,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	previousStart := doctorFixStartTunnel
	started := 0
	doctorFixStartTunnel = func(context.Context, *session.TunnelSession) error {
		started++
		return nil
	}
	t.Cleanup(func() { doctorFixStartTunnel = previousStart })

	payload, err := runRepair(context.Background(), "repairdry", true)
	if err != nil {
		t.Fatal(err)
	}
	if !payload.DryRun || payload.Results != nil {
		t.Fatalf("unexpected dry-run payload: %#v", payload)
	}
	if payload.Plan == nil || len(payload.Plan.Actions) != 1 || payload.Plan.Actions[0].Action != "start" {
		t.Fatalf("expected start plan, got %#v", payload.Plan)
	}
	if started != 0 {
		t.Fatalf("dry run executed start %d time(s)", started)
	}
}

func TestRepairExecutesAllowedAction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := session.Save(session.TunnelSession{
		TunnelID:        "repairstart",
		Secret:          "secret",
		Mode:            "daemon",
		ConnectionState: session.ConnectionStateStopped,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	previousStart := doctorFixStartTunnel
	doctorFixStartTunnel = func(context.Context, *session.TunnelSession) error {
		return nil
	}
	t.Cleanup(func() { doctorFixStartTunnel = previousStart })

	payload, err := runRepair(context.Background(), "repairstart", false)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Results == nil || len(payload.Results.Actions) != 1 || !payload.Results.Actions[0].Executed {
		t.Fatalf("expected executed repair result, got %#v", payload)
	}
}

func TestRepairRequiresKnownTunnel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := runRepair(context.Background(), "missing", true)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing tunnel error, got %v", err)
	}
}
