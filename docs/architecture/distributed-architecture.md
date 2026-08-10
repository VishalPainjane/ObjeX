# Distributed Architecture — Phase 3/4

Multi-node ObjeX uses static membership, deterministic placement, primary forwarding, **object replication**, **quorum-based consistency**, and **runtime peer health**.

## Node

A **node** is one ObjeX process with:

- **Node ID** — stable identity (`OBJEX_NODE_ID` or `--node-id`, default `node-1`)
- **Address** — configured HTTP endpoint (`localhost:9001`, etc.)
- **Status** — configured membership state (`active`)
- **Reachable** — runtime health from peer probes (Phase 4; not persisted)

Each node has its own SQLite metadata and filesystem blob store under `OBJEX_DATA_DIR`.

## Cluster

A **cluster** is the set of nodes known from shared static configuration. Every node loads the same membership list so placement calculations agree.

```text
Cluster (RF=3)
 ├── node-1  (:9001)
 ├── node-2  (:9002)
 └── node-3  (:9003)
```

## Replication topology

```text
                 Client
                   │
                   ▼
              Any Node
                   │
             Primary Route (forward if needed)
                   │
                   ▼
                Primary
              /    |    \
             /     |     \
            ▼      ▼      ▼
         local   R1      R2
    (quorum W) (replicate) (replicate)
                   │
         ┌─────────┴─────────┐
         ▼                   ▼
  Hinted Handoff        Read Repair
  (durable hints)       (stale replicas)
         │
         ▼
  Peer Health + Healing (Phase 4)
```

## Layering

```text
HTTP API (S3 + /cluster + /debug/placement)
        │
        ▼
Replication Coordinator (quorum PUT/GET/HEAD/DELETE)
        │
        ├── Quorum (N/R/W thresholds)
        ├── Replicator (HTTP to peers)
        ├── Hint Worker (durable recovery)
        └── Healing Worker (partial object scan)
        │
        ▼
Peer Health Monitor (runtime reachability)
        │
        ▼
Object Service  (local blob + metadata, replica ops)
        │
        └── Placement (rendezvous, RF nodes)
```

## Configuration

| Variable | Purpose |
|----------|---------|
| `OBJEX_NODE_ID` | Local node identity |
| `OBJEX_CLUSTER_CONFIG` | Shared membership JSON |
| `OBJEX_CLUSTER_INTERNAL_TOKEN` | Inter-node auth |
| `OBJEX_REPLICATION_FACTOR` | Replica set size **N** |
| `OBJEX_WRITE_QUORUM` | Write quorum **W** |
| `OBJEX_READ_QUORUM` | Read quorum **R** |

### Local three-node cluster

```bash
docker compose -f docker-compose.cluster.yml up
```

Or manually with `configs/cluster-3.json` and distinct `OBJEX_DATA_DIR` per node.

## Feature status

| Feature | Status |
|---------|--------|
| Primary forwarding (PUT/GET/HEAD/DELETE) | ✅ |
| Internal cluster auth | ✅ |
| Bucket metadata fan-out on create | ✅ |
| Quorum writes / reads (N/R/W) | ✅ |
| Tombstones, hints, read repair | ✅ |
| Peer health + recovery callbacks | ✅ |
| Background healing (partial objects) | ✅ |
| Multipart / copy quorum | ❌ deferred |
| Membership changes | ❌ deferred |

## Endpoints

| Path | Auth | Purpose |
|------|------|---------|
| `GET /cluster` | Skipped | Local node ID + membership + reachability |
| `GET /debug/placement?bucket=&key=` | Skipped | Primary + replica nodes |
| `GET /health/live` | Skipped | Liveness (used by peer probes) |

## Metrics

See [observability.md](observability.md) and [failure-detection.md](failure-detection.md).

## Next

Erasure coding, automatic rebalancing, and consensus remain deferred.

See also: [placement.md](placement.md), [replication.md](replication.md), [quorum.md](quorum.md)
