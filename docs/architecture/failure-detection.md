# Failure Detection & Healing — Phase 4

Runtime peer health and background healing build on Phase 3 quorum, hints, and read repair.

## Peer health monitor

Each node periodically probes peers with `GET /health/live` (no auth).

| Setting | Default | Env var |
|---------|---------|---------|
| Probe interval | 5s | `OBJEX_PEER_HEALTH_INTERVAL` (future) |
| Probe timeout | 2s | `OBJEX_PEER_HEALTH_TIMEOUT` (future) |
| Fail threshold | 3 consecutive failures | — |
| Recover threshold | 2 consecutive successes | — |

State is **in-memory only** — static membership JSON is unchanged.

- After **fail threshold**: peer marked unreachable (`objex_cluster_peer_reachable{node_id}=0`)
- After **recover threshold**: peer marked reachable again; recovery callback runs

`GET /cluster` includes optional `"reachable": true|false` per peer (omitted for local node).

## Recovery actions

When a peer transitions to reachable:

1. Reset pending hint backoff for that target (`replication_hints.next_attempt_at = now`)
2. Run one hint-worker batch (deliver due hints immediately)
3. Run one healing scan (partial objects on this primary)

## Background healing

Every 60s (default), each primary scans up to 50 objects with `replication_status = partial`:

1. Collect replica states via `replicate-head` (same as quorum reads)
2. Schedule repair hints for stale/missing replicas (reuses read-repair path)
3. If all replicas match the primary version, set `replication_status = replicated`

Healing does **not** change placement or quorum thresholds — it only enqueues hints.

## Metrics

| Metric | Description |
|--------|-------------|
| `objex_cluster_peer_reachable` | 1 when peer responds to `/health/live` |
| `objex_cluster_peer_recovery_total` | Recovery transitions |
| `objex_healing_scans_total` | Healing scan iterations |
| `objex_healing_objects_total` | Objects examined per scan |

## Explicitly deferred

- Automatic membership changes (add/remove nodes)
- Skipping unreachable peers in quorum math (still attempt replication; hints cover gaps)
- Full cluster anti-entropy scan of every object
- Raft / consensus
