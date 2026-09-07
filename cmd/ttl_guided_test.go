package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestParseGuidedTTL(t *testing.T) {
	cases := map[string]time.Duration{
		"2h":    2 * time.Hour,
		"30m":   30 * time.Minute,
		" 1h ":  time.Hour,
		"no":    0,
		"NO":    0,
		"never": 0,
		"0":     0,
	}
	for input, want := range cases {
		got, err := parseGuidedTTL(input)
		if err != nil || got != want {
			t.Fatalf("parseGuidedTTL(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
	for _, bad := range []string{"abc", "-2h", "forever"} {
		if _, err := parseGuidedTTL(bad); err == nil {
			t.Fatalf("parseGuidedTTL(%q) should fail", bad)
		}
	}
}

func TestExposeTTLHelpers(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if got := exposeTTLLabel(0); got != "" {
		t.Fatalf("zero ttl label = %q", got)
	}
	if got := exposeTTLLabel(2 * time.Hour); got != "2h0m0s" {
		t.Fatalf("ttl label = %q", got)
	}
	if got := exposeExpiresAt(0, now); got != "" {
		t.Fatalf("zero ttl expiresAt = %q", got)
	}
	got := exposeExpiresAt(2*time.Hour, now)
	if got != "2026-09-07T14:00:00Z" {
		t.Fatalf("expiresAt = %q", got)
	}
}

func TestUpPlanApplyFileCarriesTTL(t *testing.T) {
	resetUpTestGlobals(t)
	plan := &upPlan{
		Source:    "guided",
		Protocol:  "https",
		LocalPort: "3000",
		TTL:       "2h0m0s",
	}
	config := upPlanApplyFile(plan)
	if len(config.Tunnels) != 1 {
		t.Fatalf("expected one tunnel, got %d", len(config.Tunnels))
	}
	if config.Tunnels[0].TTL != "2h0m0s" {
		t.Fatalf("saved YAML lost the TTL: %#v", config.Tunnels[0])
	}
	if !strings.Contains(string(mustMarshalApplyYAML(t, config)), "ttl: 2h0m0s") {
		t.Fatal("marshaled YAML must contain the ttl field")
	}
}

func mustMarshalApplyYAML(t *testing.T, config *applyFile) []byte {
	t.Helper()
	data, err := marshalApplyFileYAML(config)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
