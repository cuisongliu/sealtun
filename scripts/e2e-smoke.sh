#!/usr/bin/env bash
# e2e-smoke.sh — real-cloud golden-path gate for Sealtun.
#
# go test covers code logic; this script covers what it cannot: real K8s
# provisioning, ingress behavior, daemon lifecycle, and the full user flow
# from expose to cleanup. It is self-cleaning (TTL backstop + trap) and exits
# non-zero on the first failed assertion.
#
# Assertion discipline (learned the hard way): never write
#   cmd 2>&1 | grep -q X && ok || bad
# pipefail plus grep -q's early exit breaks that pattern in both directions.
# Always capture first, then match.
#
# Usage: scripts/e2e-smoke.sh [binary-path]
# Requires: sealtun login already completed on this machine.
set -uo pipefail

BIN="${1:-/tmp/sealtun-e2e}"
PRIMARY_PORT=19391
API_PORT=19392
PASS=0
FAIL=0
APP_PIDS=()

say()  { printf '%s\n' "$*"; }
ok()   { PASS=$((PASS+1)); say "  PASS  $1"; }
bad()  { FAIL=$((FAIL+1)); say "  FAIL  $1"; }

# expect <label> <command...> — command must succeed.
expect() {
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then ok "$label"; else bad "$label"; fi
}

# capture <command...> — print stdout+stderr, never fail the caller.
capture() { "$@" 2>&1 || true; }

cleanup() {
  "$BIN" cleanup --all --yes >/dev/null 2>&1
  for pid in "${APP_PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null
  done
}
trap cleanup EXIT

say "== build =="
go build -o "$BIN" . || { say "build failed"; exit 1; }

say "== local apps =="
start_app() { # port, marker
  python3 - "$1" "$2" <<'PYEOF' &
import http.server, sys
port, marker = int(sys.argv[1]), sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = (marker + ":" + self.path).encode()
        self.send_response(200)
        self.send_header("content-type", "text/plain")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
PYEOF
  APP_PIDS+=($!)
}
start_app "$PRIMARY_PORT" "primary"
start_app "$API_PORT" "api"
sleep 1
OUT=$(curl -sf "http://127.0.0.1:$PRIMARY_PORT/health" || true)
[ "$OUT" = "primary:/health" ] && ok "primary app up" || { bad "primary app failed to start"; exit 1; }
OUT=$(curl -sf "http://127.0.0.1:$API_PORT/health" || true)
[ "$OUT" = "api:/health" ] && ok "api app up" || { bad "api app failed to start"; exit 1; }

say "== validation rejects before provisioning (no tunnels created) =="
OUT=$(capture "$BIN" expose "$PRIMARY_PORT" --route bad)
grep -q "invalid route" <<<"$OUT" && ok "malformed --route rejected" || bad "malformed --route accepted: $OUT"
OUT=$(capture "$BIN" expose "$PRIMARY_PORT" --route /a=1 --protocol ssh)
grep -q "only supported for https" <<<"$OUT" && ok "--route on ssh rejected" || bad "--route on ssh accepted: $OUT"
OUT=$(capture "$BIN" expose "$PRIMARY_PORT" --route /a=70000)
grep -q "between 1 and 65535" <<<"$OUT" && ok "out-of-range route port rejected" || bad "out-of-range route port accepted: $OUT"

say "== create routed tunnel (ttl backstop 15m) =="
OUT=$(capture "$BIN" expose "$PRIMARY_PORT" --route "/api=$API_PORT" --ttl 15m)
TUNNEL_URL=$(grep -o 'https://[^ ]*' <<<"$OUT" | head -1)
TUNNEL_ID=$(sed -E 's|https://sealtun-([0-9a-f]+)-.*|\1|' <<<"$TUNNEL_URL")
[ -n "$TUNNEL_URL" ] && ok "tunnel created: $TUNNEL_URL" || { bad "expose failed: $OUT"; exit 1; }
grep -q "auto-expires" <<<"$OUT" && ok "ttl summary line printed" || bad "ttl summary missing"

say "== dispatch =="
sleep 2
OUT=$(curl -sf "$TUNNEL_URL/health" || true)
[ "$OUT" = "primary:/health" ] && ok "fallback to primary" || bad "fallback wrong: $OUT"
OUT=$(curl -sf "$TUNNEL_URL/api/x" || true)
[ "$OUT" = "api:/x" ] && ok "prefix stripped to api app" || bad "route dispatch wrong: $OUT"
OUT=$(curl -sf "$TUNNEL_URL/apiserver" || true)
[ "$OUT" = "primary:/apiserver" ] && ok "segment boundary respected" || bad "/apiserver escaped the prefix: $OUT"
# A handshake aimed at a non-upgrading path must still be answered by the
# routed app (status proves dispatch through the chain). The public edge may
# strip the body of non-101 upgrade responses, so assert the status only; the
# local chain's body preservation is locked by an in-process integration test.
OUT=$(curl -s -o /dev/null -w '%{http_code}' --http1.1 --max-time 5 \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: x3JJHMbDL1EzLkh9GBhXDw==" \
  "$TUNNEL_URL/api/ws" || true)
[ "$OUT" = "200" ] && ok "upgrade request reaches routed app" || bad "upgrade did not reach routed app: status $OUT"

say "== observability =="
OUT=$(capture "$BIN" inspect "$TUNNEL_ID")
grep -q "Route: /api -> localhost:$API_PORT" <<<"$OUT" && ok "inspect shows route" || bad "inspect missing route: $OUT"
OUT=$(capture "$BIN" list)
grep -qF "(+1 route)" <<<"$OUT" && ok "list shows route marker" || bad "list missing route marker: $OUT"
OUT=$(capture "$BIN" doctor "$TUNNEL_ID")
grep -q "routes: ok" <<<"$OUT" && ok "doctor routes check ok" || bad "doctor routes check wrong: $OUT"

say "== lifecycle: stop, start, cleanup =="
expect "stop" "$BIN" stop "$TUNNEL_ID"
"$BIN" start "$TUNNEL_ID" >/dev/null 2>&1
sleep 2
OUT=$(curl -sf "$TUNNEL_URL/api/x" || true)
[ "$OUT" = "api:/x" ] && ok "start restores dispatch" || bad "start did not restore dispatch: $OUT"
"$BIN" cleanup --all --yes >/dev/null 2>&1
OUT=$(capture "$BIN" list)
grep -q "$TUNNEL_ID" <<<"$OUT" && bad "session remains after cleanup" || ok "cleanup removes session"

say "== root route shadow warning =="
OUT=$(capture "$BIN" expose "$PRIMARY_PORT" --route "/=$PRIMARY_PORT" --ttl 5m)
grep -q "matches every path" <<<"$OUT" && ok "root route warning shown" || bad "root route warning missing: $OUT"
"$BIN" cleanup --all --yes >/dev/null 2>&1

say ""
say "== result: $PASS passed, $FAIL failed =="
[ "$FAIL" = "0" ]
