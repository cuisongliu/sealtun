package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labring/sealtun/pkg/session"
)

func TestStopMarksSessionStoppedBeforePausingResources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	previousPause := pauseSessionResources
	pauseCalled := false
	pauseSessionResources = func(_ context.Context, sess session.TunnelSession) error {
		pauseCalled = true
		latest, err := session.Get(sess.TunnelID)
		if err != nil {
			t.Fatalf("load session during pause: %v", err)
		}
		if latest.ConnectionState != session.ConnectionStateStopped {
			t.Fatalf("expected stopped state before remote pause, got %q", latest.ConnectionState)
		}
		if latest.PID != 0 {
			t.Fatalf("expected PID to be cleared before remote pause, got %d", latest.PID)
		}
		return nil
	}
	t.Cleanup(func() { pauseSessionResources = previousPause })

	if err := session.Save(session.TunnelSession{
		TunnelID:        "stoptun",
		Region:          "https://gzg.sealos.run",
		Namespace:       "ns-test",
		Protocol:        "tcp",
		Host:            "stoptun.example.com",
		LocalPort:       "18081",
		Mode:            "foreground",
		PID:             currentPIDForTest(),
		ConnectionState: session.ConnectionStateConnected,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	cmd := *stopCmd
	cmd.SetContext(context.Background())
	if err := cmd.RunE(&cmd, []string{"stoptun"}); err != nil {
		t.Fatalf("stop command returned error: %v", err)
	}
	if !pauseCalled {
		t.Fatal("expected remote pause to be called")
	}
	latest, err := session.Get("stoptun")
	if err != nil {
		t.Fatalf("load final session: %v", err)
	}
	if latest.ConnectionState != session.ConnectionStateStopped {
		t.Fatalf("expected final stopped state, got %q", latest.ConnectionState)
	}
}

func TestStopRestoresSessionWhenRemotePauseFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	previousPause := pauseSessionResources
	pauseSessionResources = func(context.Context, session.TunnelSession) error {
		return fmt.Errorf("api unavailable")
	}
	t.Cleanup(func() { pauseSessionResources = previousPause })

	if err := session.Save(session.TunnelSession{
		TunnelID:        "stopfail",
		Region:          "https://gzg.sealos.run",
		Namespace:       "ns-test",
		Protocol:        "https",
		Host:            "stopfail.example.com",
		LocalPort:       "18082",
		Mode:            "foreground",
		PID:             currentPIDForTest(),
		ConnectionState: session.ConnectionStateConnected,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	sess, err := findSession("stopfail")
	if err != nil {
		t.Fatal(err)
	}
	_, err = stopTunnelSession(context.Background(), sess)
	if err == nil || !strings.Contains(err.Error(), "api unavailable") {
		t.Fatalf("expected pause failure, got %v", err)
	}
	latest, err := session.Get("stopfail")
	if err != nil {
		t.Fatalf("load final session: %v", err)
	}
	if latest.ConnectionState != session.ConnectionStateConnected {
		t.Fatalf("expected original connected state to be restored, got %q", latest.ConnectionState)
	}
	if latest.PID == 0 {
		t.Fatal("expected original PID to be restored")
	}
	if !strings.Contains(latest.LastError, "api unavailable") {
		t.Fatalf("expected pause failure to be recorded, got %q", latest.LastError)
	}
}

func TestStartRollbackMarksErrorWhenPauseRollbackFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	previousResume := resumeSessionResources
	previousPause := pauseSessionResources
	previousEnsure := ensureDaemonRunningFn
	resumeSessionResources = func(context.Context, session.TunnelSession) error {
		return nil
	}
	pauseSessionResources = func(context.Context, session.TunnelSession) error {
		return fmt.Errorf("pause rollback failed")
	}
	ensureDaemonRunningFn = func() error {
		return fmt.Errorf("daemon unavailable")
	}
	t.Cleanup(func() {
		resumeSessionResources = previousResume
		pauseSessionResources = previousPause
		ensureDaemonRunningFn = previousEnsure
	})

	if err := session.Save(session.TunnelSession{
		TunnelID:        "startfail",
		Region:          "https://gzg.sealos.run",
		Namespace:       "ns-test",
		Protocol:        "https",
		Host:            "startfail.example.com",
		LocalPort:       "18083",
		Secret:          "secret",
		ConnectionState: session.ConnectionStateStopped,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	sess, err := findSession("startfail")
	if err != nil {
		t.Fatal(err)
	}
	err = startTunnelSession(context.Background(), sess)
	if err == nil || !strings.Contains(err.Error(), "rollback pause failed") {
		t.Fatalf("expected rollback pause failure, got %v", err)
	}
	latest, err := session.Get("startfail")
	if err != nil {
		t.Fatalf("load final session: %v", err)
	}
	if latest.ConnectionState != session.ConnectionStateError {
		t.Fatalf("expected error state when rollback pause fails, got %q", latest.ConnectionState)
	}
	if !strings.Contains(latest.LastError, "daemon unavailable") {
		t.Fatalf("expected original start failure in LastError, got %q", latest.LastError)
	}
}
