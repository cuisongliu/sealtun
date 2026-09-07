package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labring/sealtun/pkg/auth"
)

func TestIsClusterAuthFailure(t *testing.T) {
	definitive := []string{
		`Get "https://x:6443/api": tls: failed to verify certificate: x509: certificate signed by unknown authority`,
		`the server has asked for the client to provide credentials: Unauthorized`,
		`401 Unauthorized`,
		`token expired`,
	}
	for _, msg := range definitive {
		if !isClusterAuthFailure(errors.New(msg)) {
			t.Fatalf("expected auth failure match for %q", msg)
		}
	}
	transient := []string{
		`dial tcp 1.2.3.4:6443: i/o timeout`,
		`no such host`,
		`connection refused`,
		`context deadline exceeded`,
	}
	for _, msg := range transient {
		if isClusterAuthFailure(errors.New(msg)) {
			t.Fatalf("transient error must not trigger refresh: %q", msg)
		}
	}
}

// refreshRegionStub plays the region's token and regionToken endpoints.
func refreshRegionStub(t *testing.T, refreshStatus int, refreshBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/oauth2/token":
			w.WriteHeader(refreshStatus)
			_, _ = w.Write([]byte(refreshBody))
		case "/api/auth/regionToken":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":{"token":"new-regional","kubeconfig":"apiVersion: v1\nclusters: []\n"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func writeAuthFixture(t *testing.T, home string, data auth.AuthData, kubeconfig string) {
	t.Helper()
	dir := filepath.Join(home, ".sealtun")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kubeconfig"), []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTryAuthRefreshRenewsCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	server := refreshRegionStub(t, http.StatusOK, `{"access_token":"fresh-access","refresh_token":"fresh-refresh","token_type":"Bearer"}`)
	defer server.Close()

	writeAuthFixture(t, home, auth.AuthData{
		Region:        server.URL,
		AccessToken:   "old-access",
		RefreshToken:  "old-refresh",
		RegionalToken: "old-regional",
	}, "old-kubeconfig")

	refreshed, err := tryAuthRefresh()
	if err != nil || !refreshed {
		t.Fatalf("expected refresh, got refreshed=%v err=%v", refreshed, err)
	}

	updated, err := auth.LoadAuthData()
	if err != nil {
		t.Fatal(err)
	}
	if updated.AccessToken != "fresh-access" || updated.RefreshToken != "fresh-refresh" || updated.RegionalToken != "new-regional" {
		t.Fatalf("credentials not renewed: %#v", updated)
	}
	kc, err := os.ReadFile(filepath.Join(home, ".sealtun", "kubeconfig"))
	if err != nil {
		t.Fatal(err)
	}
	if string(kc) != "apiVersion: v1\nclusters: []\n" {
		t.Fatalf("kubeconfig not replaced: %q", kc)
	}
}

func TestTryAuthRefreshKeepsRefreshTokenWhenNotRotated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	server := refreshRegionStub(t, http.StatusOK, `{"access_token":"fresh-access","token_type":"Bearer"}`)
	defer server.Close()

	writeAuthFixture(t, home, auth.AuthData{Region: server.URL, AccessToken: "old", RefreshToken: "keep-me"}, "old-kc")
	refreshed, err := tryAuthRefresh()
	if err != nil || !refreshed {
		t.Fatalf("expected refresh, got %v %v", refreshed, err)
	}
	updated, _ := auth.LoadAuthData()
	if updated.RefreshToken != "keep-me" {
		t.Fatalf("refresh token must survive when the server does not rotate it, got %q", updated.RefreshToken)
	}
}

func TestTryAuthRefreshFailureLeavesCredentialsUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	server := refreshRegionStub(t, http.StatusBadRequest, `{"error":"invalid_grant"}`)
	defer server.Close()

	writeAuthFixture(t, home, auth.AuthData{Region: server.URL, AccessToken: "old-access", RefreshToken: "stale"}, "old-kc")
	refreshed, err := tryAuthRefresh()
	if err != nil || refreshed {
		t.Fatalf("invalid_grant must not refresh, got %v %v", refreshed, err)
	}
	updated, _ := auth.LoadAuthData()
	if updated.AccessToken != "old-access" || updated.RefreshToken != "stale" {
		t.Fatalf("failed refresh must leave credentials untouched: %#v", updated)
	}
	kc, _ := os.ReadFile(filepath.Join(home, ".sealtun", "kubeconfig"))
	if string(kc) != "old-kc" {
		t.Fatalf("kubeconfig must remain untouched on failed refresh")
	}
}

func TestTryAuthRefreshWithoutRefreshToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeAuthFixture(t, home, auth.AuthData{Region: "https://example.invalid", AccessToken: "old"}, "old-kc")
	refreshed, err := tryAuthRefresh()
	if err != nil || refreshed {
		t.Fatalf("legacy session without refresh token must be a no-op, got %v %v", refreshed, err)
	}
}

func TestTryAuthRefreshSyncsActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	server := refreshRegionStub(t, http.StatusOK, `{"access_token":"fresh-access","refresh_token":"fresh-refresh","token_type":"Bearer"}`)
	defer server.Close()

	initial := auth.AuthData{Region: server.URL, AccessToken: "old-access", RefreshToken: "old-refresh"}
	writeAuthFixture(t, home, initial, "old-kc")
	if _, err := auth.SaveProfile("work", initial, "old-kc"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".sealtun")
	if err := os.WriteFile(filepath.Join(dir, "current_profile"), []byte("work"), 0o600); err != nil {
		t.Fatal(err)
	}

	refreshed, err := tryAuthRefresh()
	if err != nil || !refreshed {
		t.Fatalf("expected refresh, got %v %v", refreshed, err)
	}
	profile, _, err := auth.LoadProfile("work")
	if err != nil {
		t.Fatal(err)
	}
	if profile.AuthData.AccessToken != "fresh-access" || profile.AuthData.RefreshToken != "fresh-refresh" {
		t.Fatalf("profile copy not synced: %#v", profile.AuthData)
	}
}
