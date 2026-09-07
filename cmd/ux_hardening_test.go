package cmd

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/labring/sealtun/pkg/routes"
	"github.com/labring/sealtun/pkg/session"
)

func TestProbeRouteHealthReportsPerRoute(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	livePort := listener.Addr().(*net.TCPAddr).Port

	sess := session.TunnelSession{Routes: []routes.Route{
		{Path: "/api", Port: livePort},
		{Path: "/dead", Port: 1},
	}}
	health := probeRouteHealth(sess)
	if len(health) != 2 {
		t.Fatalf("expected two route health entries, got %#v", health)
	}
	if !health[0].Reachable {
		t.Fatal("live route port must be reachable")
	}
	if health[1].Reachable {
		t.Fatal("dead route port must be unreachable")
	}
	if !anyRouteUnreachable(health) {
		t.Fatal("anyRouteUnreachable must see the dead route")
	}
	if anyRouteUnreachable(health[:1]) {
		t.Fatal("all-healthy routes must not flag")
	}
	if probeRouteHealth(session.TunnelSession{}) != nil {
		t.Fatal("no routes must yield nil health")
	}
}

func TestRouteHealthSuffix(t *testing.T) {
	health := []RouteHealth{{Path: "/api", Port: 8080, Reachable: false}}
	if got := routeHealthSuffix(health, routes.Route{Path: "/api", Port: 8080}); got != ", UNREACHABLE" {
		t.Fatalf("dead route suffix = %q", got)
	}
	health[0].Reachable = true
	if got := routeHealthSuffix(health, routes.Route{Path: "/api/", Port: 8080}); got != ", reachable" {
		t.Fatalf("normalized match suffix = %q", got)
	}
	if got := routeHealthSuffix(health, routes.Route{Path: "/other", Port: 9090}); got != "" {
		t.Fatalf("unmatched route must have empty suffix, got %q", got)
	}
}

func TestRoutesDoctorCheck(t *testing.T) {
	if _, ok := routesDoctorCheck(session.TunnelSession{}, nil); ok {
		t.Fatal("no routes must skip the check")
	}
	sess := session.TunnelSession{Routes: []routes.Route{{Path: "/api", Port: 8080}}}
	stopped := sess
	stopped.ConnectionState = session.ConnectionStateStopped
	if check, ok := routesDoctorCheck(stopped, nil); !ok || check.Status != "skip" {
		t.Fatalf("stopped tunnel must skip, got %#v", check)
	}
	check, _ := routesDoctorCheck(sess, []RouteHealth{{Path: "/api", Port: 8080, Reachable: true}})
	if check.Status != "ok" {
		t.Fatalf("healthy routes must be ok, got %#v", check)
	}
	check, _ = routesDoctorCheck(sess, []RouteHealth{{Path: "/api", Port: 8080, Reachable: false}})
	if check.Status != "fail" || !strings.Contains(check.Detail, "/api -> localhost:8080") {
		t.Fatalf("dead route must fail with target detail, got %#v", check)
	}
}

func TestListTargetLabel(t *testing.T) {
	plain := session.TunnelSession{Protocol: "https", LocalPort: "3000"}
	if label := listTargetLabel(plain); strings.Contains(label, "route") {
		t.Fatalf("plain tunnel must not show a route marker, got %q", label)
	}
	routed := session.TunnelSession{Protocol: "https", LocalPort: "3000", Routes: []routes.Route{
		{Path: "/api", Port: 8080}, {Path: "/admin", Port: 9090},
	}}
	if label := listTargetLabel(routed); !strings.HasSuffix(label, "(+2 routes)") {
		t.Fatalf("routed tunnel must show route count, got %q", label)
	}
}

func TestQrNarrowTerminalWarningIgnoresNonTTY(t *testing.T) {
	if warning := qrNarrowTerminalWarning(&bytes.Buffer{}); warning != "" {
		t.Fatalf("non-terminal writers must not warn, got %q", warning)
	}
}
