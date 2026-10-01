---
name: run-cc-backend
description: Build, start, run and drive cc-backend end to end — an isolated server with the internal metric store, ingest line protocol, start jobs, query GraphQL/REST, smoke-test metric queries and scopes. Use when asked to run, start, launch, smoke-test or live-check cc-backend, or to verify a change against the running server rather than only the test suite.
---

cc-backend is a Go HTTP server (REST + GraphQL + embedded Svelte UI). Agents drive it
through `.claude/skills/run-cc-backend/driver.sh`, which builds the binary, runs the
server in the background in an isolated run directory, and wraps `curl` for line-protocol
ingestion, job start, GraphQL and metric-store debugging.

All paths are relative to the repo root. The driver never touches the repo's `./var`
(it may hold the user's real data): binary, config, DB, job archive and checkpoints all
live in `$CC_RUN_DIR` (default `$TMPDIR/cc-backend-run`).

## Prerequisites

Verified on macOS (darwin/arm64) with Go 1.27.1; `go.mod` needs Go ≥ 1.26.3. Also needs
`python3`, `curl` and `bash` on `PATH`. The frontend does **not** need to be built: the
server, REST and GraphQL run fine without `web/frontend/public/build` (only the web UI
pages lack their bundles).

## Run (agent path)

One command, end to end (~13 s; exit code 0 means healthy):

```bash
.claude/skills/run-cc-backend/driver.sh smoke
```

It runs `setup`, `start`, ingests 11 one-minute samples of `fs_read_bw` for `/home` (1) and
`/scratch` (2) on host `h1`, starts running job 4242, waits for it to be committed,
queries `jobMetrics(scopes: [node, filesystem])`, asserts per-mount series 1 and 2 and a
node series 3 without an id, all with no gaps, prints `SMOKE OK`, and stops the server. On a failed
assertion it prints `FAIL: ...`, exits 1, and still stops the server.

Step by step (a running job on `h1` with ten minutes of per-mount data):

```bash
D=.claude/skills/run-cc-backend/driver.sh
$D setup                       # build, config, archive, DB, user tester/testpw, JWT
$D start                       # background server on 127.0.0.1:18080, waits for ready

NOW=$(python3 -c 'import time; print(int(time.time()) // 60 * 60)'); START=$((NOW - 600))
python3 -c "
for ts in range($START, $NOW + 1, 60):
    for mnt, v in (('/home', 1), ('/scratch', 2)):
        print(f'fs_read_bw,cluster=devcluster,hostname=h1,type=filesystem,type-id={mnt} value={v} {ts}')
" | $D write -                 # line protocol from stdin (or: $D write FILE)

cat > "${TMPDIR:-/tmp}/job.json" <<JSON
{"cluster":"devcluster","subCluster":"a","jobId":4243,"jobState":"running","numNodes":1,
 "numHwthreads":4,"resources":[{"hostname":"h1"}],"startTime":$START,"submitTime":$START,
 "user":"tester","project":"p","walltime":86400}
JSON
$D start-job "${TMPDIR:-/tmp}/job.json"   # POST /api/jobs/start_job/
ID=$($D wait-job 4243)                    # db id once the job is committed
$D gql "{ jobMetrics(id: \"$ID\", metrics: [\"fs_read_bw\"], scopes: [node, filesystem]) { scope metric { series { id data } } } }"
$D debug h1                    # metric store levels and buffers of host h1
$D reset-store                 # stop, wipe checkpoints, start with an empty store
$D stop                        # waits for the process to exit
```

| command | what it does |
|---|---|
| `setup` | rebuilds the run dir from scratch (refuses to delete a dir it did not create) |
| `start` / `stop` | background launch with readiness poll on `/login`; stop waits for the PID to exit |
| `write FILE\|-` | `POST /api/write?cluster=devcluster`, Bearer JWT |
| `start-job FILE` | `POST /api/jobs/start_job/` with a JSON payload |
| `wait-job JOBID` | polls GraphQL `jobs` until the job appears, prints its `id` |
| `gql 'QUERY'` | raw GraphQL query text, JSON-wrapped and posted to `/query` |
| `debug HOST` | `GET /api/debug?selector=devcluster:HOST` |
| `reset-store` | empty metric store, keeps DB and jobs |
| `smoke` | the full flow above, with assertions |

Overrides: `CC_RUN_DIR=/some/dir` and `CC_PORT=18081` (both verified). Log:
`$CC_RUN_DIR/server.log`; token: `$CC_RUN_DIR/token`; last smoke result:
`$CC_RUN_DIR/result.json`.

The fixture cluster `devcluster` is `.claude/skills/run-cc-backend/cluster.json`:
subcluster `a` = host `h1` (filesystems `/home`, `/scratch`, accelerators `0`,`1`,
network `ib0`), `b` = `h2` (`/work`), `c` = `h3` (no devices). Metrics: `flops_any`
(hwthread), `fs_read_bw` (filesystem, sum), `acc_util` (accelerator, avg). Edit it to
test other topologies; `setup` copies it into the archive.

## Run (human path)

`./startDemo.sh` and `./cc-backend -server -dev` run against the repo's own
`./config.json` and `./var`, which may hold the user's real data. This skill does not
exercise them; agents use the driver.

## Test

```bash
go test ./...
```

All packages pass. The run rewrites `internal/repository/testdata/job.db`; restore it
with `git checkout -- internal/repository/testdata/job.db` before committing.

## Gotchas

- **Device data must be tagged `type=<device>,type-id=<id>`** (as for accelerators), e.g.
  `fs_read_bw,cluster=devcluster,hostname=h1,type=filesystem,type-id=/home value=1 <ts>`.
  `type=node,stype=filesystem,stype-id=…` lands in the host buffer itself, not on a
  device level, so filesystem-scope queries return nothing.
- **Running jobs are invisible to GraphQL until committed.** `start_job` writes to the job
  cache; the commit service moves it every `cron.commit-job-worker` (default 2 min). The
  driver config sets `5s`; `wait-job` polls for it.
- **Secrets in the caller's env override `config.json`.** An exported `JWT_PUBLIC_KEY` /
  `JWT_PRIVATE_KEY` (or `*_FILE`) would make the minted token invalid; the driver unsets
  them for every invocation.
- **The final checkpoint is written after the port closes.** Deleting `var/checkpoints`
  as soon as the port is free resurrects old samples on the next start, and earlier
  timestamps are then rejected with `cannot write value to buffer from past`. `stop` and
  `reset-store` wait for the process itself.
- **Samples older than a buffer's start are dropped.** Ingest each series in time order,
  and use `reset-store` before backfilling a window that starts earlier.
- **A running job's query window ends at "now".** If a minute boundary passes after the
  last ingested sample, the series gains a trailing `null` for the open minute. That is
  correct, not data loss; `smoke` tolerates trailing nulls but not gaps.
- **Fresh DB needs `-migrate-db` before `-init-db`**; `setup` does both.

## Troubleshooting

- **`unsupported database version 0, need 13`**: the DB was never migrated. Run `setup`
  again (it migrates first).
- **Pipeline hangs after `start`** (e.g. `driver.sh start | tail`): a background process
  is holding the pipe. The driver redirects the server's stdio; if you launch the binary
  yourself, use `( exec ./cc-backend ... ) > log 2>&1 < /dev/null &`.
- **macOS `seq` prints epoch seconds as `1.79084e+09`**: every line gets the same broken
  timestamp and only one sample lands. Generate timestamps with `python3` (as `smoke` does).
- **`refusing to delete it`** from `setup`: `CC_RUN_DIR` points at a directory the driver
  did not create. Pick another path.
