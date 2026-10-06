# Fleet Service — Discovery and Configuration Deployment

The fleet service lets auxiliary ClusterCockpit services (`cc-metric-store`,
`cc-metric-collector`, `cc-slurm-adapter`, …) register with cc-backend, receive
their configuration from a central, hand-edited tree of JSON files, and discover
each other over NATS — without every service carrying a hand-maintained copy of
the site topology.

This document covers both sides: how an operator turns the service on in
cc-backend, and how a client service integrates with it.

---

## Table of contents

- [1. Concepts](#1-concepts)
- [2. How it fits together](#2-how-it-fits-together)
- [3. Server setup](#3-server-setup)
- [4. Authoring the configuration tree](#4-authoring-the-configuration-tree)
- [5. Client integration tutorial](#5-client-integration-tutorial)
- [6. Reference](#6-reference)
- [7. Security](#7-security)
- [8. Operations and troubleshooting](#8-operations-and-troubleshooting)

---

## 1. Concepts

### Service type

Every fleet member declares what kind of service it is. The short code is the
canonical value: it is stored in the database, used as a directory name in the
configuration tree, and used as a NATS subject token.

| Code   | Service             |
| ------ | ------------------- |
| `ccms` | cc-metric-store     |
| `ccmc` | cc-metric-collector |
| `ccb`  | cc-backend          |
| `cces` | cc-event-store      |
| `ccsa` | cc-slurm-adapter    |
| `ccnc` | cc-node-controller  |
| `ccem` | cc-energy-manager   |

An unknown code is rejected at registration with `400`.

The codes, like every other wire definition of the fleet protocol (registration
bodies, roster entry, heartbeat and roster encoding), are defined once in the
cc-lib [`fleet`](https://github.com/ClusterCockpit/cc-lib/tree/main/fleet)
package (`fleet.ServiceTypes`) and used by both cc-backend and its members.

### Scope

A registration is either **cluster-scope** or **infra-scope**:

- **cluster** — the service belongs to exactly one cluster. Identity is
  `(cluster, hostname, serviceType)`. This is what a per-node agent uses.
- **infra** — the service is cluster-independent monitoring infrastructure.
  Identity is `(hostname, serviceType)`; the cluster column stays empty.

The scope is chosen by the registration endpoint you call, not by a field in the
body. It determines both the configuration layers that apply and the discovery
bucket the service appears in.

### Instance ID

Registration returns an `instanceId`: 16 random bytes, hex-encoded (32
characters). It is the credential for every subsequent call — heartbeat, config
pull, deregistration. **Treat it as a bearer token**: do not log it, do not put
it in a world-readable file, do not publish it.

Re-registering the same `(cluster, hostname, serviceType)` — e.g. after a
service restart — issues a **new** instance id and invalidates the previous one.
The stored `config_revision` and `last_heartbeat` survive; the state resets to
`pending` until the next heartbeat.

### Lifecycle

```
           register (REST)
                 │
                 ▼
            ┌─────────┐   first heartbeat   ┌────────┐
            │ pending │ ──────────────────► │ active │
            └─────────┘                     └────────┘
                 │                            │    ▲
                 │                no heartbeat │    │ heartbeat
                 │                for          ▼    │
                 │             stale-after  ┌───────┐
                 │                          │ stale │
                 │                          └───────┘
                 │                              │
                 └──────────► deregistered ◄────┘
                              (terminal, REST only)
```

Only `active` services appear in discovery rosters. `deregistered` is terminal:
the identity comes back only through a fresh REST registration.

### Config revision

The revision is a 64-bit FNV-1a content hash of the _merged_ configuration blob,
so it changes exactly when the effective configuration of that member changes —
no manual counter to keep in sync with the files. It is served as an HTTP
`ETag`, which makes the steady-state config poll a header-only round trip.

---

## 2. How it fits together

```
   ┌────────────────────────── cc-backend ──────────────────────────┐
   │                                                                │
   │  REST /api/fleet/*        Registry / InfraRegistry             │
   │      register  ──────────►  service table (SQLite)             │
   │      config    ◄───── ConfigStore ◄── config tree on disk      │
   │      heartbeat ──────────►  (stale sweep ages rows)            │
   │      deregister ─────────►                                     │
   │                                                                │
   │  NATS  <heartbeat-subject>  ──► batched heartbeat consumer     │
   │  NATS  <discovery-prefix>.* ◄── FleetPublisher (rosters)       │
   └────────────────────────────────────────────────────────────────┘
            ▲  ▲                                        │
   register │  │ config pull (ETag)                     │ roster
   heartbeat│  │                                        ▼
   ┌────────┴──┴────────────────────────────────────────────────────┐
   │  cc-metric-collector, cc-metric-store, cc-slurm-adapter, …     │
   └────────────────────────────────────────────────────────────────┘
```

Three independent goroutines run inside cc-backend once the subsystem is
enabled:

1. **Stale sweep** — every `sweep-interval`, flips rows whose last heartbeat is
   older than `stale-after` to `stale`.
2. **Config reloader** — every `config-reload-interval`, re-scans the
   configuration tree and swaps in a new immutable snapshot atomically. Pulls
   never touch the filesystem.
3. **Discovery publisher** — every `discovery-interval`, and immediately
   (debounced by 2 s) on registration or deregistration, publishes one roster
   per (bucket, consumer type) subject. Only started when NATS is connected.

---

## 3. Server setup

### 3.1 Enable the subsystem

The fleet service is **off** unless the `main.fleet` block exists in
`config.json`. An absent block disables the registry, configuration deployment,
discovery publishing and the heartbeat subscription together — the
`/api/fleet/*` routes are not even mounted.

```json
{
  "main": {
    "api-allowed-ips": ["10.0.0.0/8"],

    "fleet": {
      "config-dir": "./var/fleet-config",
      "stale-after": "90s",
      "sweep-interval": "30s",
      "config-reload-interval": "1m",
      "heartbeat-subject": "cc.fleet.event",
      "heartbeat-concurrency": 2,
      "discovery-subject-prefix": "cc.fleet.discovery",
      "discovery-interval": "1m"
    }
  },

  "nats": {
    "address": "nats://localhost:4222",
    "username": "cc",
    "password": "…"
  }
}
```

| Key                        | Required | Default              | Meaning                                          |
| -------------------------- | -------- | -------------------- | ------------------------------------------------ |
| `config-dir`               | yes¹     | —                    | Root of the configuration tree served to members |
| `stale-after`              | no       | `90s`                | Heartbeat age at which a service becomes `stale` |
| `sweep-interval`           | no       | `30s`                | How often the stale sweep runs                   |
| `config-reload-interval`   | no       | `1m`                 | How often the config tree is re-scanned          |
| `heartbeat-subject`        | no       | — (disabled)         | NATS subject carrying heartbeats                 |
| `heartbeat-concurrency`    | no       | `2`                  | Worker goroutines decoding heartbeats            |
| `discovery-subject-prefix` | no       | `cc.fleet.discovery` | Roster subject prefix                            |
| `discovery-interval`       | no       | `1m`                 | How often all rosters are re-published           |

¹ Technically optional: with an empty `config-dir` every config pull answers
`204` (nothing to deploy). Registration, heartbeats and discovery still work.

Durations are `time.ParseDuration` strings (`"90s"`, `"1m30s"`). A malformed
value logs a warning and falls back to the default rather than failing startup.

### 3.2 NATS (optional but recommended)

Without a `nats` block, everything still works over REST — clients heartbeat via
`POST /api/fleet/heartbeat/{instanceID}` and no discovery rosters are published
(you will see `fleet: NATS client unavailable, discovery rosters will not be
published` at startup).

With NATS configured, prefer the heartbeat subject: heartbeats from the whole
fleet are coalesced into at most one database transaction per second, instead of
one HTTP request plus one fsync per member per interval.

### 3.3 Database

The `service` table is created by migration `13_add-service-table`, applied
automatically at startup. No manual step.

### 3.4 API credentials for members

Fleet endpoints live under `/api` and require a JWT carrying the `api` role,
plus — when `main.api-allowed-ips` is configured — an allowlisted source IP.
**Every fleet member's IP has to be covered by that allowlist.**

Create a machine account and mint a token:

```bash
./cc-backend -add-user fleetbot:api:'<password>'
./cc-backend -jwt fleetbot
```

Distribute the printed token to the member services. They send it as either
header:

```
X-Auth-Token: <jwt>
Authorization: Bearer <jwt>
```

### 3.5 Verify

Start the server and look for:

```
fleet: initialized (config-dir './var/fleet-config', stale-after 1m30s, sweep 30s, reload 1m, discovery 1m)
Enabling REST fleet service API
NATS fleet heartbeat subscription started on subject "cc.fleet.event" (queue group "cc-backend-fleet") — …
fleet: publishing discovery rosters under "cc.fleet.discovery" — …
```

The two long warnings are intentional; see [Security](#7-security).

With `-dev`, the Swagger UI documents the endpoints under the **Fleet** tag.

---

## 4. Authoring the configuration tree

The tree is hand-edited JSON under `config-dir`. Configuration is resolved by
**deep-merging from broad to specific**, so a value set once high in the tree is
inherited everywhere below and only overridden where it differs.

### 4.1 Layout

```
<config-dir>/
  defaults.json                       # every member, every type
  <service_type>/
    defaults.json                     # all instances of this type
    <cluster>/defaults.json           # cluster scope: per-cluster defaults
    <cluster>/<hostname>.json         # cluster scope: one host
    <hostname>.json                   # infra scope: one host
```

Missing layers are skipped — never an error. A member for which _no_ layer
exists at all gets `204 No Content` on its config pull.

### 4.2 Merge order

```
cluster scope:  defaults.json → <type>/defaults.json → <type>/<cluster>/defaults.json → <type>/<cluster>/<host>.json
infra scope:    defaults.json → <type>/defaults.json → <type>/<host>.json
```

Objects merge recursively. **Scalars and arrays overwrite wholesale** — there is
no array append; a host-level list replaces the inherited list entirely.

Every layer file must contain a JSON **object** at the top level. An array or
scalar is a configuration error and fails the whole scan.

### 4.3 Worked example

```
var/fleet-config/
  defaults.json
  ccmc/
    defaults.json
    fritz/
      defaults.json
      f0101.json
```

`defaults.json`:

```json
{
  "log-level": "warn",
  "sinks": { "type": "nats", "address": "nats://nats01:4222" }
}
```

`ccmc/defaults.json`:

```json
{
  "interval": "10s",
  "collectors": ["cpustat", "memstat", "loadavg"]
}
```

`ccmc/fritz/defaults.json`:

```json
{
  "interval": "5s",
  "sinks": { "subject": "cc.metric.fritz" }
}
```

`ccmc/fritz/f0101.json`:

```json
{
  "log-level": "debug",
  "collectors": ["cpustat", "memstat", "loadavg", "nvidia"]
}
```

A `ccmc` on `fritz`/`f0101` resolves to:

```json
{
  "collectors": ["cpustat", "memstat", "loadavg", "nvidia"],
  "interval": "5s",
  "log-level": "debug",
  "sinks": {
    "address": "nats://nats01:4222",
    "subject": "cc.metric.fritz",
    "type": "nats"
  }
}
```

Note how `sinks` merged key-by-key while `collectors` was replaced outright.

### 4.4 Editing safely

Edits are picked up within `config-reload-interval`; there is no reload signal
and none is needed. Two properties protect running services while you edit:

- **No torn reads.** A file caught mid-write either fails to parse or changes
  size/mtime across its own read; either way the scan is discarded and the
  previously published snapshot keeps serving.
- **Last known good.** A generation is all-or-nothing. A syntax error anywhere
  in the tree means the new generation is not published — members keep the last
  configuration that parsed cleanly, and the failure is logged:
  `fleet: config reload failed, keeping previous snapshot: …`.

At startup the tree is loaded **synchronously**, so a malformed tree fails
startup instead of silently serving nothing to the whole fleet. An absent
`config-dir` is treated as a valid empty tree, not an error.

Because the revision is a content hash, a cosmetic edit (reformatting, key
reordering) produces the same revision and does **not** cause members to
re-apply their configuration.

---

## 5. Client integration tutorial

The full client lifecycle is: **register → pull config → heartbeat forever →
(subscribe to discovery) → deregister on shutdown.**

A Go service should not implement this itself: the cc-lib
[`fleet`](https://github.com/ClusterCockpit/cc-lib/tree/main/fleet) client
drives the whole lifecycle (see [5.1](#51-go-client-cc-lib-fleet)). The steps
below document the protocol for other languages and for debugging with `curl`.

Throughout, `$CCB` is the cc-backend base URL and `$TOKEN` the API JWT.

### Step 1 — Register

Cluster-scope service:

```bash
curl -sS -X POST "$CCB/api/fleet/register/cluster/" \
  -H "X-Auth-Token: $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
        "cluster":     "fritz",
        "hostname":    "f0101",
        "serviceType": "ccmc",
        "metaData":    {"version": "0.7.1", "port": "8080"}
      }'
```

Infra-scope service (no `cluster` field, different path):

```bash
curl -sS -X POST "$CCB/api/fleet/register/infra/" \
  -H "X-Auth-Token: $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"hostname": "mgmt01", "serviceType": "ccms", "metaData": {"port": "8081"}}'
```

Response — `201 Created`:

```json
{ "instanceId": "3f1c9a2b7d4e6f8a0b1c2d3e4f5a6b7c", "configRevision": 0 }
```

- `configRevision: 0` means nothing has been pulled yet — pull your initial
  configuration before starting work.
- A non-zero value is the revision this identity last acknowledged (it survives
  re-registration), so a restarting service can skip re-applying an unchanged
  configuration.

`metaData` is free-form `string → string`. It is **broadcast unauthenticated**
in discovery rosters, so put connection hints there (port, version, capability
flags) and never secrets.

Registration is validated: `hostname` is required, `serviceType` must be known,
and `hostname`/`cluster` must be safe path components (no `/`, `\`, `.`, `..`) —
they become directory names in the configuration tree.

### Step 2 — Pull configuration

```bash
curl -sS -D- "$CCB/api/fleet/config/$INSTANCE_ID" -H "X-Auth-Token: $TOKEN"
```

```
HTTP/1.1 200 OK
ETag: "4611686018427387904"
X-CC-Config-Revision: 4611686018427387904
Content-Type: application/json

{ "collectors": [ … ], "interval": "5s", … }
```

The body **is** the merged configuration object — there is no envelope. The
revision travels in `ETag` and, identically, in `X-CC-Config-Revision`.

On every later poll, send the revision back:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' \
  "$CCB/api/fleet/config/$INSTANCE_ID" \
  -H "X-Auth-Token: $TOKEN" \
  -H 'If-None-Match: "4611686018427387904"'
# → 304
```

Status codes:

| Code  | Meaning                                   | Client action                                           |
| ----- | ----------------------------------------- | ------------------------------------------------------- |
| `200` | Configuration returned                    | Apply it, store the ETag                                |
| `204` | No configuration authored for this member | Normal state — run with built-in defaults, keep polling |
| `304` | Unchanged since your `If-None-Match`      | Do nothing                                              |
| `404` | Unknown or deregistered instance          | Register again to get a new instance id                 |

`204` is **not** an error. Many sites deploy configuration to only some service
types; answering `404` there would produce a warning per member per poll forever.

`If-None-Match` is parsed leniently: wildcard `*`, comma-separated lists, weak
validators (`W/"…"`) and unquoted values all work, so an ordinary HTTP client
library needs no special handling.

A convenient poll interval is the same as your heartbeat interval or a small
multiple of it; the unchanged case costs a header-only round trip and no
database write.

### Step 3 — Heartbeat

The heartbeat is what moves a registration from `pending` to `active` and keeps
it out of `stale`. Send one comfortably more often than `stale-after` —
with the default `90s`, every 30 s is a good choice.

**Over NATS (preferred when a broker is deployed).** Publish an InfluxDB
line-protocol event to the configured `heartbeat-subject`:

```
fleet,function=heartbeat event="{\"instanceId\":\"3f1c9a2b7d4e6f8a0b1c2d3e4f5a6b7c\"}" 1734000000000000000
```

- Measurement must be `fleet`; tag `function` must be `heartbeat` — it is the
  only function accepted on this subject. `register`, `deregister` and config
  operations are rejected with a warning: they require authenticated REST.
- The `event` field is a JSON object with exactly one key, `instanceId`. Unknown
  fields are rejected.
- The message timestamp is **ignored** in favour of the server clock, so a
  skewed publisher cannot park `last_heartbeat` in the future.
- Several heartbeat lines may share one message — that is the supported way for
  an edge aggregator to batch heartbeats for many instances.
- A heartbeat for an unknown or deregistered instance id is a silent no-op
  (counted and summarised once per minute server-side, never upserted). If your
  service was deregistered or the id was rotated, only a fresh REST registration
  brings it back — so treat a `404` from a config pull as the signal to
  re-register.

**Over REST (deployments without NATS):**

```bash
curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
  "$CCB/api/fleet/heartbeat/$INSTANCE_ID" -H "X-Auth-Token: $TOKEN"
# → 204, or 404 for an unknown/deregistered instance
```

### Step 4 — Subscribe to discovery

cc-backend publishes a ready-to-use provider list per consumer type. A client
subscribes to **exactly one subject** and needs no filtering logic:

```
<discovery-subject-prefix>.<bucket>.<own-service-type>
```

- `bucket` is your cluster name for a cluster-scope service, or the literal
  `infra` for an infra-scope service.
- Examples: `cc.fleet.discovery.fritz.ccmc`, `cc.fleet.discovery.infra.ccms`.

A cluster-scope consumer on cluster _C_ sees relevant providers in _C_ plus all
infra-scope providers. The `infra` bucket sees relevant providers everywhere.

Message payload — again line protocol, with a JSON array in the `event` field:

```
fleetdiscovery,cluster=fritz,type=ccmc event="[{\"type\":\"ccb\",\"hostname\":\"mgmt01\",\"state\":\"active\",\"meta\":{\"port\":\"8080\"}}]" 1734000000000000000
```

Each entry:

| Field      | Meaning                                           |
| ---------- | ------------------------------------------------- |
| `type`     | Provider's service type                           |
| `hostname` | Provider's host                                   |
| `state`    | Always `active` — only active services are listed |
| `meta`     | The provider's registration `metaData`, if any    |

Rosters deliberately carry **no** `instance_id`, no configuration and no
`config_revision`.

Rosters are re-published every `discovery-interval` and immediately (debounced
by 2 s) when a service registers or deregisters, so a late subscriber converges
within one interval — NATS core pub/sub is fire-and-forget, with no replay.
Treat each message as a **full replacement** of your provider list, not a delta.

Which providers a consumer gets is decided server-side by a single table
(`relevantProviders` in `servicetype.go`). By default every service type
discovers `ccb` (cc-backend), because every service must reach it to register
and pull config; richer peer edges are left commented out until a site's
topology is settled. `ccb` itself discovers no peers, so no roster is published
for consumer type `ccb`.

### Step 5 — Deregister on shutdown

```bash
curl -sS -o /dev/null -w '%{http_code}\n' -X DELETE \
  "$CCB/api/fleet/deregister/$INSTANCE_ID" -H "X-Auth-Token: $TOKEN"
# → 204
```

Idempotent and terminal. The member drops out of the discovery rosters
immediately (the publisher is notified, not left to the next tick). Skipping
this is not fatal — the service simply ages to `stale` after `stale-after` —
but a clean deregistration removes it from rosters at once.

### 5.1 Go client (cc-lib `fleet`)

The cc-lib `fleet` package implements the steps above, plus the parts that are
easy to get wrong by hand: falling back from NATS to REST heartbeats, re-registering
when cc-backend answers `404`, backing off on failures, and an optional on-disk
cache of the last configuration so a member can start while cc-backend is down.

```go
cfg, err := fleet.ParseConfig(ccconfig.GetPackageConfig("fleet"))
client, err := fleet.New(fleet.Options{
    Config:      cfg,
    ServiceType: fleet.ServiceMetricCollector,
    Meta:        map[string]string{"version": version}, // broadcast: no secrets
    NATS:        natsClient.Load, // func() *nats.Client, nil until connected
})

initial, err := client.Bootstrap(ctx, 10*time.Second) // register + first pull
applyConfig(initial.Config) // nil: run with the local configuration

go client.Run(ctx)
defer client.Close(shutdownCtx) // stops Run, then deregisters

for {
    select {
    case u := <-client.Configs():
        applyConfig(u.Config) // only sent when the configuration really changed
    case providers := <-client.Rosters():
        replaceProviders(providers) // full list, not a delta
    case <-ctx.Done():
        return
    }
}
```

The member's local `fleet` section (`url`, `token`, `cluster`, intervals,
`heartbeat-subject`, `cache-path`) and the full behaviour are documented in the
cc-lib `fleet` README.

---

## 6. Reference

### 6.1 REST endpoints

All are machine-to-machine, mounted under `/api`, and require a JWT with the
`api` role plus an allowlisted IP when `main.api-allowed-ips` is set. Read views
for the web UI are served by GraphQL, not from here.

| Method   | Path                                 | Success                              | Notes                                                           |
| -------- | ------------------------------------ | ------------------------------------ | --------------------------------------------------------------- |
| `POST`   | `/api/fleet/register/cluster/`       | `201` `{instanceId, configRevision}` | Body: `cluster`, `hostname`, `serviceType`, optional `metaData` |
| `POST`   | `/api/fleet/register/infra/`         | `201` `{instanceId, configRevision}` | Same without `cluster`                                          |
| `POST`   | `/api/fleet/heartbeat/{instanceID}`  | `204`                                | `404` for unknown/deregistered id                               |
| `GET`    | `/api/fleet/config/{instanceID}`     | `200` / `204` / `304`                | Merged configuration; `ETag` + `X-CC-Config-Revision`           |
| `DELETE` | `/api/fleet/deregister/{instanceID}` | `204`                                | Idempotent, terminal                                            |

Common failures: `400` malformed instance id or invalid registration, `401`
missing/invalid token, `403` token without the `api` role, `404` unknown or
deregistered instance, `500` server-side failure.

A malformed instance id (not exactly 32 lowercase hex characters) is rejected
with `400` before any lookup, so an id-guessing client cannot fill the log.

### 6.2 NATS subjects

| Subject                                                       | Direction            | Payload                                                        |
| ------------------------------------------------------------- | -------------------- | -------------------------------------------------------------- |
| `main.fleet.heartbeat-subject` (e.g. `cc.fleet.event`)        | client → cc-backend  | `fleet,function=heartbeat event="{\"instanceId\":\"…\"}" <ts>` |
| `<discovery-subject-prefix>.<cluster\|infra>.<consumer-type>` | cc-backend → clients | `fleetdiscovery,cluster=…,type=… event="[{…}]" <ts>`           |

The heartbeat subscription uses the queue group `cc-backend-fleet`, so several
cc-backend instances share the stream instead of each writing the same row.

Heartbeat writes are coalesced: decoder workers feed a single flusher that
applies at most one transaction per second, or as soon as 512 distinct instances
have accumulated. When the internal queues are full, heartbeats are dropped
rather than blocking the NATS callback — harmless, because the next heartbeat
arrives within seconds, and drops are summarised once per minute in the log.

### 6.3 Database

Table `service` (migration 13):

| Column                                | Notes                                              |
| ------------------------------------- | -------------------------------------------------- |
| `cluster`, `hostname`, `service_type` | Unique together; `cluster` empty for infra scope   |
| `instance_id`                         | Unique; rotated on every re-registration           |
| `scope`                               | `cluster` \| `infra`                               |
| `state`                               | `pending` \| `active` \| `stale` \| `deregistered` |
| `registered_at`, `last_heartbeat`     | Unix seconds                                       |
| `config_revision`                     | Last acknowledged content hash                     |
| `meta_data`                           | JSON `string → string`                             |

---

## 7. Security

**REST is the only path that can create or resurrect a service identity.**
Registration, deregistration and configuration all require an authenticated call
with the `api` role. This is a deliberate split, because the NATS subjects have
no application-layer authentication.

What a NATS publisher can do on the heartbeat subject, and only that: keep an
instance marked `active` — and therefore present in discovery rosters — _if it
has learned that instance's id_. It cannot register, deregister, change
configuration, or create an identity. Any other `function` value is rejected and
counted.

Consequences for operators:

- **Restrict publish ACLs on the broker.** Only trusted producers should be able
  to publish to `subject-job-event`, `subject-node-state` and the fleet
  `heartbeat-subject`. A shared, unrestricted NATS broker is not a safe
  deployment topology for this API. cc-backend logs a warning at startup when
  these subscriptions are enabled.
- **An `instance_id` is a bearer credential.** Do not log it, share it, or store
  it world-readable.
- **Discovery rosters are broadcast unauthenticated** and contain hostnames,
  service types and registration metadata. Never put secrets in a service's
  `metaData`.
- **Every fleet member's IP must be covered by `main.api-allowed-ips`** when
  that allowlist is configured, or registration and config pulls are refused.

---

## 8. Operations and troubleshooting

| Symptom                                                        | Likely cause                                                                                                                          |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `404` on every `/api/fleet/*` route                            | `main.fleet` block missing — the routes are not mounted                                                                               |
| `403` on registration                                          | Token lacks the `api` role                                                                                                            |
| Connection refused / rejected from the API                     | Member IP not covered by `main.api-allowed-ips`                                                                                       |
| Config pull always returns `204`                               | No layer file applies to this `(type, cluster, hostname)`, or `config-dir` is empty/absent                                            |
| Config edits are not picked up                                 | Waiting for `config-reload-interval`, or the tree failed to parse — check for `config reload failed, keeping previous snapshot`       |
| Startup fails with `initializing fleet: loading config tree …` | The tree is malformed; the initial load is synchronous by design                                                                      |
| Service stuck in `pending`                                     | It registered but never heartbeated — check the NATS subject name on both sides, or fall back to the REST heartbeat                   |
| Service flips to `stale`                                       | Heartbeat interval is too close to `stale-after`, heartbeats are being dropped, or the instance id was rotated by a re-registration   |
| Heartbeats appear to be ignored                                | Instance id unknown or deregistered — look for `heartbeat(s) for unknown or deregistered instance ids` in the log, then re-register   |
| `rejected message(s) in the last 1m0s`                         | Something is publishing a non-`heartbeat` function, a wrong measurement, or a malformed payload to the heartbeat subject              |
| No discovery rosters arrive                                    | NATS not connected (`discovery rosters will not be published`), wrong subject, or the consumer type has no relevant providers (`ccb`) |
| Roster is empty                                                | No provider of a relevant type is `active` in that bucket                                                                             |

Useful log lines to grep for: `fleet: initialized`, `fleet: registered`,
`fleet: marked N service(s) stale`, `fleet: config reload failed`,
`NATS fleet:`.

Rate-limited counters (unknown, rejected and dropped heartbeats) are summarised
once per minute rather than logged per message, so that anyone with publish
rights cannot flood the log.
