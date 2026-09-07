package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRefreshTokensSuccess(t *testing.T) {
	var gotGrant, gotRefresh, gotClientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/oauth2/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		gotGrant = formValue(string(body), "grant_type")
		gotRefresh = formValue(string(body), "refresh_token")
		gotClientID = formValue(string(body), "client_id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh", TokenType: "Bearer"})
	}))
	defer server.Close()

	res, err := RefreshTokens(server.URL, "old-refresh")
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if res.AccessToken != "new-access" || res.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected token response: %#v", res)
	}
	if gotGrant != "refresh_token" || gotRefresh != "old-refresh" || gotClientID != ClientID {
		t.Fatalf("unexpected form: grant=%q refresh=%q client=%q", gotGrant, gotRefresh, gotClientID)
	}
}

func TestRefreshTokensInvalidGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token expired"}`))
	}))
	defer server.Close()

	_, err := RefreshTokens(server.URL, "stale")
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected ErrInvalidGrant, got %v", err)
	}
}

func TestRefreshTokensTransientFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := RefreshTokens(server.URL, "stale")
	if err == nil || errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expected transient error, got %v", err)
	}
}

func TestRefreshTokensRejectsEmptyAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"","token_type":"Bearer"}`))
	}))
	defer server.Close()

	if _, err := RefreshTokens(server.URL, "x"); err == nil || !strings.Contains(err.Error(), "empty access token") {
		t.Fatalf("expected empty-token rejection, got %v", err)
	}
}

func formValue(body, key string) string {
	for _, pair := range strings.Split(body, "&") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 && parts[0] == key {
			return parts[1]
		}
	}
	return ""
}
