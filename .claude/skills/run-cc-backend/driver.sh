#!/usr/bin/env bash
# Driver for an isolated cc-backend instance with the internal metric store.
#
# Everything (binary, config, db, job archive, checkpoints) lives in $CC_RUN_DIR,
# so the repo's own ./var is never touched. Run from the repo root.
#
#   driver.sh setup              build binary, write config/archive, init db, mint JWT
#   driver.sh start              start server in background, wait until ready
#   driver.sh stop               stop server and wait for the process to exit
#   driver.sh write FILE|-       POST line protocol to /api/write?cluster=devcluster
#   driver.sh start-job FILE     POST a start_job payload (JSON)
#   driver.sh wait-job JOBID     wait until the job is committed; prints its db id
#   driver.sh gql 'QUERY'        POST a GraphQL query, print the JSON response
#   driver.sh debug HOST         dump the metric store levels/buffers of HOST
#   driver.sh reset-store        stop, delete checkpoints, start (empty metric store)
#   driver.sh smoke              all of the above end to end, with assertions
set -euo pipefail

SKILL_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_DIR=$(cd "$SKILL_DIR/../../.." && pwd)
TMP=${TMPDIR:-/tmp}
RUN_DIR=${CC_RUN_DIR:-${TMP%/}/cc-backend-run}
PORT=${CC_PORT:-18080}
BASE="http://127.0.0.1:$PORT"
CLUSTER=devcluster

die() { echo "driver: $*" >&2; exit 1; }
token() { cat "$RUN_DIR/token" 2>/dev/null || die "no token, run setup first"; }
pid() { cat "$RUN_DIR/server.pid" 2>/dev/null || true; }
running() { local p; p=$(pid); [ -n "$p" ] && kill -0 "$p" 2>/dev/null; }
pause() { python3 -c "import time; time.sleep($1)"; }

# Run the binary with config secrets only: $VAR and $VAR_FILE take precedence
# over config.json, so a key exported in the caller's shell would break auth.
ccb() {
  (cd "$RUN_DIR" && env -u JWT_PUBLIC_KEY -u JWT_PRIVATE_KEY \
    -u JWT_PUBLIC_KEY_FILE -u JWT_PRIVATE_KEY_FILE \
    ./cc-backend -config ./config.json "$@")
}

cmd_setup() {
  running && die "server is running (pid $(pid)), stop it first"
  if [ -e "$RUN_DIR" ]; then
    [ -e "$RUN_DIR/.cc-backend-run" ] || die "$RUN_DIR exists and is not a driver run dir; refusing to delete it"
    rm -rf "$RUN_DIR"
  fi
  mkdir -p "$RUN_DIR/var/job-archive/$CLUSTER"
  touch "$RUN_DIR/.cc-backend-run"

  echo "building cc-backend ..."
  (cd "$REPO_DIR" && go build -o "$RUN_DIR/cc-backend" ./cmd/cc-backend)

  echo 3 > "$RUN_DIR/var/job-archive/version.txt"
  cp "$SKILL_DIR/cluster.json" "$RUN_DIR/var/job-archive/$CLUSTER/cluster.json"

  # Dev keys from configs/config-demo.json, never for production.
  python3 - "$REPO_DIR/configs/config-demo.json" "$RUN_DIR/config.json" "$PORT" <<'EOF'
import json, sys
demo = json.load(open(sys.argv[1]))
json.dump({
    "main": {"addr": f"127.0.0.1:{sys.argv[3]}", "db": "./var/job.db"},
    "cron": {"commit-job-worker": "5s"},
    "auth": demo["auth"],
    "archive": {"kind": "file", "path": "./var/job-archive"},
    "metric-store": {
        "retention-in-memory": "24h",
        "memory-cap": 100,
        "checkpoints": {"file-format": "wal", "directory": "./var/checkpoints"},
    },
}, open(sys.argv[2], "w"), indent=1)
EOF

  ccb -migrate-db -loglevel warn >/dev/null
  ccb -init-db -add-user tester:admin,api:testpw -loglevel warn >/dev/null
  ccb -jwt tester -loglevel warn 2>&1 \
    | grep -oE 'eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' | head -1 > "$RUN_DIR/token"
  [ -s "$RUN_DIR/token" ] || die "could not mint a JWT"
  echo "setup done: $RUN_DIR (user tester/testpw, token in $RUN_DIR/token)"
}

cmd_start() {
  running && { echo "already running (pid $(pid))"; return; }
  [ -x "$RUN_DIR/cc-backend" ] || die "run setup first"
  # exec, so $! is the server itself; the subshell's own redirects keep the
  # caller's stdout pipe from being held open by the background process.
  (cd "$RUN_DIR" && exec env -u JWT_PUBLIC_KEY -u JWT_PRIVATE_KEY \
    -u JWT_PUBLIC_KEY_FILE -u JWT_PRIVATE_KEY_FILE \
    ./cc-backend -config ./config.json -server -dev -loglevel info) \
    > "$RUN_DIR/server.log" 2>&1 < /dev/null &
  echo $! > "$RUN_DIR/server.pid"
  for _ in $(seq 1 60); do
    if [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/login")" = 200 ]; then
      echo "server up at $BASE (pid $(pid), log $RUN_DIR/server.log)"
      return
    fi
    running || { tail -20 "$RUN_DIR/server.log"; die "server exited during startup"; }
    pause 0.5
  done
  tail -20 "$RUN_DIR/server.log"; die "server not ready after 30s"
}

