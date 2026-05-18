package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/k8s"
)

func TestPrintEventsShowsRecentEvents(t *testing.T) {
	payload := &eventsPayload{
		TunnelID: "web",
		Events: []k8s.EventDiagnostic{{
			Type:          "Warning",
			Reason:        "FailedScheduling",
			Message:       "0/3 nodes are available",
			Object:        "Pod/sealtun-web-pod",
			Count:         2,
			LastTimestamp: "2026-05-18T10:00:00Z",
		}},
	}

	var output bytes.Buffer
	eventsCmd.SetOut(&output)
	t.Cleanup(func() { eventsCmd.SetOut(nil) })

	printEvents(eventsCmd, payload)
	text := output.String()
	for _, want := range []string{
		"Sealtun Events",
		"Tunnel ID: web",
		"2026-05-18T10:00:00Z [Warning/FailedScheduling x2] Pod/sealtun-web-pod: 0/3 nodes are available",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected events output to contain %q, got:\n%s", want, text)
		}
	}
}

func TestPrintEventsShowsEmptyState(t *testing.T) {
	payload := &eventsPayload{TunnelID: "web"}

	var output bytes.Buffer
	eventsCmd.SetOut(&output)
	t.Cleanup(func() { eventsCmd.SetOut(nil) })

	printEvents(eventsCmd, payload)
	if text := output.String(); !strings.Contains(text, "No recent remote events found.") {
		t.Fatalf("expected empty state, got:\n%s", text)
	}
}
