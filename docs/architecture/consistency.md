# Consistency Model

ObjeX provides **single-object, quorum-based, version-aware consistency** in distributed mode. This is not a global transaction model, not linearizable storage, and not serializable across objects.

## Single-node

Unchanged from prior phases: local SQLite + filesystem without cross-store transactions. See historical notes in git for upload/delete ordering.

## Distributed parameters

| Parameter | Env var | Meaning |
|-----------|---------|---------|
| **N** | `OBJEX_REPLICATION_FACTOR` | Nodes in replica set (includes primary) |
| **W** | `OBJEX_WRITE_QUORUM` | Required write acknowledgements |
| **R** | `OBJEX_READ_QUORUM` | Required read responses |

Defaults when cluster has ≥3 nodes: `N=3`, `W=2`, `R=2`.

Validation at startup:

```text
1 <= W <= N
1 <= R <= N
W + R > N   (default overlap mode)
```

Configurations with `W + R <= N` are rejected unless explicitly relaxed in a future mode.

## Why W + R > N?

With `N=3, W=2, R=2`, any set of W write acknowledgements and any set of R read responses must intersect in at least one replica. That intersection is where version information can overlap.

**Important:** overlap alone does not guarantee linearizability. ObjeX additionally:

- assigns monotonic versions at the primary per `{bucket,key}`
- compares versions on reads and selects the newest
- rejects stale replica writes (`incoming.version < stored.version`)
- uses versioned tombstones for delete

## Version model

Each object mutation receives a monotonic integer `version` per `{bucket,key}`. The **primary is the serialization point** for version assignment in this phase (no multi-primary conflict resolution).

| Case | Behavior |
|------|----------|
| `incoming < stored` | Stale — reject |
| `incoming == stored` && same ETag | Idempotent success |
| `incoming == stored` && different ETag | Conflict error |
| `incoming > stored` | Accept after checksum verify (puts) |

Concurrent client PUTs to the same key are ordered by the primary's version assignment; readers never mix metadata from one version with bytes from another.

## PUT (distributed)

A successful PUT means **at least W replicas** durably acknowledged the new version (primary counts toward W).

Example `N=3, W=2`:

```text
Primary ✓  Replica1 ✓  Replica2 ✗  →  success + hint for Replica2
Primary ✓  Replica1 ✗  Replica2 ✗  →  failure (partial state may exist)
```

Failed replicas are recorded in durable **hinted handoff** state; a background worker retries delivery across process restarts.

## GET / HEAD (distributed)

A successful read requires **at least R** replica metadata responses. The implementation queries **all N** nodes (bounded timeout), then selects the **highest version** observed.

If the winning state is a **tombstone**, the API returns Not Found (S3 semantics).

Stale replicas (`version < winner`) trigger **read repair** hints (durable, async).

## DELETE (distributed)

DELETE writes a **versioned tombstone** (`deleted=true`). Success requires W tombstone acknowledgements. Tombstones participate in version comparison so an old object copy cannot resurrect after delete.

Physical blob GC for tombstoned objects is deferred.

## Hinted handoff

When a write reaches W quorum but some replicas failed, durable hints queue delivery to missing nodes. Retries use exponential backoff and survive process restart.

## Read repair

When a read detects `replica.version < winner.version`, a durable repair hint schedules `replicate-put` to the stale node. Repair does not block the client response.

## Weaker / out of scope

| Operation | Consistency |
|-----------|-------------|
| LIST objects | Not quorum-consistent (existing semantics) |
| Multipart | Primary-local, not replicated |
| Copy-object | Not quorum-replicated |
| Bucket operations | Fan-out metadata only |

## Recovery limitations

Failure detection is probe-based (HTTP `/health/live`), not Byzantine-fault tolerant. Unreachable peers are surfaced in metrics and `/cluster` but quorum math still counts all **N** placement nodes. Hints, read repair, and the healing scheduler close gaps when peers return.

No automatic cluster membership changes or full anti-entropy scan.

## ETags

- Single PUT: MD5 of object bytes (hex, lowercase)
- Multipart: S3 multipart ETag format

## Crash recovery

Local per-node: orphan `.tmp` cleanup, orphan blob job, SQLite WAL. No cross-node transaction.

See [quorum.md](quorum.md) and [replication.md](replication.md) for protocol detail.
