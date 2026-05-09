package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/auth"
	"github.com/labring/sealtun/pkg/k8s"
	"github.com/labring/sealtun/pkg/session"
)

func TestRunApplyDryRunDoesNotRequireLogin(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sealtun.yaml")
	data := []byte(`version: v1
tunnels:
  - name: web
    localPort: 3000
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	results, err := runApply(context.Background(), path, true)
	if err != nil {
		t.Fatalf("dry-run apply should not require login: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].TunnelID != "web" || results[0].LocalPort != "3000" || results[0].Status != "planned" {
		t.Fatalf("unexpected dry-run result: %+v", results[0])
	}
}

func TestBuildApplySessionRecordPersistsCustomDomain(t *testing.T) {
	record := buildApplySessionRecord(normalizedApplyTunnel{
		TunnelID:  "web",
		LocalPort: "3000",
		Protocol:  "https",
	}, &auth.AuthData{Region: "https://gzg.sealos.run"}, "ns-demo", "kubeconfig", "secret", k8s.TunnelHosts{
		PublicHost:   "app.example.com",
		SealosHost:   "sealtun-web-ns-demo.sealosgzg.site",
		CustomDomain: "app.example.com",
	}, "2026-05-09T00:00:00Z")

	if record.CustomDomain != "app.example.com" {
		t.Fatalf("expected custom domain to be persisted, got %q", record.CustomDomain)
	}
	if record.Host != "app.example.com" || record.SealosHost != "sealtun-web-ns-demo.sealosgzg.site" {
		t.Fatalf("unexpected hosts in session record: %#v", record)
	}
}

func TestNormalizeApplyTunnelRejectsUnsafeNames(t *testing.T) {
	t.Parallel()

	invalid := []string{"Web", "-web", "web_", "../web", ""}
	for _, name := range invalid {
		if _, err := normalizeApplyTunnel(applyTunnel{Name: name, LocalPort: 3000}); err == nil {
			t.Fatalf("expected invalid apply tunnel name %q to fail", name)
		}
	}
}

func TestNormalizeApplyTunnelDefaultsProtocol(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeApplyTunnel(applyTunnel{Name: "api", Port: 8080})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Protocol != "https" {
		t.Fatalf("expected default https protocol, got %q", normalized.Protocol)
	}
	if normalized.LocalPort != "8080" {
		t.Fatalf("expected port alias to be used, got %q", normalized.LocalPort)
	}
}

func TestRunApplyRejectsDuplicateTunnelNames(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sealtun.yaml")
	data := []byte(`version: v1
tunnels:
  - name: web
    localPort: 3000
  - name: web
    localPort: 3001
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := runApply(context.Background(), path, true); err == nil {
		t.Fatal("expected duplicate tunnel names to be rejected")
	}
}

func TestLoadApplyFileRejectsOversizedFiles(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sealtun.yaml")
	data := make([]byte, applyFileMaxBytes+1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadApplyFile(path); err == nil {
		t.Fatal("expected oversized apply file to be rejected")
	}
}

func TestLoadApplyFileRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sealtun.yaml")
	data := []byte(`version: v1
tunnels:
  - name: web
    localPort: 3000
    waitDomian: true
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadApplyFile(path); err == nil || !strings.Contains(err.Error(), "field waitDomian not found") {
		t.Fatalf("expected unknown field to be rejected, got %v", err)
	}
}

func TestLoadApplyFileRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sealtun.yaml")
	data := []byte(`version: v1
tunnels:
  - name: web
    localPort: 3000
---
version: v1
tunnels:
  - name: api
    localPort: 3001
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := loadApplyFile(path); err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("expected multiple documents to be rejected, got %v", err)
	}
}

func TestApplyOneTunnelRejectsCorruptExistingSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := auth.GetSealosDir()
	if err != nil {
		t.Fatal(err)
	}
	sessionsDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "web.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = applyOneTunnel(context.Background(), applyTunnel{Name: "web", LocalPort: 3000}, &auth.AuthData{}, nil, "", false)
	if err == nil {
		t.Fatal("expected corrupt existing session to be rejected before provisioning")
	}
}

func TestApplyOneTunnelRejectsExistingSessionFromDifferentRegion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:  "web",
		Region:    "https://old.sealos.run",
		Namespace: "default",
		LocalPort: "3000",
		Protocol:  "https",
		Secret:    "secret",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := applyOneTunnel(context.Background(), applyTunnel{Name: "web", LocalPort: 3000}, &auth.AuthData{Region: "https://gzg.sealos.run"}, nil, "", false)
	if err == nil || !strings.Contains(err.Error(), "already belongs to region") {
		t.Fatalf("expected region mismatch to be rejected before provisioning, got %v", err)
	}
}

