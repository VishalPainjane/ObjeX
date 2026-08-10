# Replication

Replication copies versioned objects across the placement replica set. **Phase 3** adds quorum thresholds (N/R/W), tombstones, durable hinted handoff, and read repair. See `quorum.md` and `consistency.md` for the full model.

## Replication factor (N)

Configured via `OBJEX_REPLICATION_FACTOR` (default: `3` when the cluster has ≥3 nodes, otherwise `1`).

Invariant: **RF ≤ cluster size**. If RF exceeds membership, placement returns `ErrReplicationFactorTooLarge`.

## Placement

Rendezvous hashing ranks all nodes by score for `{bucket}/{key}`. The top **RF** distinct nodes form the replica set:

```text
Primary  = highest score
Replica1 = second highest
Replica2 = third highest
```

`PlacementResult` exposes `Primary`, `Replicas`, and `ReplicaSet()` (primary + replicas).

## Write path

```text
Client → any node → forward to primary (if needed)
Primary:
  1. allocate monotonic object version
  2. write local blob + metadata (replication_status=partial)
  3. stream object to each replica (parallel HTTP)
  4. replicas verify MD5 checksum
  5. if W replicas ACK (including primary) → success; enqueue hints for failures
  6. if quorum not met → return error (partial state may remain on some nodes)
```

**Policy (Phase 3):** PUT succeeds when **W** replicas acknowledge (`OBJEX_WRITE_QUORUM`, default `2` when N=3). Failed replicas receive durable hints for background delivery.

## Read path

GET/HEAD remain **primary-only**. Replicas exist for durability but are not consulted on reads yet.

## Delete path

Primary deletes locally, then issues internal replicate-delete to each replica. DELETE succeeds only when all replicas acknowledge (idempotent on missing objects).

## Internal protocol

Authenticated with `X-ObjeX-Internal-Token`. Operation semantics are separate:

| Header | Value |
|--------|-------|
| `X-ObjeX-Internal-Operation` | `replicate-put` or `replicate-delete` |
| `X-ObjeX-Object-Version` | monotonic generation |
| `X-ObjeX-Expected-ETag` | MD5 hex (replicate-put only) |

Replica writes **never** trigger further replication (loop prevention).

Clients cannot forge internal operations without the cluster token.

## Object version

Each object has a monotonic `version` per `{bucket,key}` on the primary. Replicas store the same version.

Stale rule on replica PUT:

```text
incoming version < stored version  → reject (409)
incoming version == stored && same ETag → idempotent success
incoming version > stored version → accept after checksum verify
```

## Replication metadata

Primary metadata tracks `replication_status`:

- `partial` — primary (and maybe some replicas) have the object; not all replicas ACKed
- `replicated` — all replicas acknowledged

This is **local primary metadata only** — not a distributed consensus log.

## Deferred in this phase

- Read quorum / write quorum
- Multipart replication (multipart remains primary-local)
- Copy-object replication
- Automatic healing / read repair
- Failure detection

## Metrics

- `objex_replication_operations_total{operation,result}`
- `objex_replication_failures_total{operation}`
- `objex_replication_bytes_total`
- `objex_replication_duration_seconds{operation}`
