package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labring/sealtun/pkg/accesspolicy"
	"github.com/labring/sealtun/pkg/k8s"
	"github.com/labring/sealtun/pkg/session"
)

func TestGenerateShareTokenIsValidAccessPolicyToken(t *testing.T) {
	t.Parallel()

	token, err := generateShareToken()
	if err != nil {
		t.Fatalf("generateShareToken returned error: %v", err)
	}
	if len(token) < 8 {
		t.Fatalf("expected generated token to be at least 8 characters, got %q", token)
	}
	if _, err := accesspolicy.HashToken(token); err != nil {
		t.Fatalf("generated token should be hashable: %v", err)
	}
}

func TestReplaceTemporaryTokenReplacesByName(t *testing.T) {
	t.Parallel()

	tokens := []session.TemporaryToken{
		{Name: "review", TokenHash: "old", ExpiresAt: "2026-01-01T00:00:00Z"},
	}
	got := replaceTemporaryToken(tokens, session.TemporaryToken{Name: "review", TokenHash: "new", ExpiresAt: "2026-01-02T00:00:00Z"})
	if len(got) != 1 {
		t.Fatalf("expected one token, got %#v", got)
	}
	if got[0].TokenHash != "new" {
		t.Fatalf("expected token to be replaced, got %#v", got[0])
	}
}

func TestListShareLinksMarksExpired(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sess := session.TunnelSession{
		TunnelID:  "web",
		Protocol:  "https",
		Host:      "web.example.com",
		LocalPort: "3000",
		AccessPolicy: &session.AccessPolicy{TemporaryTokens: []session.TemporaryToken{
			{Name: "old", TokenHash: strings.Repeat("a", 71), ExpiresAt: "2026-01-01T00:00:00Z"},
			{Name: "new", TokenHash: strings.Repeat("b", 71), ExpiresAt: "2026-01-01T02:00:00Z"},
		}},
	}
	if err := session.Save(sess); err != nil {
		t.Fatal(err)
	}

	items, err := listShareLinks("web", time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("listShareLinks returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two links, got %#v", items)
	}
	if !items[0].Expired || items[1].Expired {
		t.Fatalf("unexpected expiration status: %#v", items)
	}
}