func TestApplyOneTunnelRejectsExistingSessionWithoutSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := session.Save(session.TunnelSession{
		TunnelID:        "web",
		Region:          "https://gzg.sealos.run",
		Namespace:       "default",
		LocalPort:       "3000",
		Protocol:        "https",
		ConnectionState: session.ConnectionStateStopped,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := applyOneTunnel(context.Background(), applyTunnel{Name: "web", LocalPort: 3000}, &auth.AuthData{Region: "https://gzg.sealos.run"}, nil, "", false)
	if err == nil || !strings.Contains(err.Error(), "local secret is unavailable") {
		t.Fatalf("expected missing secret to be rejected before provisioning, got %v", err)
	}
}

func TestApplyOneTunnelRequiresVerifiedCustomDomainBeforeUpdatingExistingTunnel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	previous := session.TunnelSession{
		TunnelID:     "web",
		Region:       "https://gzg.sealos.run",
		Namespace:    "default",
		Protocol:     "https",
		Host:         "old.example.com",
		SealosHost:   "sealtun-web-default.sealosgzg.site",
		CustomDomain: "old.example.com",
		LocalPort:    "3000",
		Secret:       "secret",
		Mode:         "daemon",
	}
	if err := session.Save(previous); err != nil {
		t.Fatal(err)
	}
	originalLookup := lookupCNAME
	lookupCNAME = func(context.Context, string) (string, error) {
		return "wrong.example.com.", nil
	}
	defer func() {
		lookupCNAME = originalLookup
	}()

	_, err := applyOneTunnel(context.Background(), applyTunnel{Name: "web", LocalPort: 3001, Domain: "new.example.com"}, &auth.AuthData{Region: "https://gzg.sealos.run"}, nil, "", false)
	if err == nil || !strings.Contains(err.Error(), "custom domain DNS must be verified before updating an existing tunnel") {
		t.Fatalf("expected DNS verification error before update, got %v", err)
	}
	current, err := session.Get("web")
	if err != nil {
		t.Fatal(err)
	}
	if current.LocalPort != previous.LocalPort || current.CustomDomain != previous.CustomDomain || current.Host != previous.Host {
		t.Fatalf("existing session was modified despite DNS failure: %#v", current)
	}
}

func TestRollbackApplyResultsRestoresExistingLocalSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	previous := session.TunnelSession{
		TunnelID:     "web",
		Region:       "https://gzg.sealos.run",
		Namespace:    "default",
		Protocol:     "https",
		Host:         "old.example.com",
		SealosHost:   "sealtun-web-default.sealosgzg.site",
		CustomDomain: "old.example.com",
		LocalPort:    "3000",
		Secret:       "secret",
		Mode:         "daemon",
	}
	if err := session.Save(previous); err != nil {
		t.Fatal(err)
	}
	if err := session.Save(session.TunnelSession{
		TunnelID:  "web",
		Region:    "https://gzg.sealos.run",
		Namespace: "default",
		Protocol:  "https",
		Host:      "new.example.com",
		LocalPort: "3001",
		Secret:    "secret",
		Mode:      "daemon",
	}); err != nil {
		t.Fatal(err)
	}

	rollbackApplyResults(nil, []applyResult{{
		TunnelID: "web",
		Previous: &session.TunnelSession{
			TunnelID:     previous.TunnelID,
			Region:       previous.Region,
			Namespace:    previous.Namespace,
			Protocol:     previous.Protocol,
			Host:         previous.Host,
			SealosHost:   previous.SealosHost,
			CustomDomain: previous.CustomDomain,
			LocalPort:    previous.LocalPort,
			Secret:       previous.Secret,
			Mode:         previous.Mode,
		},
	}})

	current, err := session.Get("web")
	if err != nil {
		t.Fatal(err)
	}
	for field, got := range map[string]string{
		"host":         current.Host,
		"sealosHost":   current.SealosHost,
		"customDomain": current.CustomDomain,
		"localPort":    current.LocalPort,
	} {
		want := map[string]string{
			"host":         previous.Host,
			"sealosHost":   previous.SealosHost,
			"customDomain": previous.CustomDomain,
			"localPort":    previous.LocalPort,
		}[field]
		if got != want {
			t.Fatalf("expected restored %s %q, got %q", field, want, got)
		}
	}
}