cmd_stop() {
  local p; p=$(pid)
  if [ -z "$p" ] || ! kill -0 "$p" 2>/dev/null; then echo "not running"; return; fi
  kill "$p"
  # The metric store writes its final checkpoint after the port has closed:
  # wait for the process itself, not the port.
  for _ in $(seq 1 120); do kill -0 "$p" 2>/dev/null || break; pause 0.5; done
  kill -0 "$p" 2>/dev/null && die "pid $p did not exit within 60s"
  rm -f "$RUN_DIR/server.pid"
  echo "stopped"
}

cmd_write() {
  local src=${1:?usage: write FILE|-}
  curl -sf -H "Authorization: Bearer $(token)" --data-binary "@$src" \
    "$BASE/api/write?cluster=$CLUSTER" >/dev/null || die "write failed (see $RUN_DIR/server.log)"
  echo "write ok"
}

cmd_start_job() {
  local f=${1:?usage: start-job FILE}
  curl -sf -H "Authorization: Bearer $(token)" -H 'Content-Type: application/json' \
    -d "@$f" "$BASE/api/jobs/start_job/" || die "start_job failed"
  echo
}

cmd_gql() {
  local q=${1:?usage: gql 'QUERY'}
  python3 -c 'import json,sys; print(json.dumps({"query": sys.argv[1]}))' "$q" \
    | curl -s -H "Authorization: Bearer $(token)" -H 'Content-Type: application/json' \
      -d @- "$BASE/query"
  echo
}

cmd_wait_job() {
  local job=${1:?usage: wait-job JOBID} id=""
  for _ in $(seq 1 60); do
    id=$(cmd_gql "{ jobs(filter:[{jobId:{eq:\"$job\"}}]) { items { id } } }" \
      | python3 -c 'import json,sys; i=json.load(sys.stdin)["data"]["jobs"]["items"]; print(i[0]["id"] if i else "")')
    [ -n "$id" ] && { echo "$id"; return; }
    pause 1
  done
  die "job $job not committed after 60s"
}

cmd_debug() {
  curl -s -H "Authorization: Bearer $(token)" "$BASE/api/debug?selector=$CLUSTER:${1:?usage: debug HOST}"
  echo
}

cmd_reset_store() {
  cmd_stop
  rm -rf "$RUN_DIR/var/checkpoints"
  cmd_start
}

# End to end: per-mount filesystem series and their node sum for a running job.
cmd_smoke() {
  cmd_setup
  cmd_start
  trap 'cmd_stop >/dev/null || true' EXIT   # also on a failed assertion
  local now start lines job
  now=$(python3 -c 'import time; print(int(time.time()) // 60 * 60)')
  start=$((now - 600))
  lines="$RUN_DIR/fs.lp"
  python3 - "$lines" "$start" "$now" <<'EOF'
import sys
out, start, now = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
with open(out, "w") as f:
    for ts in range(start, now + 1, 60):
        f.write(f"fs_read_bw,cluster=devcluster,hostname=h1,type=filesystem,type-id=/home value=1 {ts}\n")
        f.write(f"fs_read_bw,cluster=devcluster,hostname=h1,type=filesystem,type-id=/scratch value=2 {ts}\n")
EOF
  cmd_write "$lines"

  job="$RUN_DIR/job.json"
  cat > "$job" <<EOF
{"cluster":"$CLUSTER","subCluster":"a","jobId":4242,"jobState":"running","numAcc":0,
 "numHwthreads":4,"numNodes":1,"partition":"main",
 "resources":[{"hostname":"h1","hwthreads":[0,1,2,3]}],
 "startTime":$start,"submitTime":$start,"user":"tester","project":"p","walltime":86400}
EOF
  cmd_start_job "$job"
  local id; id=$(cmd_wait_job 4242)
  echo "job 4242 committed as id $id"

  cmd_gql "{ jobMetrics(id: \"$id\", metrics: [\"fs_read_bw\"], scopes: [node, filesystem]) {
      name scope metric { series { hostname id data } } } }" > "$RUN_DIR/result.json"
  python3 - "$RUN_DIR/result.json" $(( (now - start) / 60 + 1 )) <<'EOF'
import json, sys
d = json.load(open(sys.argv[1]))
ingested = int(sys.argv[2])
if d.get("errors"):
    sys.exit(f"GraphQL errors: {d['errors']}")
got = {}
for m in d["data"]["jobMetrics"]:
    for s in m["metric"]["series"]:
        vals = [v for v in s["data"] if v is not None]
        key = (m["scope"], s["id"] if m["scope"] != "node" else "node")
        # A running job's window ends at "now": the minute after the last sample
        # may be open, so trailing nulls are fine, gaps and missing samples are not.
        data = list(s["data"])
        while data and data[-1] is None:
            data.pop()
        got[key] = (None not in data and len(data) >= ingested, sorted(set(vals)))
        print(f"{m['name']} scope={m['scope']:10} host={s['hostname']} id={s['id']!s:9} "
              f"points={len(s['data'])} non-null={len(vals)} values={sorted(set(vals))}")
want = {("filesystem", "/home"): 1.0, ("filesystem", "/scratch"): 2.0, ("node", "node"): 3.0}
for key, value in want.items():
    if key not in got:
        sys.exit(f"FAIL: missing series {key}")
    complete, values = got[key]
    if values != [value] or not complete:
        sys.exit(f"FAIL: {key}: values {values} (want {value}), all {ingested} samples without gaps: {complete}")
print("SMOKE OK")
EOF
  cmd_stop
}

case "${1:-}" in
  setup) cmd_setup ;;
  start) cmd_start ;;
  stop) cmd_stop ;;
  write) shift; cmd_write "$@" ;;
  start-job) shift; cmd_start_job "$@" ;;
  wait-job) shift; cmd_wait_job "$@" ;;
  gql) shift; cmd_gql "$@" ;;
  debug) shift; cmd_debug "$@" ;;
  reset-store) cmd_reset_store ;;
  smoke) cmd_smoke ;;
  *) sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