func TestListShareLinksRejectsNonHTTPS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:  "ssh",
		Protocol:  "ssh",
		Host:      "ssh.example.com",
		LocalPort: "22",
		AccessPolicy: &session.AccessPolicy{TemporaryTokens: []session.TemporaryToken{{
			Name:      "review",
			TokenHash: strings.Repeat("a", 71),
			ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := listShareLinks("ssh", time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "only supported for https") {
		t.Fatalf("expected non-https rejection, got %v", err)
	}
}

func TestListShareLinksRefreshesRemoteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	hash, err := accesspolicy.HashToken("remote-token")
	if err != nil {
		t.Fatal(err)
	}
	originalCollector := collectSessionRemoteState
	collectSessionRemoteState = func(ctx context.Context, sess session.TunnelSession) (*k8s.TunnelRemoteState, error) {
		return &k8s.TunnelRemoteState{
			Protocol: "https",
			AccessPolicy: &accesspolicy.Policy{
				TemporaryTokens: []accesspolicy.TemporaryToken{{
					Name:      "remote",
					TokenHash: hash,
					ExpiresAt: "2026-01-01T02:00:00Z",
				}},
			},
			DeploymentOK: true,
			AuthSecretOK: true,
		}, nil
	}
	t.Cleanup(func() { collectSessionRemoteState = originalCollector })

	if err := session.Save(session.TunnelSession{
		TunnelID:  "web",
		Protocol:  "https",
		LocalPort: "3000",
		Region:    "https://gzg.sealos.run",
		Namespace: "ns-demo",
	}); err != nil {
		t.Fatal(err)
	}

	items, err := listShareLinks("web", time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "remote" || items[0].Expired {
		t.Fatalf("expected refreshed remote share links, got %#v", items)
	}
}

func TestCreateShareLinkRejectsStoppedAndExpiredSessionsBeforeRemoteMutation(t *testing.T) {
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
	if _, err := createShareLink(context.Background(), "stopped", "review", time.Hour, "review-token"); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("expected stopped tunnel rejection, got %v", err)
	}

	expired := session.TunnelSession{
		TunnelID:  "expired",
		Protocol:  "https",
		Host:      "expired.example.com",
		LocalPort: "3000",
		Secret:    "secret",
		ExpiresAt: "2026-01-01T00:00:00Z",
	}
	if err := session.Save(expired); err != nil {
		t.Fatal(err)
	}
	if _, err := createShareLink(context.Background(), "expired", "review", time.Hour, "review-token"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired tunnel rejection, got %v", err)
	}
}

func TestSharePublicHostFallsBackToSealosHost(t *testing.T) {
	t.Parallel()

	got := sharePublicHost(session.TunnelSession{SealosHost: "sealtun-web.example.com"})
	if got != "sealtun-web.example.com" {
		t.Fatalf("expected Sealos host fallback, got %q", got)
	}
}

func TestCloneAccessPolicyPreservesRateLimitAndAudit(t *testing.T) {
	t.Parallel()

	got := cloneAccessPolicy(&session.AccessPolicy{
		RateLimit: "60/m",
		Audit:     &session.AuditConfig{Enabled: true},
	})
	if got.RateLimit != "60/m" || got.Audit == nil || !got.Audit.Enabled {
		t.Fatalf("expected rate limit and audit to be cloned, got %#v", got)
	}
	got.Audit.Enabled = false
	original := &session.AccessPolicy{Audit: &session.AuditConfig{Enabled: true}}
	cloned := cloneAccessPolicy(original)
	cloned.Audit.Enabled = false
	if !original.Audit.Enabled {
		t.Fatal("clone must not alias audit config")
	}
}

func TestRotateShareLinkRequiresExistingNameBeforeRemoteMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hash, err := accesspolicy.HashToken("old-token")
	if err != nil {
		t.Fatal(err)
	}
	sess := session.TunnelSession{
		TunnelID:  "web",
		Protocol:  "https",
		Host:      "web.example.com",
		LocalPort: "3000",
		Secret:    "secret",
		AccessPolicy: &session.AccessPolicy{
			RateLimit: "60/m",
			Audit:     &session.AuditConfig{Enabled: true},
			TemporaryTokens: []session.TemporaryToken{{
				Name:      "review",
				TokenHash: hash,
				TTL:       "1h",
				ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			}},
		},
	}
	if err := session.Save(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := rotateShareLink(context.Background(), "web", "missing", time.Hour); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing link rejection, got %v", err)
	}
}

func TestCreateShareLinkRejectsDuplicateName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hash, err := accesspolicy.HashToken("old-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Save(session.TunnelSession{
		TunnelID:  "webdup",
		Protocol:  "https",
		Host:      "web.example.com",
		LocalPort: "3000",
		Secret:    "secret",
		AccessPolicy: &session.AccessPolicy{TemporaryTokens: []session.TemporaryToken{{
			Name:      "review",
			TokenHash: hash,
			TTL:       "1h",
			ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := createShareLink(context.Background(), "webdup", "review", time.Hour, "new-token"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate share name rejection, got %v", err)
	}
}

func TestCreateShareLinkReturnsPayloadWhenCommittedWaitFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:  "webpartial",
		Protocol:  "https",
		Host:      "web.example.com",
		LocalPort: "3000",
		Secret:    "secret",
	}); err != nil {
		t.Fatal(err)
	}
	previousUpdate := updateHTTPSAccessPolicy
	updateHTTPSAccessPolicy = func(_ context.Context, sess *session.TunnelSession, policy *session.AccessPolicy) error {
		sess.AccessPolicy = policy
		if err := session.Update(*sess); err != nil {
			t.Fatal(err)
		}
		return committedAccessPolicyError{err: fmt.Errorf("wait failed")}
	}
	t.Cleanup(func() { updateHTTPSAccessPolicy = previousUpdate })

	payload, err := createShareLink(context.Background(), "webpartial", "review", time.Hour, "review-token")
	if err == nil || !strings.Contains(err.Error(), "wait failed") {
		t.Fatalf("expected committed wait failure, got %v", err)
	}
	if payload == nil || !strings.Contains(payload.URL, "_sealtun_token=review-token") {
		t.Fatalf("expected one-time URL payload despite committed wait failure, got %#v", payload)
	}
}
